# Test-runner image for the adstash test suite: HTCondor CLI tools and python
# bindings, plus pytest and the elasticsearch-py/opensearch-py client libraries
# used by the tests in test-configs/adstash/tests/. SE_PY_VERSION
# select which client library versions to install.
FROM htcondor/mini:latest

ARG SE_PY_VERSION="elasticsearch>=7,<8"

RUN pip3 install --no-cache-dir \
    "pytest" \
    "${SE_PY_VERSION}"

# Upgrade condor_adstash to a build that supports --init_index.
RUN dnf -y --enablerepo=htcondor-snapshot upgrade condor \
    && dnf clean all

# submituser is a regular user account provided by htcondor/mini, used here
# so that jobs submitted by the test suite aren't owned by root.
USER submituser
CMD ["sleep", "infinity"]
