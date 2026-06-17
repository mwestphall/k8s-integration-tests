Overview
========
This repository runs tests for different OSG services running in Kubernetes via github actions test matrices.
We want to create a web viewer for the results of the tests, and this document is a plan for how to do that.

Technology Stack
================
The web viewer will consist of a Golang application serving static web pages via the `html/template` package.
Use plain CSS classes for styling, and use as little javascript as possible.
The application will read test results via the GitHub API (`github.com/google/go-github/v68/github`),
specifically the `Actions` API to get the workflow runs and their artifacts. Several views in the app depend on files 
contained in Github Actions artifacts. When implementing these views, download the relevant artifact once and extract 
to disk, rather than repeatedly downloading the artifact on each view that needs it.

Configuration
=============
The application is configured via environment variables:
- `GITHUB_TOKEN` (required): Personal access token used to authenticate with the GitHub API.
- `GITHUB_OWNER` (required): The GitHub organization or user that owns the repository (e.g. `osg-htc`).
- `GITHUB_REPO` (required): The repository name (e.g. `k8s-integration-tests`).
- `PORT` (optional, default `8080`): The port the HTTP server listens on.

Artifact Caching
================
Downloaded artifacts are extracted into a subdirectory of `os.TempDir()` named
`k8s-webapp-cache/<run-id>/<artifact-id>/`. Before downloading, the handler checks whether
the extraction directory already exists; if so, it reuses the cached files. No background
refresh or expiry logic is needed — GitHub Actions already expires artifacts after 5 days,
and runs older than that will not have downloadable artifacts.

Test Structure
==============
Tests are organized in GitHub Action workflows, with a single "Do Everything" workflow at `.github/workflows/run-tests.yaml` acting
as the Entrypoint to other tests. When enumerating actions in the repo to display, only this "Do Everything" test should be considered.
Within the "Do Everything" workflow, there are multiple jobs, each representing a different test suite (currently, 'ospool-ep' and 'pelican').
In a given run, jobs are named `<test suite> / <test name> / Run Tests`, and should be grouped by `test suite` in the web viewer.
Each job also contains a step named `Job Matrix: <json-encoded environment>` whose name encodes the key-value pairs for that matrix combination.


Views
=====

1. **Landing Page**: Displays a list of the 20 most recent runs of `run-tests.yaml` showing the date, and the status (success count, fail count) of each run,
    grouped by test suite. Each run should be clickable, leading to a detailed view of that run. No pagination is required.
2. **Run Details Page**: For a specific run, display the status of each test suite (set of jobs with the same prefix), and the status of each individual test (job).
    A suite's status is "success" only if every job in the suite succeeded; otherwise it is "failure".
    Each job is displayed using its human-readable test name (the middle segment of the job name, e.g. "Run Pelican tests"),
    with its matrix key-value pairs shown as chips beneath the name.
    Clicking into a test should link to a detailed view of that test.
3. **Test Details View**: For a specific test job:
    * Display the human-readable test name as the page title, with the matrix key-value pairs shown as chips beneath it.
    * Link to the Github Actions page of that test.
    * Include a per-Pod overview of the test results, populated via the artifacts produced for that job.
      **Artifact-to-job correlation**: Each job includes a step named
      `Upload Test Logs of <hash>`. Artifacts are named `<anything>-<hash>`. To match an artifact
      to a job, extract the hash from that step name and find the artifact whose name ends with
      `-<hash>`. The artifact name prefix is not tied to the suite name.
      The artifact for each job is a zip file containing the following files for each pod that was tested:
        * `<pod-name>.events`: The list of Kubernetes events that occured for that pod during the test run.
        * `<pod-name>_<container-name>.log`: The logs for the container in that pod during the test run.
      Note that each test may have multiple pods, and each pod may have multiple containers, so the number of files in the artifact is not fixed. 
      The web viewer should be able to handle this dynamic structure when displaying the results. Also note that the test results place the zip file artifacts
      into a variable top-level directory inside the zip file, which will need to be examined during runtime.
    * The name of each pod and its pod events should be shown in the Test Details View, and a link to the logs for each container in that pod should be provided. 
    * A test result summary, derived from the logs of the test, should be shown at the top of the page.
      * For jobs containing a step named `Test <Test Name>`, the logs for that test will contain several lines in the format `--- (PASS|FAIL): <Test Name> (test duration)`.
      * If a job contains the appropriate step, fetch its logs via the GitHub API and parse these lines to determine the pass/fail status of each test, and display a summary
        at the top of the page showing a table of the passing and failing tests.
      * For each row in the table, link to the appropriate Logs View (Test Logs) page for that sub-text. See the description of the Logs View (Test Logs) below 
        for details on how to determine the appropriate link for each test.
      * Cache the tests' logs as they're downloaded.
      
4. **Logs View (Pod Logs)**: For a specific container in a specific pod for a specific test, display the logs for that container as plain preformatted text (inside a `<pre>` element).

5. **Logs View (Test Logs)**: For a specific test job step (named `Test <Test Name>`), the output consists of Go test logs in the format of
   `<TestName> <TimeStamp> <test code line> <log message>`. When a user clicks in from a test name in the Test Details View, 
   they should be taken to a view showing the logs for that test. The logs for all tests come from the same `Test <Test Name>` step, so they can be fetched together. 
   The viewer should filter the logs to only show lines relevant to the test that was clicked on (i.e. lines whose `<TestName>` matches the test that was clicked on).
   Display the tests in a `<pre>` element, as in the Pod Logs view.

Code Structure
==============
The code will be organized into the following packages:
1. `webapp/main`: The entrypoint for the application
2. `webapp/util`: Utility functions for interacting with the GitHub API
3. `webapp/handlers`: HTTP handlers for the different views in the application.
4. `webapp/templates`: HTML templates for the different views in the application.
5. `webapp/static`: Static files (CSS, JS) for the application.
