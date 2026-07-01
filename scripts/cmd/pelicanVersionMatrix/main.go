// pelicanVersionMatrix generates a JSON matrix of Pelican versions for testing.
// It fetches the latest stable release and the latest release candidate (RC) from GitHub,
// and constructs a matrix that can be used in GitHub Actions to run tests against these versions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"

	"github.com/google/go-github/v68/github"
	"github.com/osg-htc/k8s-integration-tests/scripts/internal/util"
	"golang.org/x/mod/semver"
)

const (
	owner = "PelicanPlatform"
	repo  = "pelican"
)

var (
	releasePattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	rcPattern      = regexp.MustCompile(`^(v\d+\.\d+\.\d+)-rc\.(\d+)$`)
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

// semVerCmp compares two releases on the assumption that their tag names are SemVers
// Returns 0 if either tag is not a SemVer
func semVerCmp(r1, r2 *github.RepositoryRelease) int {
	return semver.Compare(r1.GetTagName(), r2.GetTagName())
}

type rcSemVer struct {
	semver string
	rc     int
}

func parseRcMatch(r string) (ver rcSemVer, err error) {
	matches := rcPattern.FindStringSubmatch(r)
	if len(matches) != 3 {
		return ver, fmt.Errorf("rc tag %v does not match pattern %v", r, rcPattern.String())
	}

	if !semver.IsValid(matches[1]) {
		return ver, fmt.Errorf("rc tag %v has invalid semver %v", r, matches[1])
	}

	rcVer, err := strconv.Atoi(matches[2])
	if err != nil {
		return ver, err
	}
	return rcSemVer{matches[1], rcVer}, nil
}

// rcSemVerCmp compares two releases on the assumption that their tag names are formatted as:
// `<semver>-rc.<release candidate number>`. Compares by semver and then by rc number
func rcSemVerCmp(r1, r2 *github.RepositoryRelease) int {
	r1Ver, err := parseRcMatch(r1.GetTagName())
	r2Ver, err2 := parseRcMatch(r2.GetTagName())
	if errs := errors.Join(err, err2); errs != nil {
		return 0
	}
	if semVerCmp := semver.Compare(r1Ver.semver, r2Ver.semver); semVerCmp != 0 {
		return semVerCmp
	}

	return r1Ver.rc - r2Ver.rc
}

func main() {
	ctx := context.Background()
	client := util.NewGitHubClient(ctx)

	// Find the latest stable release
	release, err := util.LatestMatchingRelease(ctx, client, owner, repo, releasePattern, semVerCmp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching latest release: %v\n", err)
		os.Exit(1)
	}

	// Find the latest release candidate
	rc, err := util.LatestMatchingRelease(ctx, client, owner, repo, rcPattern, rcSemVerCmp)
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
