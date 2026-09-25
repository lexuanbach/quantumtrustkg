(* Examples.v: a small finance mesh on which select is evaluated by
   computation.

   The scenario is modelled on the finance fixture in
   implementation/semantics/fixtures (Sect. 4 of the paper uses much larger
   generated meshes). A frontend calls an auth service over an internal
   boundary, and auth calls a payment
   service across a regulated boundary that a PCI policy governs. The
   frontend advertises only a hybrid profile and the payment service only a
   quantum-safe one. The examples check four behaviours: a direct promotion
   of a hybrid profile on the internal edge, a direct promotion of the
   quantum-safe profile on the regulated edge, and a block for stale and for
   contradictory evidence. The last two examples relate the regulated edge to
   Req(e) and to G2. Each example is closed by reflexivity, which checks that
   select reduces to the stated outcome. The hybrid fallback branch is not
   exercised by any example here. It is covered by G3 in general and by unit
   tests of the Go controller. The model is a simplification of the
   fixture. For instance, the PCI policy here forbids the hybrid fallback,
   and identifiers are numbers instead of names. *)

From QuantumTrustKG Require Import Model QTPO Guarantees.

(* Numeric identifiers for the services, the two profiles and the two
   policies of the scenario. *)
Definition sid_frontend : service_id := 1.
Definition sid_auth : service_id := 2.
Definition sid_payment : service_id := 3.
Definition sid_ledger : service_id := 4.

Definition pid_hybrid : profile_id := 10.
Definition pid_quantum : profile_id := 11.

Definition pol_default : policy_id := 20.
Definition pol_pci : policy_id := 21.

(* Capability records. The frontend supports only the hybrid profile, auth
   supports both profiles, and payment and ledger support only the
   quantum-safe profile. The ledger service is declared but no edge in this
   file uses it. *)
Definition svc_frontend : service :=
  {| service_name := sid_frontend;
     service_supported_profiles := pid_hybrid :: nil |}.

Definition svc_auth : service :=
  {| service_name := sid_auth;
     service_supported_profiles := pid_hybrid :: pid_quantum :: nil |}.

Definition svc_payment : service :=
  {| service_name := sid_payment;
     service_supported_profiles := pid_quantum :: nil |}.

Definition svc_ledger : service :=
  {| service_name := sid_ledger;
     service_supported_profiles := pid_quantum :: nil |}.

(* Two profiles with distinct classes. The hybrid profile is cheaper and is
   recorded as robust. The quantum-safe profile is not marked as a hybrid
   combiner. It is therefore never a fallback candidate. Neither is deprecated. *)
Definition hybrid_profile : profile :=
  {| profile_name := pid_hybrid;
     profile_class := Hybrid;
     profile_deprecated := false;
     profile_hybrid_robust := true;
     profile_score := 1 |}.

Definition quantum_profile : profile :=
  {| profile_name := pid_quantum;
     profile_class := QuantumSafe;
     profile_deprecated := false;
     profile_hybrid_robust := false;
     profile_score := 2 |}.

(* The default policy asks for hybrid and permits the fallback. The PCI
   policy asks for quantum-safe and does not permit the fallback. *)
Definition default_policy : policy :=
  {| policy_name := pol_default;
     policy_required := Hybrid;
     policy_allow_hybrid := true |}.

Definition pci_policy : policy :=
  {| policy_name := pol_pci;
     policy_required := QuantumSafe;
     policy_allow_hybrid := false |}.

(* The control state of the scenario. The graph is reachable. *)
Definition finance_state : control_state :=
  {| st_services := svc_frontend :: svc_auth :: svc_payment :: svc_ledger :: nil;
     st_profiles := hybrid_profile :: quantum_profile :: nil;
     st_policies := default_policy :: pci_policy :: nil;
     st_graph_available := true |}.

(* Three provenance records. The good one passes gate (iv). The stale one is
   outside its freshness window and the conflicting one carries a
   contradiction. *)
Definition good_provenance : provenance :=
  {| fact_available := true; fact_fresh := true; fact_conflict := false |}.

Definition stale_provenance : provenance :=
  {| fact_available := true; fact_fresh := false; fact_conflict := false |}.

Definition conflicting_provenance : provenance :=
  {| fact_available := true; fact_fresh := true; fact_conflict := true |}.

(* A rollout state that lets the edge move now. *)
Definition ready_rollout : rollout_state :=
  {| rollout_feasible := true; mesh_ready := true |}.

(* An internal edge governed by the default policy. Its effective requirement
   is hybrid. *)
Definition frontend_auth_edge : edge :=
  {| edge_name := 100;
     edge_src := sid_frontend;
     edge_dst := sid_auth;
     edge_boundary := Internal;
     edge_policy_names := pol_default :: nil;
     edge_current_profile := None;
     edge_provenance := good_provenance;
     edge_rollout := ready_rollout |}.

(* A regulated edge governed by the PCI policy. Its effective requirement is
   quantum-safe from both the boundary and the policy. *)
Definition auth_payment_edge : edge :=
  {| edge_name := 101;
     edge_src := sid_auth;
     edge_dst := sid_payment;
     edge_boundary := Regulated;
     edge_policy_names := pol_pci :: nil;
     edge_current_profile := None;
     edge_provenance := good_provenance;
     edge_rollout := ready_rollout |}.

(* The regulated edge with stale evidence. QTPO must block it. *)
Definition stale_auth_payment_edge : edge :=
  {| edge_name := 102;
     edge_src := sid_auth;
     edge_dst := sid_payment;
     edge_boundary := Regulated;
     edge_policy_names := pol_pci :: nil;
     edge_current_profile := None;
     edge_provenance := stale_provenance;
     edge_rollout := ready_rollout |}.

(* The regulated edge with contradictory evidence. QTPO must block it. *)
Definition conflicting_auth_payment_edge : edge :=
  {| edge_name := 103;
     edge_src := sid_auth;
     edge_dst := sid_payment;
     edge_boundary := Regulated;
     edge_policy_names := pol_pci :: nil;
     edge_current_profile := None;
     edge_provenance := conflicting_provenance;
     edge_rollout := ready_rollout |}.

(* Internal edge: only the hybrid profile is common to both endpoints and it
   meets the hybrid requirement. *)
Example finance_internal_success :
  select finance_state frontend_auth_edge = PromoteDirect hybrid_profile.
Proof. reflexivity. Qed.

(* Regulated edge: only the quantum-safe profile is common to both endpoints
   and it meets the requirement. *)
Example finance_regulated_success :
  select finance_state auth_payment_edge = PromoteDirect quantum_profile.
Proof. reflexivity. Qed.

(* Stale evidence blocks with the reason StaleFacts, an instance of G4. *)
Example stale_fact_blocks :
  select finance_state stale_auth_payment_edge = Block StaleFacts.
Proof. reflexivity. Qed.

(* Contradictory evidence blocks with the reason ContradictoryFacts, an
   instance of G4. *)
Example conflicting_fact_blocks :
  select finance_state conflicting_auth_payment_edge = Block ContradictoryFacts.
Proof. reflexivity. Qed.

(* The regulated boundary and the PCI policy give Req(e) equal to
   quantum-safe. *)
Example regulated_edge_requires_quantum_safe :
  effective_requirement finance_state auth_payment_edge = QuantumSafe.
Proof. reflexivity. Qed.

(* G2 applied to the regulated edge. The promoted profile is at least as
   strong as Req(e). *)
Example regulated_direct_preserves_security_class :
  class_le
    (effective_requirement finance_state auth_payment_edge)
    (profile_class quantum_profile).
Proof.
  apply G2_security_class_preserved with (st := finance_state)
    (e := auth_payment_edge).
  reflexivity.
Qed.
