package util

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v68/github"
)

// TestResult holds the outcome of a single Go test case parsed from job logs.
type TestResult struct {
	Name     string
	Status   string // "PASS" or "FAIL"
	Duration string
}

// HasTestStep reports whether the job contains a step whose name matching "Test <SingleWordTestName>",
// indicating that go test output is present in the job logs.
func HasTestStep(steps []*github.TaskStep) bool {
	for _, step := range steps {
		if match, err := regexp.MatchString(`^Test \w+$`, step.GetName()); err == nil && match {
			return true
		}
	}
	return false
}

// fetchRawJobLog downloads (or reads from cache) the raw log for a job.
func fetchRawJobLog(ctx context.Context, client *github.Client, owner, repo string, runID, jobID int64) ([]byte, error) {
	cachePath := filepath.Join(os.TempDir(), "k8s-webapp-cache",
		fmt.Sprintf("%d", runID), fmt.Sprintf("job-%d.log", jobID))

	if data, err := os.ReadFile(cachePath); err == nil {
		return data, nil
	}

	u, _, err := client.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 3)
	if err != nil {
		return nil, fmt.Errorf("getting job log URL: %w", err)
	}
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("downloading job logs: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading job logs: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err == nil {
		os.WriteFile(cachePath, data, 0644)
	}
	return data, nil
}

// FetchAndParseTestResults downloads the job log, caches it under
// k8s-webapp-cache/<runID>/job-<jobID>.log, and returns all parsed test results.
func FetchAndParseTestResults(ctx context.Context, client *github.Client, owner, repo string, runID, jobID int64) ([]TestResult, error) {
	data, err := fetchRawJobLog(ctx, client, owner, repo, runID, jobID)
	if err != nil {
		return nil, err
	}
	return parseTestResults(data), nil
}

// FetchFilteredTestLogs downloads the job log (using the same cache) and returns the
// lines relevant to testName. If testName is empty, all lines are returned unfiltered.
// Each line is matched by stripping the leading GHA timestamp prefix (up to the first space)
// and checking whether the remainder starts with "<testName> ".
// Lines not matching the "<testName> <timestamp>" format are omitted when filtering.
func FetchFilteredTestLogs(ctx context.Context, client *github.Client, owner, repo string, runID, jobID int64, testName string) (string, error) {
	data, err := fetchRawJobLog(ctx, client, owner, repo, runID, jobID)
	if err != nil {
		return "", err
	}
	if testName == "" {
		return string(data), nil
	}
	return filterLogByTest(data, testName), nil
}

// filterLogByTest returns only the lines whose content (after stripping the leading GHA
// timestamp) starts with "<testName> ". Lines that do not match this pattern are omitted.
func filterLogByTest(data []byte, testName string) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lines []string
	parser := regexp.MustCompile(`\S+ (\S+) (\S+) \S+: (.*)`)
	for scanner.Scan() {
		line := scanner.Text()
		// Strip leading GHA timestamp (everything up to and including the first space).
		matches := parser.FindStringSubmatch(line)
		if len(matches) == 4 && matches[1] == testName {
			lines = append(lines, fmt.Sprintf("%v %v", matches[2], matches[3]))
		}
	}
	return strings.Join(lines, "\n")
}

// parseTestResults scans log output for "--- PASS:" and "--- FAIL:" summary lines
// emitted by `go test -v`. The pattern is found anywhere in the line to tolerate
// GitHub Actions timestamp prefixes.
func parseTestResults(data []byte) []TestResult {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var results []TestResult
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, "--- ")
		if idx < 0 {
			continue
		}
		rest := line[idx+4:]
		var status string
		switch {
		case strings.HasPrefix(rest, "PASS: "):
			status, rest = "PASS", rest[6:]
		case strings.HasPrefix(rest, "FAIL: "):
			status, rest = "FAIL", rest[6:]
		default:
			continue
		}
		// rest is "<name> (<duration>)"
		parenIdx := strings.LastIndex(rest, " (")
		if parenIdx < 0 {
			continue
		}
		results = append(results, TestResult{
			Name:     rest[:parenIdx],
			Status:   status,
			Duration: strings.TrimSuffix(rest[parenIdx+2:], ")"),
		})
	}
	return results
}
