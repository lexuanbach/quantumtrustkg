# Ontology Notes

This folder defines the semantic assets used by QuantumTrustKG.

Current contents:

- `ontology/quantumtrustkg.ttl`: core classes and properties
- `constraints/*.shacl.ttl`: lightweight validation shapes
- `queries/*.rq`: reusable SPARQL query stubs
- `fixtures/*.ttl`: scenario data for experiments and demos

The semantic layer is intentionally lightweight. It is meant to support:

- candidate protocol discovery,
- policy lookup,
- provenance and freshness checks,
- invalid-assignment detection,
- migration-edge identification.
