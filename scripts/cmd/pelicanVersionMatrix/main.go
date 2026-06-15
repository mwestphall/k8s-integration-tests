// pelicanVersionMatrix generates a JSON matrix of Pelican versions for testing.
// It fetches the latest stable release and the latest release candidate (RC) from GitHub,
// and constructs a matrix that can be used in GitHub Actions to run tests against these versions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/osg-htc/k8s-integration-tests/scripts/internal/util"
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
	CacheVersion    string `json:"cacheVersion"`
	OriginVersion   string `json:"originVersion"`
	RegistryVersion string `json:"registryVersion"`
	DirectorVersion string `json:"directorVersion"`
	ClientVersion   string `json:"clientVersion"`
}

type versionIncluesMatrix struct {
	Include []versionMatrix `json:"include"`
}

func newVersionMatrix(baseTag string) versionMatrix {
	return versionMatrix{
		CacheVersion:    baseTag,
		OriginVersion:   baseTag,
		RegistryVersion: baseTag,
		DirectorVersion: baseTag,
		ClientVersion:   baseTag,
	}
}

func main() {
	ctx := context.Background()
	client := util.NewGitHubClient(ctx)

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

	relTag := release.GetTagName()
	rcTag := rc.GetTagName()

	// Return a version matrix that convolves versions for the relevant components.
	// Test sequence should be as follows:
	// 1. Test the release candidate across all components
	// 2. For each component, test the stable release of that component against other RCs to catch upgrade issues.
	fields := reflect.VisibleFields(reflect.TypeFor[versionMatrix]())
	includes := []versionMatrix{newVersionMatrix(rcTag)}
	for _, field := range fields {
		verMat := newVersionMatrix(rcTag)
		refMat := reflect.ValueOf(&verMat).Elem()
		refMat.FieldByName(field.Name).SetString(relTag)

		includes = append(includes, verMat)
	}

	out, err := json.Marshal(versionIncluesMatrix{Include: includes})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling matrix: %v\n", err)
		os.Exit(1)
	}

	// Print matrix to stdout, which GHAs can capture and use to define test runs
	fmt.Println(string(out))
}
