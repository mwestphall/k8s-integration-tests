// pelicanVersionMatrix generates a JSON matrix of Pelican versions for testing.
// It fetches the latest stable release and the latest release candidate (RC) from GitHub,
// and constructs a matrix that can be used in GitHub Actions to run tests against these versions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-github/v68/github"
	"github.com/osg-htc/k8s-integration-tests/scripts/internal/util"
	"golang.org/x/oauth2"
)

const (
	owner          = "PelicanPlatform"
	repo           = "pelican"
	releasePattern = `^v\d+\.\d+\.\d+$`
	rcPattern      = `^v\d+\.\d+\.\d+-rc\.\d+$`
)

// versionMatrix matches the GitHub Actions matrix format. Each field lists the
// versions to test; GHA expands them into a full cross-product of combinations.
type versionMatrix struct {
	CacheVersion    []string `json:"cacheVersion"`
	OriginVersion   []string `json:"originVersion"`
	RegistryVersion []string `json:"registryVersion"`
	DirectorVersion []string `json:"directorVersion"`
	ClientVersion   []string `json:"clientVersion"`
}

// newGitHubClient creates a GitHub client, based on the `GITHUB_TOKEN` environment variable.
// This is set by default inside GHAs.
func newGitHubClient(ctx context.Context) *github.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return github.NewClient(nil)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return github.NewClient(oauth2.NewClient(ctx, ts))
}

func main() {
	ctx := context.Background()
	client := newGitHubClient(ctx)

	// Find the latest stable release
	release, err := util.LatestMatchingRelease(ctx, client, owner, repo, releasePattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching latest release: %v\n", err)
		os.Exit(1)
	}

	// Find the latest release candidate
	rc, err := util.LatestMatchingRelease(ctx, client, owner, repo, rcPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching latest RC: %v\n", err)
		os.Exit(1)
	}

	versions := []string{release.GetTagName(), rc.GetTagName()}

	// Return a version matrix that convolves all versions for the relevant components.
	// Results in a total of 16 tests (2^4)
	matrix := versionMatrix{
		CacheVersion:    versions,
		OriginVersion:   versions,
		RegistryVersion: versions,
		DirectorVersion: versions,
		ClientVersion:   versions[0:1],
	}

	out, err := json.Marshal(matrix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling matrix: %v\n", err)
		os.Exit(1)
	}

	// Print matrix to stdout, which GHAs can capture and use to define test runs
	fmt.Println(string(out))
}
