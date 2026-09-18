package test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

// adstashRunnerImage is the tag prefix for the test-runner image built by
// TestAdstash into minikube (see buildMinikubeImage in test_runner_utils.go).
const adstashRunnerImage = "condor-adstash-tests"

// adstashFormatArgs parameterizes the search-engine backend under test, the
// HTCondor image, and the test-runner image (which bakes in a specific
// elasticsearch-py/opensearch-py combo at build time).
type adstashFormatArgs struct {
	SEBackendKey string // es7 | es8 | es9 | os2 | os3 — selects the search-engine image/version and its env vars
	DBTag        string // search-engine image tag matching SEBackendKey's family
	CondorTag    string // htcondor/mini image tag
	RunnerTag    string // test-runner image tag (encodes the ES/OS client-lib combo baked into it)
	ESPyVersion  string // pip version constraint for elasticsearch-py, baked into the runner image at build time
	OSPyVersion  string // pip version constraint for opensearch-py, baked into the runner image at build time
}

var defaultAdstashFormatArgs = adstashFormatArgs{
	SEBackendKey: "es8",
	DBTag:        "8.19.20",
	CondorTag:    "lts",
	RunnerTag:    "es7py-os2py",
	ESPyVersion:  "elasticsearch>=7,<8",
	OSPyVersion:  "opensearch-py>=2,<3",
}

// TestAdstash runs the adstash test suite against the search-engine backend
// configured in defaultAdstashFormatArgs (overridable via ADSTASH_* env vars).
func TestAdstash(t *testing.T) {
	t.Parallel()
	kustomizeDir := "../manifests/adstash"

	namespace := "test-adstash-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	// Create a directory for log output
	logDir := th.makeLogDir(kustomizeDir)

	// create k8s namespace for the test
	k8s.CreateNamespace(t, options, namespace)

	// Template the kustomize dir
	th.fillTemplateStructFromEnv(&defaultAdstashFormatArgs, "ADSTASH_")

	// Build the test-runner image into minikube before applying manifests.
	th.buildMinikubeImage("../images/adstash",
		fmt.Sprintf("%v:%v", adstashRunnerImage, defaultAdstashFormatArgs.RunnerTag),
		map[string]string{
			"ES_PY_VERSION": defaultAdstashFormatArgs.ESPyVersion,
			"OS_PY_VERSION": defaultAdstashFormatArgs.OSPyVersion,
		})

	formattedKustomizeDir := th.formatKustomizeDir(kustomizeDir, defaultAdstashFormatArgs)

	// create k8s resources for the test
	k8s.KubectlApplyFromKustomize(t, options, formattedKustomizeDir)

	// defer deleting the k8s resources created for the test
	t.Cleanup(func() {
		th.dumpPodInformation(logDir)
		k8s.DeleteNamespace(t, options, namespace)
		k8s.KubectlDeleteFromKustomize(t, options, formattedKustomizeDir)
		os.RemoveAll(formattedKustomizeDir)
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		th := TestHandle{t, options}
		th.waitUntilAllDeploymentsReady(SIX_MINUTES)
	})

	// Bail early here if the deployments do not become live
	if t.Failed() {
		return
	}

	th.RunTestConfigDir("../test-configs/adstash")
}
