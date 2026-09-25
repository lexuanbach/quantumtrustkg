# Controller Flow

The current artifact implements the paper-level control-plane path over fixture
data, Kubernetes CRD state, and optional Fuseki queries.

1. Load ontology, SPARQL query, SHACL aid, and fixture assets.
2. Build communication edges from fixtures or Kubernetes service annotations.
3. Apply watched policy, capability, migration, and status resources.
4. Derive the strongest edge requirement from policies, trust boundary, and
   defaults.
5. Check graph availability. In manager mode, graph outage blocks new
   promotions and preserves the last committed active assignment.
6. Validate freshness, contradiction-freedom, required class, and candidate
   presence.
7. Run QTPO: support/policy/threat filters first, then ranking over admissible
   candidates; hybrid fallback only when policy explicitly allows it.
8. Synthesize Istio `PeerAuthentication` and `DestinationRule` resources.
9. Record status and staged-rollout progress; rollback freezes later batches
   when mesh health turns unhealthy.

The controller does not implement a full OWL/SHACL runtime, a liboqs/Envoy data
plane fork, real PQ negotiation, or signed workload-claim verification in this
milestone. PQ setup-cost values in the paper are model-calibrated estimates,
not data-plane measurements.
