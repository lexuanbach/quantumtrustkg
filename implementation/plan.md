# Implementation Plan

## Goal

This plan translates the current QuantumTrustKG paper into a concrete engineering roadmap. The implementation goal is to realize a semantic orchestration framework for quantum-safe microservices that is consistent with the paper's narrative, evaluation claims, and future extension points.

## Scope

The implementation should cover:

1. a Kubernetes-native controller,
2. a graph-backed semantic control plane,
3. protocol-selection and migration logic,
4. policy-to-mesh synthesis,
5. evaluation assets and reproducibility artifacts.

The implementation does not need to deliver full production-grade PQC data-plane support in the first phase. It does need to support a credible prototype that can:

- maintain semantic state,
- compute admissible protocol assignments,
- push service-mesh configuration,
- stage migration and rollback,
- generate evidence for the paper.

## Paper Coverage Checklist

This section tracks the current implementation status of the main features claimed in the paper.

Status labels:

- `Implemented`: present in code and covered at least at scaffold or test level
- `Partial`: represented in design or stubs, but not fully integrated
- `Missing`: not yet implemented in a meaningful way

### Semantic Control Plane

- `Implemented`: edge-level semantic model with trust boundary, policy context, provenance, and assignment state
- `Implemented`: service-to-policy derivation from semantic fixtures
- `Implemented`: trust-boundary-aware policy propagation
- `Implemented`: endpoint-supported protocol candidate filtering
- `Implemented`: stale-fact blocking
- `Implemented`: conflicting-fact blocking
- `Partial`: SHACL-like constraint files exist, but no SHACL execution engine is wired into the runtime
- `Partial`: semantic graph is modeled in Turtle and queries exist, but the runtime still uses lightweight fixture parsing rather than a real RDF stack

### QTPO and Orchestration

- `Implemented`: local QTPO-style selection pipeline
- `Implemented`: policy filtering
- `Implemented`: threat filtering as a deprecated-profile blacklist
- `Implemented`: hybrid fallback path
- `Partial`: threat model is minimal and does not yet reflect the richer graph-based restriction model described in the paper
- `Partial`: ranking is overhead-based only and does not yet include disruption or stability scoring
- `Missing`: global or graph-wide optimization baseline

### Controller Behavior

- `Implemented`: structured assignment status tracking
- `Implemented`: structured blocked and promoted outcomes
- `Implemented`: multi-edge fixture reconciliation path
- `Implemented`: controller entrypoint supports demo and named fixture reconciliation
- `Partial`: reconcilers exist as skeletons but do not watch Kubernetes resources
- `Missing`: real controller-runtime manager and watch registration
- `Missing`: generation-based reconciliation and concurrent update handling

### Kubernetes and CRD Integration

- `Implemented`: CRD definitions for policy, capability, migration, and assignment status
- `Implemented`: JSON schemas for core resources
- `Implemented`: RBAC, namespace, controller deployment, and Fuseki deployment manifests exist
- `Missing`: actual CRD ingestion from a running cluster
- `Missing`: status writeback to Kubernetes custom resources

### Graph Store Integration

- `Implemented`: graph client issues Fuseki-compatible SPARQL query and update requests
- `Partial`: ontology, constraints, queries, and fixtures exist
- `Partial`: runtime still relies on fixture parsing rather than live SPARQL query results for assignment decisions
- `Missing`: graph write synchronization from observed cluster state

### Mesh Enforcement

- `Implemented`: mesh synthesis produces concrete `PeerAuthentication` and `DestinationRule` manifests
- `Partial`: apply path validates manifests before a future Kubernetes client-backed write
- `Missing`: apply to Kubernetes or Istio APIs
- `Missing`: observed active mesh-state verification

### Migration and Rollout

- `Partial`: assignment states and blocked/promoted transitions exist
- `Partial`: migration-related CRD and rollout placeholders exist
- `Missing`: staged rollout sequencing
- `Missing`: rollback triggers from runtime health signals
- `Missing`: dependency-aware ordering and canary promotion

### Observation and Capability Ingestion

- `Partial`: capability schema and types exist
- `Partial`: fixture-based capability ingestion exists
- `Missing`: service metadata ingestion from Kubernetes
- `Missing`: sidecar or gateway capability observation
- `Missing`: workload identity and provenance source diversity beyond fixture metadata

### Evaluation and Experimentation

- `Implemented`: unit tests for semantic parsing, validation, and runtime reconciliation
- `Partial`: finance fixture and failure fixtures exist
- `Missing`: 50/100/200-service generated workloads
- `Missing`: 500-service scaling fixture generation
- `Partial`: baseline configuration files and a fixture runner script exist
- `Missing`: metrics collection and output generation

### Case Study Alignment

- `Implemented`: finance-oriented semantic fixture and reconciliation path
- `Partial`: the fixture mirrors the paper's financial scenario at small scale
- `Missing`: larger case-study workload resembling the 85-service narrative in the paper
- `Missing`: measured case-study outputs tied to the manuscript claims

### Highest-Priority Missing Features

1. Real Kubernetes controller-runtime integration and CRD watches
2. Runtime use of live Fuseki query results instead of fixture-only parsing
3. Kubernetes or Istio API-backed apply and verification flow
4. Staged migration and rollback orchestration
5. Scaled experiment generation, metrics capture, and baseline execution

## Recommended Folder Structure

The following structure is intended to separate controller logic, semantic assets, deployment manifests, and experiments clearly.

```text
implementation/
├── plan.md
├── README.md
├── docs/
│   ├── architecture.md
│   ├── ontology.md
│   ├── controller-flow.md
│   ├── evaluation.md
│   └── reproducibility.md
├── api/
│   ├── crds/
│   │   ├── quantumsecuritypolicies.yaml
│   │   ├── cryptocapabilityprofiles.yaml
│   │   ├── migrationplans.yaml
│   │   └── assignmentstatuses.yaml
│   └── schemas/
│       ├── policy.schema.json
│       ├── capability.schema.json
│       └── assignment.schema.json
├── controller/
│   ├── cmd/
│   │   └── manager/
│   │       └── main.go
│   ├── internal/
│   │   ├── controllers/
│   │   │   ├── policy_controller.go
│   │   │   ├── capability_controller.go
│   │   │   ├── assignment_controller.go
│   │   │   └── migration_controller.go
│   │   ├── graph/
│   │   │   ├── client.go
│   │   │   ├── model.go
│   │   │   ├── writers.go
│   │   │   ├── queries.go
│   │   │   └── validation.go
│   │   ├── orchestration/
│   │   │   ├── qtpo.go
│   │   │   ├── policy_filter.go
│   │   │   ├── threat_filter.go
│   │   │   ├── fallback.go
│   │   │   ├── ranking.go
│   │   │   └── rollout.go
│   │   ├── mesh/
│   │   │   ├── synthesize.go
│   │   │   ├── peerauth.go
│   │   │   ├── destinationrule.go
│   │   │   └── apply.go
│   │   ├── state/
│   │   │   ├── edge_state.go
│   │   │   ├── assignment_state.go
│   │   │   └── provenance.go
│   │   └── observability/
│   │       ├── metrics.go
│   │       ├── events.go
│   │       └── logging.go
│   ├── pkg/
│   │   └── types/
│   │       ├── policy.go
│   │       ├── capability.go
│   │       ├── protocol.go
│   │       └── edge.go
│   └── tests/
│       ├── unit/
│       ├── integration/
│       └── e2e/
├── semantics/
│   ├── ontology/
│   │   ├── quantumtrustkg.ttl
│   │   ├── classes.ttl
│   │   ├── properties.ttl
│   │   └── examples.ttl
│   ├── constraints/
│   │   ├── assignment.shacl.ttl
│   │   ├── policy.shacl.ttl
│   │   └── provenance.shacl.ttl
│   ├── queries/
│   │   ├── candidate_protocols.rq
│   │   ├── active_policies.rq
│   │   ├── stale_facts.rq
│   │   ├── migration_edges.rq
│   │   └── invalid_assignments.rq
│   └── fixtures/
│       ├── small-topology.ttl
│       ├── finance-scenario.ttl
│       └── scale-500.ttl
├── deploy/
│   ├── base/
│   │   ├── namespace.yaml
│   │   ├── serviceaccount.yaml
│   │   ├── rbac.yaml
│   │   ├── manager.yaml
│   │   └── fuseki.yaml
│   ├── overlays/
│   │   ├── dev/
│   │   ├── eval/
│   │   └── demo/
│   ├── istio/
│   │   ├── templates/
│   │   ├── generated/
│   │   └── examples/
│   └── scripts/
│       ├── install.sh
│       ├── reset.sh
│       └── port-forward.sh
├── scenarios/
│   ├── topology/
│   │   ├── frontend-backend/
│   │   ├── event-driven/
│   │   └── multitier/
│   ├── policies/
│   │   ├── pci/
│   │   ├── cross-zone/
│   │   └── deprecation/
│   ├── capabilities/
│   │   ├── classical-only/
│   │   ├── hybrid/
│   │   └── pqc/
│   └── failures/
│       ├── stale-facts/
│       ├── unsupported-peer/
│       └── rollback/
├── experiments/
│   ├── runners/
│   │   ├── run_compliance.sh
│   │   ├── run_latency.sh
│   │   ├── run_scaling.sh
│   │   └── run_migration.sh
│   ├── configs/
│   │   ├── exp-50.yaml
│   │   ├── exp-100.yaml
│   │   ├── exp-200.yaml
│   │   └── exp-500.yaml
│   ├── outputs/
│   │   ├── raw/
│   │   ├── processed/
│   │   └── figures/
│   └── notebooks/
│       ├── compliance.ipynb
│       ├── latency.ipynb
│       └── scaling.ipynb
└── tools/
    ├── generators/
    ├── converters/
    └── validators/
```

## Folder Responsibilities

### `docs/`

Purpose:

- capture design rationale,
- explain architecture,
- document ontology decisions,
- describe how experiments map to the paper.

Must contain:

- one high-level architecture document,
- one ontology document,
- one controller-flow document,
- one evaluation/reproducibility document.

### `api/`

Purpose:

- define the user-facing control-plane contract.

This folder should hold:

- CRDs for policy, capability, migration, and status,
- JSON schemas for validation,
- versioned API definitions if the prototype evolves.

### `controller/`

Purpose:

- implement the Kubernetes-native control plane.

Internal ownership:

- `controllers/`: watch resources and trigger reconciliation,
- `graph/`: write semantic state and query graph facts,
- `orchestration/`: implement QTPO and rollout logic,
- `mesh/`: synthesize and apply Istio resources,
- `state/`: maintain desired, active, blocked, and rollback state,
- `observability/`: metrics, events, logs.

### `semantics/`

Purpose:

- store the semantic assets independently from controller code.

This is where to place:

- ontology files,
- constraints,
- SPARQL queries,
- sample graph fixtures for experiments.

This separation is important because it lets the semantic layer be inspected, tested, and cited independently from the controller implementation.

### `deploy/`

Purpose:

- make deployment reproducible across dev, evaluation, and demo environments.

This folder should include:

- base manifests,
- overlays for specific environments,
- generated or templated Istio output examples,
- operational helper scripts.

### `scenarios/`

Purpose:

- hold reusable input scenarios for experiments and demos.

Scenario assets should define:

- topology,
- capability profiles,
- policy sets,
- failure injections.

### `experiments/`

Purpose:

- make the evaluation rerunnable.

This folder should isolate:

- experiment runner scripts,
- configs for each scale,
- outputs,
- plotting or notebook logic.

## Detailed Work Plan

## Phase 1: Foundations

### 1.1 Define API and State Model

Design the core resources first.

Required CRDs:

- `QuantumSecurityPolicy`
- `CryptoCapabilityProfile`
- `MigrationPlan`
- `AssignmentStatus`

Each resource should define:

- desired fields,
- validation rules,
- status fields,
- reconciliation triggers.

Expected outputs:

- CRD YAML files in `api/crds/`
- internal Go or typed model structs in `controller/pkg/types/`

### 1.2 Define Edge-State Model

Create a single internal representation for communication edges.

Required fields:

- source workload
- destination workload
- namespace pair
- trust boundary
- active policy set
- candidate profile set
- desired assignment
- active assignment
- rollout phase
- provenance metadata
- last-evaluated timestamp

This is the key runtime abstraction that should connect the paper's semantic model to the controller logic.

## Phase 2: Semantic Layer

### 2.1 Ontology Implementation

Implement the ontology in Turtle.

Minimum classes:

- `:Service`
- `:Communication`
- `:Algorithm`
- `:Protocol`
- `:Policy`
- `:Configuration`
- `:Observation`

Minimum properties:

- `:supports`
- `:dependsOn`
- `:subjectTo`
- `:requiresLevel`
- `:crossesBoundary`
- `:assignedProtocol`
- `:hasDesiredAssignment`
- `:hasActiveAssignment`
- `:observedBy`
- `:observedAt`
- `:freshUntil`

Outputs:

- `semantics/ontology/quantumtrustkg.ttl`
- example fixtures in `semantics/fixtures/`

### 2.2 Constraint Layer

Add lightweight validation rules.

Priority constraints:

1. a `quantum_safe` assignment cannot use a classical primitive,
2. a regulated edge must satisfy the required minimum security level,
3. an assigned protocol must be supported by both endpoints,
4. a stale capability observation must invalidate assignment admissibility,
5. an edge in blocked state must not be rendered as active mesh configuration.

Implementation options:

- SHACL files in `semantics/constraints/`
- equivalent internal validation code in `controller/internal/graph/validation.go`

Recommended approach:

- keep the normative rules in SHACL-like files,
- mirror critical runtime checks in controller code.

### 2.3 Core Queries

Implement reusable queries for:

- feasible candidate discovery,
- active policy lookup,
- stale fact detection,
- affected-edge recomputation,
- invalid assignment detection,
- migration candidate discovery.

Outputs:

- `.rq` files in `semantics/queries/`
- query-wrapper functions in `controller/internal/graph/queries.go`

## Phase 3: Controller Implementation

### 3.1 Watchers and Event Sources

The controller must react to:

- service create/update/delete,
- capability profile changes,
- policy changes,
- migration plan changes,
- graph validation failures,
- rollout health signals.

Each event should map to:

- graph update,
- edge impact analysis,
- orchestration run,
- synthesis/apply phase,
- status update.

### 3.2 Reconciliation Flow

Recommended flow:

1. receive event,
2. normalize resource state,
3. update graph facts,
4. determine impacted edges,
5. run graph validation,
6. run QTPO on impacted edges,
7. generate desired mesh configuration,
8. stage rollout,
9. verify active state,
10. update status and metrics.

### 3.3 Failure Handling

The controller must fail closed on:

- stale capability facts,
- contradictory endpoint capabilities,
- unsatisfied policy constraints,
- invalid generated assignments,
- rollout verification mismatch.

The controller should emit:

- Kubernetes events,
- structured logs,
- metrics labels for failure cause.

## Phase 4: QTPO and Orchestration Logic

### 4.1 Candidate Discovery

Input:

- source and destination edge context,
- endpoint capability profiles,
- graph protocol definitions,
- active policy set,
- threat restrictions.

Output:

- admissible candidate set,
- blocked reason if empty.

### 4.2 Filter Pipeline

The filter pipeline should be implemented as explicit stages:

1. endpoint compatibility filter,
2. policy filter,
3. threat/deprecation filter,
4. provenance/freshness filter,
5. fallback constructor,
6. ranking stage.

Each stage should produce:

- filtered candidate set,
- explanation metadata,
- optional failure code.

This is important for paper-aligned explainability.

### 4.3 Ranking

Ranking factors:

- estimated overhead,
- security category preference,
- migration stability,
- rollout disruption risk.

First prototype recommendation:

- lexicographic priority:
  - admissibility,
  - required security category,
  - lower overhead,
  - lower disruption score.

### 4.4 Fallback and Blocking

If no pure PQC profile is feasible:

- attempt hybrid construction,
- if hybrid is feasible, emit staged-migration assignment,
- if not feasible, mark edge as blocked and emit remediation reason.

Possible remediation tags:

- `legacy-peer`
- `stale-facts`
- `policy-conflict`
- `unsupported-profile`

## Phase 5: Mesh Synthesis

### 5.1 Translation Contract

Define a deterministic mapping from semantic assignments to mesh resources.

Examples:

- `quantum_safe` assignment
  - strict mutual TLS
  - specific credential bundle
  - dedicated `DestinationRule`
- `hybrid` assignment
  - transitional credentials
  - rollout subset
  - staged routing annotation

### 5.2 Resource Generation

Generate:

- `PeerAuthentication`
- `DestinationRule`
- optional subset routing resources
- rollout annotations and status markers

Each generated resource should include:

- assignment ID,
- source edge reference,
- protocol profile label,
- generation timestamp.

### 5.3 Apply and Verify

After applying resources:

- read back active mesh state,
- compare desired vs active assignment,
- mark success, uncertain, or rollback-needed.

## Phase 6: Migration Engine

### 6.1 Rollout State Machine

Recommended states:

- `Pending`
- `Validated`
- `Applying`
- `Canary`
- `Promoted`
- `Blocked`
- `Rollback`
- `Failed`

### 6.2 Trigger Sources

Migration recalculation should happen on:

- capability upgrades,
- policy tightening,
- deprecation advisories,
- trust-zone changes,
- endpoint replacement,
- stale-fact invalidation.

### 6.3 Rollback Logic

Rollback conditions:

- request failure spike,
- latency threshold violation,
- mesh state mismatch,
- policy non-compliance,
- unsupported peer behavior.

## Phase 7: Evaluation Implementation

### 7.1 Testbed Construction

Prepare scenario generators for:

- 50 services,
- 100 services,
- 200 services,
- optional 500-service semantic scaling run.

Each generated environment should vary:

- service capability mix,
- topology,
- trust-zone distribution,
- compliance policy density.

### 7.2 Baseline Realization

Implement:

- `Static Istio`
  - fixed protocol choice,
- `OPA-guided`
  - policy checks without graph-backed state,
- `Manual rollout`
  - scripted static edge assignments,
- `Greedy non-KG`
  - flattened metadata, no semantic edge model.

### 7.3 Metrics Instrumentation

Required metric groups:

- orchestration quality:
  - compliance rate
  - failed assignments
  - rollback count
- latency:
  - query latency
  - rule latency
  - reconciliation latency
  - p95 controller latency
- resource usage:
  - controller CPU/memory
  - triplestore CPU/memory
- migration:
  - rollout duration
  - downtime events
  - latency delta during migration

### 7.4 Scenario Matrix

Run at minimum:

1. initial deployment,
2. policy change,
3. capability upgrade,
4. deprecation advisory,
5. stale-fact injection,
6. unsupported-peer failure,
7. burst update batch,
8. rollback under elevated error rate.

## Phase 8: Documentation and Reproducibility

Required docs:

- `docs/architecture.md`
- `docs/ontology.md`
- `docs/controller-flow.md`
- `docs/evaluation.md`
- `docs/reproducibility.md`

Minimum reproducibility bundle:

- deployment manifests,
- scenario configs,
- ontology files,
- query files,
- experiment runners,
- processed output and figure scripts.

## Milestones

### Milestone 1: Core Repository Skeleton

- folder structure created
- CRDs drafted
- controller bootstrap added
- ontology scaffold added

### Milestone 2: Semantic Control Plane

- graph writes working
- queries working
- edge-level model implemented
- provenance fields supported

### Milestone 3: Orchestration Pipeline

- QTPO implemented
- policy, threat, and provenance filters working
- fallback behavior tested

### Milestone 4: Mesh Integration

- Istio synthesis working
- assignment apply/verify loop working
- rollout state machine implemented

### Milestone 5: Evaluation and Paper Alignment

- baselines runnable
- core figures reproducible
- appendix artifacts generated
- implementation evidence aligned with manuscript claims

## Immediate Next Steps

1. Create the folder skeleton under `implementation/`.
2. Draft CRDs and internal edge-state structs first.
3. Write the ontology and the first three SPARQL queries:
   - candidate discovery,
   - active policies,
   - stale facts.
4. Implement the controller reconciliation skeleton.
5. Build a single small end-to-end scenario before scaling to evaluation runs.
