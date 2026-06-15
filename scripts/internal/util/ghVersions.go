package util

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// LatestMatchingRelease returns the most recently created release in `owner`/`repo`
// whose tag name matches `pattern`. Draft releases are skipped. Returns an error if
// no matching release is found on the first page of results or the pattern is invalid.
func LatestMatchingRelease(ctx context.Context, client *github.Client, owner, repo, pattern string) (*github.RepositoryRelease, error) {
	// Confirm given tag regexp is valid
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	// Get the first page of releases (sorted by creation date descending)
	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("listing releases for %s/%s: %w", owner, repo, err)
	}

	// Return the first releaes in the list that matches the given pattern
	for _, r := range releases {
		if !r.GetDraft() && re.MatchString(r.GetTagName()) {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no release matching %q found in %s/%s", pattern, owner, repo)
}

// newGitHubClient creates a GitHub client, based on the `GITHUB_TOKEN` environment variable.
// This is set by default inside GHAs.
func NewGitHubClient(ctx context.Context) *github.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return github.NewClient(nil)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return github.NewClient(oauth2.NewClient(ctx, ts))
}
