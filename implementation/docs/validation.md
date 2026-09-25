# Validation Notes

The current validation layer is split into two parts:

1. semantic constraints in `semantics/constraints/`
2. runtime admissibility checks in controller code

The semantic constraints are intentionally lightweight and model:

- assignment cardinality,
- valid policy security categories,
- presence of provenance timestamps.

Runtime validation mirrors the promotion-critical constraints in controller code
using explicit Go checks and SPARQL query results, so rollout decisions fail
closed even when SHACL validation is not executed as a separate pipeline step.
The SHACL files should be treated as artifact validation aids for fixtures and
ontology examples, not as the runtime semantics or as a deep OWL/SHACL
reasoning engine.
