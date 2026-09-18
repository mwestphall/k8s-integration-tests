#!/usr/bin/env -S pytest -v -s

"""
Basic connectivity and version check for the deployed search engine backend.
"""

import os
import socket
import time

import pytest
import elasticsearch
import opensearchpy


# The single search-engine backend deployed by manifests/adstash/search/deployment.yaml;
# SE_BACKEND/SE_HOST are set by manifests/adstash/runner/deployment.yaml to match.
_SE_MAJOR = {"es7": 7, "es8": 8, "es9": 9, "os2": 2, "os3": 3}
SE_BACKEND = os.environ.get("SE_BACKEND", "es8")
SE_HOST = os.environ.get("SE_HOST", "localhost:9200")
SE_CLIENT_TYPE = "opensearch" if SE_BACKEND.startswith("os") else "elasticsearch"
SE_MAJOR = _SE_MAJOR[SE_BACKEND]


def get_client():
    if SE_CLIENT_TYPE == "elasticsearch":
        return elasticsearch.Elasticsearch(f"http://{SE_HOST}")
    else:
        host, port = SE_HOST.split(":")
        return opensearchpy.OpenSearch(hosts=[{"host": host, "port": int(port)}])


@pytest.fixture
def client():
    host = SE_HOST.split(":")[0]
    try:
        socket.getaddrinfo(host, None)
    except socket.gaierror:
        pytest.skip(f"{SE_BACKEND} ({host}) not in DNS, container not running")

    c = get_client()
    # Wait up to 60s for the backend to become healthy
    for attempt in range(12):
        try:
            if c.ping():
                return c
        except Exception:
            pass
        time.sleep(5)
    pytest.skip(f"{SE_BACKEND} not reachable after 60s")


class TestConnectivity:
    """Verify the deployed backend is reachable and reports the expected major version."""

    def test_ping(self, client):
        assert client.ping()

    def test_cluster_health(self, client):
        health = client.cluster.health()
        assert health["status"] in ("green", "yellow")

    def test_major_version(self, client):
        info = client.info()
        version_str = info["version"]["number"]
        major = int(version_str.split(".")[0])
        assert major == SE_MAJOR
