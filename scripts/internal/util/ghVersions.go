package util

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/go-github/v68/github"
)

// LatestMatchingRelease returns the most recently published release in owner/repo
// whose tag name matches pattern. Draft releases are skipped. Returns an error if
// no matching release is found or the pattern is invalid.
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

	opts := &github.ListOptions{PerPage: 100}
	var latest *github.RepositoryRelease

	for {
		releases, resp, err := client.Repositories.ListReleases(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("listing releases for %s/%s: %w", owner, repo, err)
		}

		for _, r := range releases {
			if r.GetDraft() {
				continue
			}
			if !re.MatchString(r.GetTagName()) {
				continue
			}
			if r.PublishedAt == nil {
				continue
			}
			if latest == nil || r.PublishedAt.After(latest.PublishedAt.Time) {
				latest = r
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	if latest == nil {
		return nil, fmt.Errorf("no release matching %q found in %s/%s", pattern, owner, repo)
	}
	return latest, nil
}
