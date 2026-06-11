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

	release, err := util.LatestMatchingRelease(ctx, client, owner, repo, releasePattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching latest release: %v\n", err)
		os.Exit(1)
	}

	rc, err := util.LatestMatchingRelease(ctx, client, owner, repo, rcPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching latest RC: %v\n", err)
		os.Exit(1)
	}

	versions := []string{release.GetTagName(), rc.GetTagName()}

	matrix := versionMatrix{
		CacheVersion:    versions,
		OriginVersion:   versions,
		RegistryVersion: versions,
		DirectorVersion: versions,
		ClientVersion:   versions,
	}

	out, err := json.Marshal(matrix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling matrix: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(out))
}
