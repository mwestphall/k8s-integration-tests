package util

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/google/go-github/v68/github"
)

// PodDetail holds the events and container names for a single pod.
type PodDetail struct {
	Name       string
	Events     string
	Containers []string
}

// SuiteStatus summarises the job outcomes for one test suite.
type SuiteStatus struct {
	Name    string
	Success int
	Failure int
	Ongoing int
	Total   int
}

// MatrixEntry is a single key-value pair from a job's matrix environment.
type MatrixEntry struct {
	Key   string
	Value string
}

// JobSuite extracts the suite prefix from a job name ("suite / rest" → "suite").
func JobSuite(jobName string) string {
	if strings.Contains(jobName, " / ") {
		return strings.Split(jobName, " / ")[0]
	}
	return ""
}

// JobDisplayName returns the human-readable test name from a job name.
//
// Job names take the form "<suite> / <display name> / Run Tests [(...)]".
// This function returns the middle segment (e.g. "Run Pelican tests").
func JobDisplayName(jobName string) string {
	parts := strings.Split(jobName, " / ")
	if len(parts) >= 2 {
		// Fallback for old-format names: strip any trailing " (...)" matrix suffix.
		name := strings.TrimSpace(parts[1])
		if idx := strings.Index(name, " ("); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
		}
		return name
	}
	return jobName
}

// ParseJobMatrix parses the "Job Matrix: <json>" step name and returns the
// key-value pairs sorted alphabetically by key.
func ParseJobMatrix(steps []*github.TaskStep) []MatrixEntry {
	const prefix = "Job Matrix: "
	for _, step := range steps {
		name := step.GetName()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(name, prefix)), &m); err != nil {
			return nil
		}
		keys := slices.Sorted(maps.Keys(m))
		entries := make([]MatrixEntry, 0, len(keys))
		for _, k := range keys {
			displayKey := k
			if strings.Contains(k, "_") {
				keyParts := strings.Split(k, "_")
				displayKey = keyParts[len(keyParts)-1]
			}
			entries = append(entries, MatrixEntry{Key: displayKey, Value: m[k]})
		}
		return entries
	}
	return nil
}

// JobConclusion returns a display-ready status string for a job.
func JobConclusion(job *github.WorkflowJob) string {
	if job.GetStatus() != "completed" {
		return job.GetStatus()
	}
	return job.GetConclusion()
}

// RunSummary holds the run and its jobs grouped by suite, as returned by FetchRunSummary.
type RunSummary struct {
	Run       *github.WorkflowRun
	Suites    []SuiteStatus
	SuiteJobs map[string][]*github.WorkflowJob
}

// FetchRunSummary fetches a workflow run and all its jobs, then groups the jobs by suite.
// The error returned is the raw API error; callers may inspect it as *github.ErrorResponse
// (e.g. to distinguish a 404 from other failures).
func FetchRunSummary(ctx context.Context, client *github.Client, owner, repo string, runID int64) (*RunSummary, error) {
	run, _, err := client.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}

	jobsResp, _, err := client.Actions.ListWorkflowJobs(
		ctx, owner, repo, runID,
		&github.ListWorkflowJobsOptions{ListOptions: github.ListOptions{PerPage: 100}},
	)
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}

	suites, suiteJobs := GroupJobsBySuite(jobsResp.Jobs)
	return &RunSummary{Run: run, Suites: suites, SuiteJobs: suiteJobs}, nil
}

// GroupJobsBySuite partitions jobs by suite prefix and builds SuiteStatus summaries.
// It returns the statuses in the order suites were first encountered, and a map from
// suite name to the jobs in that suite.
func GroupJobsBySuite(jobs []*github.WorkflowJob) ([]SuiteStatus, map[string][]*github.WorkflowJob) {
	suiteJobs := make(map[string][]*github.WorkflowJob)

	for _, j := range jobs {
		suite := JobSuite(j.GetName())
		if suite == "" {
			continue
		}
		suiteJobs[suite] = append(suiteJobs[suite], j)
	}

	order := slices.Sorted(maps.Keys(suiteJobs))

	statuses := make([]SuiteStatus, 0, len(order))
	for _, suite := range order {
		ss := SuiteStatus{Name: suite, Total: len(suiteJobs[suite])}
		for _, j := range suiteJobs[suite] {
			switch JobConclusion(j) {
			case "success":
				ss.Success++
			case "queued", "in_progress":
				ss.Ongoing++
			default:
				ss.Failure++
			}
		}
		statuses = append(statuses, ss)
	}
	return statuses, suiteJobs
}

// MatchArtifactToJob finds the artifact corresponding to the given job.
//
// Each job has a step named "Upload Test Logs for <hash>". Artifacts are named
// "<anything>-<hash>". The hash is extracted from the step name and matched
// against the artifact name suffix; the prefix is no longer suite-specific.
func MatchArtifactToJob(job *github.WorkflowJob, artifacts []*github.Artifact) *github.Artifact {
	const stepPrefix = "Upload Test Logs of "
	var hash string
	for _, step := range job.Steps {
		if strings.HasPrefix(step.GetName(), stepPrefix) {
			hash = strings.TrimPrefix(step.GetName(), stepPrefix)
			break
		}
	}
	if hash == "" {
		return nil
	}

	hashSuffix := "-" + hash
	for _, artifact := range artifacts {
		if strings.HasSuffix(artifact.GetName(), hashSuffix) {
			return artifact
		}
	}
	return nil
}

// EnsureArtifactExtracted downloads and extracts the artifact if not already cached.
// Returns the path to the directory containing the extracted files.
func EnsureArtifactExtracted(ctx context.Context, client *github.Client, owner, repo string, runID, artifactID int64) (string, error) {
	cacheDir := filepath.Join(os.TempDir(), "k8s-webapp-cache",
		fmt.Sprintf("%d", runID), fmt.Sprintf("%d", artifactID))
	sentinel := filepath.Join(cacheDir, ".complete")

	if _, err := os.Stat(sentinel); err == nil {
		return cacheDir, nil
	}

	u, _, err := client.Actions.DownloadArtifact(ctx, owner, repo, artifactID, 3)
	if err != nil {
		return "", fmt.Errorf("getting artifact download URL: %w", err)
	}

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("downloading artifact: %w", err)
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "artifact-*.zip")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return "", fmt.Errorf("writing artifact: %w", err)
	}
	tmp.Close()

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	if err := extractZip(tmpName, cacheDir); err != nil {
		return "", fmt.Errorf("extracting artifact: %w", err)
	}
	if err := os.WriteFile(sentinel, []byte{}, 0644); err != nil {
		return "", err
	}
	return cacheDir, nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// Discover the common top-level directory (if any).
	topDir := ""
	for _, f := range r.File {
		if parts := strings.SplitN(f.Name, "/", 2); len(parts) == 2 && parts[0] != "" {
			topDir = parts[0]
			break
		}
	}
	// Verify every entry shares that directory; if not, treat as no top-level dir.
	if topDir != "" {
		for _, f := range r.File {
			if f.Name != topDir && !strings.HasPrefix(f.Name, topDir+"/") {
				topDir = ""
				break
			}
		}
	}

	for _, f := range r.File {
		relPath := f.Name
		if topDir != "" {
			relPath = strings.TrimPrefix(f.Name, topDir+"/")
			if relPath == "" {
				continue
			}
		}

		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
		rel, err := filepath.Rel(destDir, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue // zip slip protection
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			dst.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// ParsePodFiles reads the extracted artifact directory and returns one PodDetail
// per pod found, combining .events and _<container>.logs files.
func ParsePodFiles(dir string) ([]PodDetail, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type podData struct {
		events     string
		containers []string
	}
	pods := make(map[string]*podData)

	ensure := func(name string) *podData {
		if pods[name] == nil {
			pods[name] = &podData{}
		}
		return pods[name]
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		switch {
		case strings.HasSuffix(name, ".events"):
			podName := strings.TrimSuffix(name, ".events")
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			ensure(podName).events = string(data)

		case strings.HasSuffix(name, ".log"):
			base := strings.TrimSuffix(name, ".log")
			idx := strings.LastIndex(base, "_")
			if idx < 0 {
				continue
			}
			podName, containerName := base[:idx], base[idx+1:]
			pd := ensure(podName)
			pd.containers = append(pd.containers, containerName)
		}
	}

	result := make([]PodDetail, 0, len(pods))
	for name, pd := range pods {
		sort.Strings(pd.containers)
		result = append(result, PodDetail{
			Name:       name,
			Events:     pd.events,
			Containers: pd.containers,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
