package main

import (
	"context"

	"github.com/osg-htc/k8s-integration-tests/scripts/internal/util"
)

func main() {
	ctx := context.TODO()
	client := util.NewGitHubClient(ctx)

	util.ListWorkflowRuns(ctx, client, "osg-htc", "k8s-integration-tests")
}
