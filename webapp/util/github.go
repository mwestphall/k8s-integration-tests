package util

import (
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v68/github"
)

// NewGitHubAppClient builds a GitHub client authenticated as a GitHub App
// installation. It signs requests with a JWT derived from appID and the
// PEM-encoded private key at privateKeyPath, then transparently exchanges
// that for a short-lived installation access token scoped to
// installationID, refreshing it as needed.
func NewGitHubAppClient(appID, installationID int64, privateKeyPath string) (*github.Client, error) {
	transport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, installationID, privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("creating GitHub App installation transport: %w", err)
	}
	return github.NewClient(&http.Client{Transport: transport}), nil
}
