#!/usr/bin/env sh
# Reconcile one fixture once, without a cluster.
#
# Usage: run-fixture.sh [FIXTURE], with FIXTURE a file name under
# implementation/semantics/fixtures (default finance-scenario.ttl). It starts
# cmd/manager with --fixture, which runs the QTPO loop once over the fixture edges,
# prints the decisions to the log and exits. The generated Istio manifests are
# written by the mesh applier in filesystem mode.
set -eu

FIXTURE="${1:-finance-scenario.ttl}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

cd "$ROOT/controller"
go run ./cmd/manager --fixture "$FIXTURE"
