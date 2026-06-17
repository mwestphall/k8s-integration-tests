package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/google/go-github/v68/github"
	"github.com/osg-htc/k8s-integration-tests/webapp/util"
)

// App holds shared dependencies for all HTTP handlers.
type App struct {
	client *github.Client
	owner  string
	repo   string
	tmpl   *template.Template
	static fs.FS
}

func NewApp(client *github.Client, owner, repo string, tmpl *template.Template, static fs.FS) *App {
	return &App{client: client, owner: owner, repo: repo, tmpl: tmpl, static: static}
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(a.static)))
	mux.HandleFunc("GET /", a.handleLanding)
	mux.HandleFunc("GET /runs/{runID}", a.handleRun)
	mux.HandleFunc("GET /runs/{runID}/jobs/{jobID}", a.handleJob)
	mux.HandleFunc("GET /runs/{runID}/jobs/{jobID}/logs/{pod}/{container}", a.handleLogs)
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %q error: %v", name, err)
	}
}

func (a *App) httpError(w http.ResponseWriter, err error, code int) {
	log.Printf("http %d: %v", code, err)
	http.Error(w, err.Error(), code)
}

// handleLanding shows the 20 most recent runs of run-tests.yaml with per-suite
// success/failure counts. Job lists are fetched concurrently.
func (a *App) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	runsResp, _, err := a.client.Actions.ListWorkflowRunsByFileName(
		r.Context(), a.owner, a.repo, "run-tests.yaml",
		&github.ListWorkflowRunsOptions{ListOptions: github.ListOptions{PerPage: 20}},
	)
	if err != nil {
		a.httpError(w, fmt.Errorf("listing workflow runs: %w", err), http.StatusInternalServerError)
		return
	}

	type runRow struct {
		Run    *github.WorkflowRun
		Suites []util.SuiteStatus
	}

	rows := make([]runRow, len(runsResp.WorkflowRuns))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i, run := range runsResp.WorkflowRuns {
		wg.Add(1)
		go func(i int, run *github.WorkflowRun) {
			defer wg.Done()
			jobs, _, err := a.client.Actions.ListWorkflowJobs(
				r.Context(), a.owner, a.repo, run.GetID(),
				&github.ListWorkflowJobsOptions{ListOptions: github.ListOptions{PerPage: 100}},
			)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("listing jobs for run %d: %w", run.GetID(), err)
				}
				return
			}
			suites, _ := util.GroupJobsBySuite(jobs.Jobs)
			rows[i] = runRow{Run: run, Suites: suites}
		}(i, run)
	}
	wg.Wait()

	if firstErr != nil {
		a.httpError(w, firstErr, http.StatusInternalServerError)
		return
	}

	a.render(w, "landing", map[string]any{"Rows": rows})
}

// handleRun shows all jobs for a single run, grouped by suite.
func (a *App) handleRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	run, _, err := a.client.Actions.GetWorkflowRunByID(r.Context(), a.owner, a.repo, runID)
	if err != nil {
		a.httpError(w, fmt.Errorf("getting run: %w", err), http.StatusInternalServerError)
		return
	}

	jobsResp, _, err := a.client.Actions.ListWorkflowJobs(
		r.Context(), a.owner, a.repo, runID,
		&github.ListWorkflowJobsOptions{ListOptions: github.ListOptions{PerPage: 100}},
	)
	if err != nil {
		a.httpError(w, fmt.Errorf("listing jobs: %w", err), http.StatusInternalServerError)
		return
	}

	artifactsResp, _, err := a.client.Actions.ListWorkflowRunArtifacts(
		r.Context(), a.owner, a.repo, runID,
		&github.ListOptions{PerPage: 100},
	)
	if err != nil {
		a.httpError(w, fmt.Errorf("listing artifacts: %w", err), http.StatusInternalServerError)
		return
	}

	suiteStatuses, suiteJobs := util.GroupJobsBySuite(jobsResp.Jobs)

	type jobRow struct {
		Job         *github.WorkflowJob
		HasArtifact bool
		Conclusion  string
		DisplayName string
		Matrix      []util.MatrixEntry
	}
	type suiteRow struct {
		Status util.SuiteStatus
		Jobs   []jobRow
	}

	suiteRows := make([]suiteRow, 0, len(suiteStatuses))
	for _, ss := range suiteStatuses {
		jrows := make([]jobRow, 0, len(suiteJobs[ss.Name]))
		for _, j := range suiteJobs[ss.Name] {
			jrows = append(jrows, jobRow{
				Job:         j,
				HasArtifact: util.MatchArtifactToJob(j, artifactsResp.Artifacts) != nil,
				Conclusion:  util.JobConclusion(j),
				DisplayName: util.JobDisplayName(j.GetName()),
				Matrix:      util.ParseJobMatrix(j.Steps),
			})
		}
		suiteRows = append(suiteRows, suiteRow{Status: ss, Jobs: jrows})
	}

	a.render(w, "run", map[string]any{
		"Run":    run,
		"Suites": suiteRows,
	})
}

// handleJob shows pod events and container log links for a specific job.
func (a *App) handleJob(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	jobID, err := strconv.ParseInt(r.PathValue("jobID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	job, _, err := a.client.Actions.GetWorkflowJobByID(r.Context(), a.owner, a.repo, jobID)
	if err != nil {
		a.httpError(w, fmt.Errorf("getting job: %w", err), http.StatusInternalServerError)
		return
	}

	artifactsResp, _, err := a.client.Actions.ListWorkflowRunArtifacts(
		r.Context(), a.owner, a.repo, runID,
		&github.ListOptions{PerPage: 100},
	)
	if err != nil {
		a.httpError(w, fmt.Errorf("listing artifacts: %w", err), http.StatusInternalServerError)
		return
	}

	// Fetch test results independently of artifact availability.
	var testResults []util.TestResult
	passCount, failCount := 0, 0
	if util.HasTestStep(job.Steps) {
		if results, err := util.FetchAndParseTestResults(r.Context(), a.client, a.owner, a.repo, runID, job.GetID()); err != nil {
			log.Printf("fetching test results for job %d: %v", job.GetID(), err)
		} else {
			testResults = results
			for _, res := range testResults {
				if res.Status == "PASS" {
					passCount++
				} else {
					failCount++
				}
			}
		}
	}

	data := map[string]any{
		"RunID":         runID,
		"JobID":         jobID,
		"Job":           job,
		"Conclusion":    util.JobConclusion(job),
		"DisplayName":   util.JobDisplayName(job.GetName()),
		"Matrix":        util.ParseJobMatrix(job.Steps),
		"Pods":          nil,
		"TestResults":   testResults,
		"TestPassCount": passCount,
		"TestFailCount": failCount,
	}

	artifact := util.MatchArtifactToJob(job, artifactsResp.Artifacts)
	if artifact == nil {
		a.render(w, "job", data)
		return
	}

	dir, err := util.EnsureArtifactExtracted(r.Context(), a.client, a.owner, a.repo, runID, artifact.GetID())
	if err != nil {
		a.httpError(w, fmt.Errorf("extracting artifact: %w", err), http.StatusInternalServerError)
		return
	}

	pods, err := util.ParsePodFiles(dir)
	if err != nil {
		a.httpError(w, fmt.Errorf("parsing pod files: %w", err), http.StatusInternalServerError)
		return
	}

	data["Pods"] = pods
	a.render(w, "job", data)
}

// handleLogs displays the raw log file for a specific container in a specific pod.
func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	jobID, err := strconv.ParseInt(r.PathValue("jobID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pod := r.PathValue("pod")
	container := r.PathValue("container")

	job, _, err := a.client.Actions.GetWorkflowJobByID(r.Context(), a.owner, a.repo, jobID)
	if err != nil {
		a.httpError(w, fmt.Errorf("getting job: %w", err), http.StatusInternalServerError)
		return
	}

	artifactsResp, _, err := a.client.Actions.ListWorkflowRunArtifacts(
		r.Context(), a.owner, a.repo, runID,
		&github.ListOptions{PerPage: 100},
	)
	if err != nil {
		a.httpError(w, fmt.Errorf("listing artifacts: %w", err), http.StatusInternalServerError)
		return
	}

	artifact := util.MatchArtifactToJob(job, artifactsResp.Artifacts)
	if artifact == nil {
		http.Error(w, "artifact not found for this job", http.StatusNotFound)
		return
	}

	dir, err := util.EnsureArtifactExtracted(r.Context(), a.client, a.owner, a.repo, runID, artifact.GetID())
	if err != nil {
		a.httpError(w, fmt.Errorf("extracting artifact: %w", err), http.StatusInternalServerError)
		return
	}

	logFile := filepath.Join(dir, fmt.Sprintf("%s_%s.log", pod, container))
	content, err := os.ReadFile(logFile)
	if err != nil {
		a.httpError(w, fmt.Errorf("reading log file: %w", err), http.StatusNotFound)
		return
	}

	a.render(w, "logs", map[string]any{
		"RunID":       runID,
		"JobID":       jobID,
		"Job":         job,
		"Pod":         pod,
		"Container":   container,
		"Content":     string(content),
		"Conclusion":  util.JobConclusion(job),
		"DisplayName": util.JobDisplayName(job.GetName()),
	})
}
