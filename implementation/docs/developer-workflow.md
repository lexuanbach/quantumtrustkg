# Developer Workflow

The controller workspace now includes a minimal `Makefile`.

Common commands:

- `make test`
- `make build`
- `make run`
- `make fmt`
- `make tidy`

Notes:

- the controller tests exercise validation, QTPO, fail-closed runtime paths,
  mesh synthesis, staged rollout, and generated scale fixtures,
- Fuseki/Istio integration is represented by controller clients, manifests, and
  fixture-backed checks in this artifact,
- PQ data-plane negotiation remains out of scope for the current milestone.
