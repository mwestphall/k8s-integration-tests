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
In a given run, jobs are named `<test suite> / <test name>`, and should be grouped by `test suite` in the web viewer.


Views
=====

1. **Landing Page**: Displays a list of the 20 most recent runs of `run-tests.yaml` showing the date, and the status (success count, fail count) of each run,
    grouped by test suite. Each run should be clickable, leading to a detailed view of that run. No pagination is required.
2. **Run Details Page**: For a specific run, display the status of each test suite (set of jobs with the same prefix), and the status of each individual test (job).
    A suite's status is "success" only if every job in the suite succeeded; otherwise it is "failure".
    Clicking into a test should link to a detailed view of that test.
3. **Test Details View**: For a specific test job:
    * Link to the Github Actions page of that test. 
    * Include a per-Pod overview of the test results, populated via the artifacts produced for that job.
      **Artifact-to-job correlation**: Each job includes a step named
      `Upload Test Logs for <hash>`. Artifacts are named `<suite>-<hash>`. To match an artifact
      to a job, extract the hash from that step name and perform an exact match against the artifact
      named `<suite>-<hash>`, where `<suite>` is the part of the job name before ` / `.
      The artifact for each job is a zip file containing the following files for each pod that was tested:
        * `<pod-name>.events`: The list of Kubernetes events that occured for that pod during the test run.
        * `<pod-name>_<container-name>.log`: The logs for the container in that pod during the test run.
      Note that each test may have multiple pods, and each pod may have multiple containers, so the number of files in the artifact is not fixed. 
      The web viewer should be able to handle this dynamic structure when displaying the results. Also note that the test results place the zip file artifacts
      into a variable top-level directory inside the zip file, which will need to be examined during runtime.
    * The name of each pod and its pod events should be shown in the Test Details View, and a link to the logs for each container in that pod should be provided. 
      
4. **Logs View**: For a specific container in a specific pod for a specific test, display the logs for that container as plain preformatted text (inside a `<pre>` element).

Code Structure
==============
The code will be organized into the following packages:
1. `webapp/main`: The entrypoint for the application
2. `webapp/util`: Utility functions for interacting with the GitHub API
3. `webapp/handlers`: HTTP handlers for the different views in the application.
4. `webapp/templates`: HTML templates for the different views in the application.
5. `webapp/static`: Static files (CSS, JS) for the application.
