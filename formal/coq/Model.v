(* Model.v: the abstract control-plane model behind QTPO (Sect. 3 of the paper).

   This file fixes the vocabulary of the formalisation. It defines the ordered
   security classes, services, policies, cryptographic profiles, per-edge
   provenance and rollout state, and the Boolean predicates that decide
   whether a profile is a candidate for an edge. QTPO.v uses these predicates
   to define the selection function, and Guarantees.v proves the properties
   G1 to G4 about it. Examples.v runs the definitions on a small finance mesh.

   Correspondence with the paper. The predicates below are the Coq form of the
   hard gates FilterGates(p, e, t) of Sect. 3 and of the definition of the
   admissible set A(e, t) that precedes Theorem 1:
     class gate (i)      class_leb (effective_requirement st e) (profile_class p)
     threat gates (ii, iii)  threat_okb, which reads one flag, profile_deprecated
     evidence gate (iv)  provenance_okb (available, fresh, not contradicted)
     rollout gate (v)    rollout_okb and mesh readiness
     endpoint support    endpoint_supportb (the compatible set C_e of Algorithm 1)
   Freshness is abstracted to a Boolean. The model does not carry time, an
   observation timestamp or a freshness window. The Go controller derives
   that Boolean from the FreshUntil time of the edge provenance before it
   consults the mirrored predicate.

   Scope. The model is a typed abstraction of declared metadata. It says
   nothing about the bytes negotiated on the wire, and it does not model
   RDF, SPARQL or SHACL. The development uses only the standard library and
   builds with Rocq/Coq 9.1.1. Identifiers are natural numbers so that
   lookups reduce by computation in Examples.v. *)

From Stdlib Require Import Arith.PeanoNat Bool.Bool Lists.List.
Import ListNotations.

Set Implicit Arguments.

(* Declared security class of a profile or policy requirement, written
   classical, hybrid and quantum_safe in Sect. 2 of the paper. *)
Inductive security_class : Type :=
| Classical
| Hybrid
| QuantumSafe.

(* Numeric encoding of the total order classical <= hybrid <= quantum_safe.
   Every comparison between classes goes through this rank. *)
Definition class_rank (c : security_class) : nat :=
  match c with
  | Classical => 1
  | Hybrid => 2
  | QuantumSafe => 3
  end.

(* Propositional order on classes: required is at most offered. Used in the
   statements of the theorems. *)
Definition class_le (required offered : security_class) : Prop :=
  class_rank required <= class_rank offered.

(* Decidable version of class_le. It is the class gate (i) of FilterGates. *)
Definition class_leb (required offered : security_class) : bool :=
  Nat.leb (class_rank required) (class_rank offered).

(* Boolean equality on classes. The fallback branch uses it to test for Hybrid. *)
Definition class_eqb (a b : security_class) : bool :=
  match a, b with
  | Classical, Classical => true
  | Hybrid, Hybrid => true
  | QuantumSafe, QuantumSafe => true
  | _, _ => false
  end.

(* Least upper bound of two classes. It folds several attached policies into
   one required class, which keeps a permissive policy from lowering a stricter one. *)
Definition max_class (a b : security_class) : security_class :=
  if class_leb a b then b else a.

(* Soundness of class_leb with respect to class_le. *)
Lemma class_leb_true :
  forall required offered,
    class_leb required offered = true ->
    class_le required offered.
Proof.
  intros required offered H.
  unfold class_leb, class_le in *.
  apply Nat.leb_le; exact H.
Qed.

(* Soundness of class_eqb with respect to Leibniz equality. *)
Lemma class_eqb_true :
  forall a b, class_eqb a b = true -> a = b.
Proof.
  intros a b H.
  destruct a, b; simpl in H; try discriminate; reflexivity.
Qed.

(* Identifiers are plain natural numbers. Names in the Go controller are
   strings, and only their equality matters to the model. *)
Definition service_id := nat.
Definition edge_id := nat.
Definition profile_id := nat.
Definition policy_id := nat.

(* Trust boundary crossed by an edge. Only Regulated raises the baseline
   requirement, see boundary_requirement. *)
Inductive trust_boundary : Type :=
| Internal
| Partner
| Regulated
| OtherBoundary.

(* A service and the profile identifiers its endpoint advertises. This is the
   capability evidence that the paper intersects on both ends of an edge. *)
Record service : Type := {
  service_name : service_id;
  service_supported_profiles : list profile_id
}.

(* An intent-level policy. policy_required is the minimum class it demands.
   policy_allow_hybrid is the explicit permission that opens the hybrid
   fallback branch of QTPO. *)
Record policy : Type := {
  policy_name : policy_id;
  policy_required : security_class;
  policy_allow_hybrid : bool
}.

(* A deployable cryptographic profile. profile_class is the declared class.
   profile_deprecated abstracts the threat gates (ii) and (iii): the Go
   controller also rejects insufficient proof status and active advisories,
   and this model folds all of those into one flag. profile_hybrid_robust
   records that a hybrid combiner is modelled as secure while at least one
   component assumption holds. profile_score is the cost of the ranking step.
   No definition in this development reads it, see QTPO.v. *)
Record profile : Type := {
  profile_name : profile_id;
  profile_class : security_class;
  profile_deprecated : bool;
  profile_hybrid_robust : bool;
  profile_score : nat
}.

(* Summary of the evidence attached to an edge. Each flag is the outcome of a
   check made outside the model: a fact exists, it is inside its freshness
   window at the reconciliation time, and no contradiction note is recorded
   against it. This is the evidence gate (iv). *)
Record provenance : Type := {
  fact_available : bool;
  fact_fresh : bool;
  fact_conflict : bool
}.

(* Rollout gate (v) and mesh readiness. rollout_feasible says that the staged
   plan lets this edge move now. mesh_ready says that the mesh objects and
   both workloads are ready to carry the new profile. *)
Record rollout_state : Type := {
  rollout_feasible : bool;
  mesh_ready : bool
}.

(* A communication edge from edge_src to edge_dst with the policies attached
   to it. edge_current_profile is the last committed assignment. select does
   not read it. The statement that a blocked edge keeps its previous
   assignment is therefore not part of the mechanised guarantees. *)
Record edge : Type := {
  edge_name : edge_id;
  edge_src : service_id;
  edge_dst : service_id;
  edge_boundary : trust_boundary;
  edge_policy_names : list policy_id;
  edge_current_profile : option profile_id;
  edge_provenance : provenance;
  edge_rollout : rollout_state
}.

(* Everything QTPO reads besides the edge itself. st_graph_available is false
   when the Quantum Trust Graph cannot be queried, in which case no promotion
   is allowed. *)
Record control_state : Type := {
  st_services : list service;
  st_profiles : list profile;
  st_policies : list policy;
  st_graph_available : bool
}.

(* First service with the given identifier, if any. Lookups return the first
   match. A duplicated identifier is shadowed by the earlier entry. *)
Fixpoint lookup_service (sid : service_id) (services : list service)
  : option service :=
  match services with
  | [] => None
  | s :: rest =>
      if Nat.eqb sid (service_name s)
      then Some s
      else lookup_service sid rest
  end.

(* First profile with the given identifier. Not used by the selection
   function, which scans st_profiles directly. It is kept for reasoning about
   profiles by identifier. *)
Fixpoint lookup_profile (pid : profile_id) (profiles : list profile)
  : option profile :=
  match profiles with
  | [] => None
  | p :: rest =>
      if Nat.eqb pid (profile_name p)
      then Some p
      else lookup_profile pid rest
  end.

(* First policy with the given identifier. Same remarks as lookup_profile. *)
Fixpoint lookup_policy (pid : policy_id) (policies : list policy)
  : option policy :=
  match policies with
  | [] => None
  | p :: rest =>
      if Nat.eqb pid (policy_name p)
      then Some p
      else lookup_policy pid rest
  end.

(* The policy is attached to the edge when its identifier is listed in
   edge_policy_names. *)
Definition attached_policyb (e : edge) (p : policy) : bool :=
  existsb (Nat.eqb (policy_name p)) (edge_policy_names e).

(* Baseline class demanded by the boundary alone. A regulated boundary needs
   quantum_safe and every other boundary needs at least hybrid. *)
Definition boundary_requirement (b : trust_boundary) : security_class :=
  match b with
  | Regulated => QuantumSafe
  | Internal => Hybrid
  | Partner => Hybrid
  | OtherBoundary => Hybrid
  end.

(* Req(e) of the paper. It starts from the boundary baseline and joins in
   the required class of every attached policy. The result is the
   strongest class that applies to the edge. *)
Definition effective_requirement (st : control_state) (e : edge)
  : security_class :=
  fold_left
    (fun acc p =>
       if attached_policyb e p
       then max_class acc (policy_required p)
       else acc)
    (st_policies st)
    (boundary_requirement (edge_boundary e)).

(* PolicyPermitsHybrid(e) of Algorithm 1: some attached policy explicitly
   allows the hybrid fallback. *)
Definition policy_permits_hybrid (st : control_state) (e : edge) : bool :=
  existsb
    (fun p => attached_policyb e p && policy_allow_hybrid p)
    (st_policies st).

(* A service supports a profile when its record lists it. An unknown service
   supports nothing, which is the fail-closed reading of missing capability
   evidence. *)
Definition supports_profileb
    (st : control_state) (sid : service_id) (pid : profile_id) : bool :=
  match lookup_service sid (st_services st) with
  | Some s => existsb (Nat.eqb pid) (service_supported_profiles s)
  | None => false
  end.

(* Both endpoints of the edge advertise the profile, that is, p belongs to
   the compatible set C_e of Algorithm 1. *)
Definition endpoint_supportb
    (st : control_state) (e : edge) (p : profile) : bool :=
  supports_profileb st (edge_src e) (profile_name p) &&
  supports_profileb st (edge_dst e) (profile_name p).

(* Threat gates (ii) and (iii) at the level of abstraction of this model. *)
Definition threat_okb (p : profile) : bool :=
  negb (profile_deprecated p).

(* Evidence gate (iv). The facts must exist, be fresh and be free of
   contradiction. *)
Definition provenance_okb (e : edge) : bool :=
  fact_available (edge_provenance e) &&
  fact_fresh (edge_provenance e) &&
  negb (fact_conflict (edge_provenance e)).

(* Rollout gate (v), together with mesh readiness. *)
Definition rollout_okb (e : edge) : bool :=
  rollout_feasible (edge_rollout e) && mesh_ready (edge_rollout e).

(* The gates that depend on the edge and the graph but not on the candidate
   profile: the graph must be reachable, the evidence must be usable and the
   rollout must be feasible. Theorem G4 in Guarantees.v is stated in terms
   of this predicate. *)
Definition gates_okb (st : control_state) (e : edge) : bool :=
  st_graph_available st && provenance_okb e && rollout_okb e.

(* A profile that QTPO may promote directly. It must be supported by both
   endpoints, pass the threat gate, meet the required class and satisfy the
   edge-level gates. This is the Coq form of membership in A(e, t). *)
Definition direct_candidateb
    (st : control_state) (e : edge) (p : profile) : bool :=
  endpoint_supportb st e p &&
  threat_okb p &&
  class_leb (effective_requirement st e) (profile_class p) &&
  gates_okb st e.

(* A profile that QTPO may promote through the guarded hybrid fallback. The
   class gate is replaced by three conditions: the class is exactly Hybrid,
   the combiner is recorded as robust and a policy on the edge permits the
   fallback. All other gates still apply. *)
Definition fallback_candidateb
    (st : control_state) (e : edge) (p : profile) : bool :=
  endpoint_supportb st e p &&
  threat_okb p &&
  class_eqb (profile_class p) Hybrid &&
  profile_hybrid_robust p &&
  policy_permits_hybrid st e &&
  gates_okb st e.
