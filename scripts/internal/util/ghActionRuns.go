package util

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
)

func ListWorkflowRuns(ctx context.Context, client *github.Client, owner, repo string) (err error) {
	runs, _, err := client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &github.ListWorkflowRunsOptions{})

	if err != nil {
		return
	}

	latestRun := runs.WorkflowRuns[0]

	jobs, _, err := client.Actions.ListWorkflowJobs(ctx, owner, repo, latestRun.GetID(), &github.ListWorkflowJobsOptions{})

	if err != nil {
		return
	}

	fmt.Print(jobs)

	return
}
