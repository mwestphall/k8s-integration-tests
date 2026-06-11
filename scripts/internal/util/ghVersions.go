package util

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/go-github/v68/github"
)

// LatestMatchingRelease returns the most recently created release in owner/repo
// whose tag name matches pattern. Draft releases are skipped. Returns an error if
// no matching release is found on the first page of results or the pattern is invalid.
//
// The GitHub API returns releases newest-first, so the first non-draft match is
// the latest.
//
// Pass a nil client to use an unauthenticated client (60 req/hr rate limit).
// For CI use, construct an authenticated client via:
//
//	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
//	client := github.NewClient(oauth2.NewClient(ctx, ts))
func LatestMatchingRelease(ctx context.Context, client *github.Client, owner, repo, pattern string) (*github.RepositoryRelease, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	if client == nil {
		client = github.NewClient(nil)
	}

	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("listing releases for %s/%s: %w", owner, repo, err)
	}

	for _, r := range releases {
		if !r.GetDraft() && re.MatchString(r.GetTagName()) {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no release matching %q found in %s/%s", pattern, owner, repo)
}
