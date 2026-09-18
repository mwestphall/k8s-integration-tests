# Test-runner image for the adstash test suite: HTCondor CLI tools and python
# bindings, plus pytest and the elasticsearch-py/opensearch-py client libraries
# used by the tests in test-configs/adstash/tests/. ES_PY_VERSION/OS_PY_VERSION
# select which client library versions to install.
FROM htcondor/mini:latest

ARG ES_PY_VERSION="elasticsearch>=7,<8"
ARG OS_PY_VERSION="opensearch-py>=2,<3"

RUN pip3 install --no-cache-dir \
    "pytest" \
    "${ES_PY_VERSION}" \
    "${OS_PY_VERSION}"

# Upgrade condor_adstash to a build that supports --init_index.
RUN dnf -y --enablerepo=htcondor-snapshot upgrade condor \
    && dnf clean all

# submituser is a regular user account provided by htcondor/mini, used here
# so that jobs submitted by the test suite aren't owned by root.
USER submituser
CMD ["sleep", "infinity"]
