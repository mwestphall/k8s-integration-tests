package test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

type pelicanFormatArgs struct {
	Tag string
}

var defaultFormatArgs pelicanFormatArgs = pelicanFormatArgs{
	Tag: "v7.22.0",
}

// PelicanTestContext holds all setup components needed to run the Pelican tests, and provides a convenient way to pass them around
type PelicanTestContext struct {
	TestHandle
	logDir                string
	cancelCtx             context.CancelFunc
	secretsManifest       string
	formattedKustomizeDir string
	namespace             string
	kubectlOptions        *k8s.KubectlOptions
}

// subtestGetDataFromOrigin checks that the Pelican CLI tools in the dev pod
// can fetch data from the origin pod
func subtestGetDataFromOrigin(th TestHandle) {
	devPod := th.getPodNameByLabel("app.kubernetes.io/name=dev")
	// Check that condor_status filtered on the EP's name returns a non-empty string
	cmd := "pelican object get pelican://director:8444/public/data/0.0 /dev/null"
	th.waitUntilPodExecSucceeds(devPod, "", cmd, TWO_MINUTES, zeroExitCode)
}



func setupPelicanTestSpace(t *testing.T) *PelicanTestContext {
	// -----------------------
	// Test environment setup
	// -----------------------

	// Define a test namespace name for the test
	namespace := "test-pelican-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	// Create a logging dir for the test output, fail fast if we can't make it
	kustomizeDir := "../manifests/pelican"
	logDir := th.makeLogDir(kustomizeDir)

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)

	// bind mount the origin's test data into minikube
	ctx, cancelCtx := context.WithCancel(context.Background())
	th.minikubeBindMount(ctx, "../data/pelican", "/data")

	// Create secrets for the pelican services: cert + signing keys
	// TODO OIDC secrets and web UI password are cargo culted from Brian A's repo, their values
	// have no meaning
	secretsManifest := th.applyPelicanSecrets(
		"Placeholder for the registry.",
		"Placeholder for the registry.",
		// Generated using `htpasswd -nbB -C 10 admin asdf`.
		"admin:$2y$10$ONeUS/VGwL9CoAD6pyZ2kusUjX8z0Sxuf8kz2g4PGbFb1GKUQ9J3C")

	// Template the kustomize dir
	th.fillTemplateStructFromEnv(&defaultFormatArgs, "PELICAN_")
	formattedKustomizeDir := th.formatKustomizeDir(kustomizeDir, defaultFormatArgs)
	k8s.KubectlApplyFromKustomize(t, options, formattedKustomizeDir)

	return &PelicanTestContext{
		TestHandle:            th,
		logDir:                logDir,
		cancelCtx:             cancelCtx,
		secretsManifest:       secretsManifest,
		formattedKustomizeDir: formattedKustomizeDir,
		namespace:             namespace,
		kubectlOptions:        options,
	}
}

func cleanupPelicanTestSpace(setup *PelicanTestContext) {
	setup.dumpPodInformation(setup.logDir)
	setup.deletePelicanSecrets(setup.secretsManifest)
	k8s.KubectlDeleteFromKustomize(setup.T, setup.kubectlOptions, setup.formattedKustomizeDir)
	k8s.DeleteNamespace(setup.T, setup.kubectlOptions, setup.namespace)
	setup.cancelCtx()
	os.RemoveAll(setup.formattedKustomizeDir)
}

func TestPelican(t *testing.T) {

	testContext := setupPelicanTestSpace(t)

	// --------------------------
	// Test environment teardown
	// --------------------------

	// Cleanup runs all the reciporical functions that delete created resources
	t.Cleanup(func() {
		cleanupPelicanTestSpace(testContext)
	})

	// -------------
	// Actual tests
	// -------------

	// First test: Confirm that the kustomized resources pass their liveness/health checks
	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		testContext.waitUntilAllDeploymentsReady(SIX_MINUTES)
	})

	if t.Failed() {
		return
	}

	// Second test: Run a basic pelican object get
	t.Run("Confirm public `pelican object get` succeeds", func(t *testing.T) {
		subtestGetDataFromOrigin(testContext.TestHandle)
	})

	
}
