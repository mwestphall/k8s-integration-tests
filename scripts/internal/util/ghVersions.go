package util

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

type Comparator func(*github.RepositoryRelease, *github.RepositoryRelease) int

// LatestMatchingRelease returns the most recently created release in `owner`/`repo`
// whose tag name matches `pattern`. Draft releases are skipped. Returns an error if
// no matching release is found on the first page of results or the pattern is invalid.
func LatestMatchingRelease(ctx context.Context, client *github.Client, owner, repo string, re *regexp.Regexp, cmp Comparator) (*github.RepositoryRelease, error) {
	// Confirm given tag regexp is valid

	// Get the first page of releases (sorted by creation date descending)
	// We assume that all relevant results are on the first page here, should usually be true
	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("listing releases for %s/%s: %w", owner, repo, err)
	}

	// Find all releases in the list that matches the given pattern
	candidates := make([]*github.RepositoryRelease, 0)
	for _, r := range releases {
		if !r.GetDraft() && re.MatchString(r.GetTagName()) {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) > 0 {
		slices.SortFunc(candidates, cmp)
		return candidates[len(candidates)-1], nil
	}

	return nil, fmt.Errorf("no release matching %q found in %s/%s", re.String(), owner, repo)
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
