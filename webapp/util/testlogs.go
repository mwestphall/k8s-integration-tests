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

// FetchAndParseTestResults downloads the job log, caches it under
// k8s-webapp-cache/<runID>/job-<jobID>.log, and returns all parsed test results.
func FetchAndParseTestResults(ctx context.Context, client *github.Client, owner, repo string, runID, jobID int64) ([]TestResult, error) {
	cachePath := filepath.Join(os.TempDir(), "k8s-webapp-cache",
		fmt.Sprintf("%d", runID), fmt.Sprintf("job-%d.log", jobID))

	var logData []byte
	if data, err := os.ReadFile(cachePath); err == nil {
		logData = data
	} else {
		u, _, err := client.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 3)
		if err != nil {
			return nil, fmt.Errorf("getting job log URL: %w", err)
		}
		resp, err := http.Get(u.String())
		if err != nil {
			return nil, fmt.Errorf("downloading job logs: %w", err)
		}
		defer resp.Body.Close()

		logData, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading job logs: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err == nil {
			os.WriteFile(cachePath, logData, 0644)
		}
	}

	return parseTestResults(logData), nil
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
