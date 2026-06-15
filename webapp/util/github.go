package util

import (
	"context"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

func NewGitHubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return github.NewClient(oauth2.NewClient(ctx, ts))
}
