(* Guarantees.v: machine-checked versions of the guarantees G1 to G4 of Sect. 3.

   Mapping to the paper.
     G1_promoted_admissible          G1, first part of Theorem 1
     G2_security_class_preserved     G2, second part of Theorem 1 (direct case)
     G3_hybrid_metadata_preserved    the fallback case of Theorem 1
     G4_fail_closed                  G4, Theorem 2
   The camera-ready names G1, G2 and G4 as the checked statements. G3 is an
   additional lemma about the hybrid fallback, and it spells out what the
   exception clause of Theorem 1 gives.

   What the statements say. G1 says that any promoted profile is a candidate
   for the edge. It therefore passed every gate of Model.v. G2 says that a direct
   promotion meets the required class. G3 says that a fallback promotion is
   a hybrid profile with the robust flag and that a policy on the edge
   permits the fallback. It does not say that no direct candidate existed,
   although select only reaches the fallback in that case. G4 says that if
   an edge-level gate fails then the outcome is a block. Freshness and
   contradiction are the Boolean fields of the provenance record. The
   theorems are therefore about declared metadata. They say nothing about the
   cryptography that a workload actually negotiates.

   Proof style. Every proof is a case analysis on the Boolean conditions that
   select and the candidate predicates test. No axioms are used. *)

From Stdlib Require Import Bool.Bool.
From QuantumTrustKG Require Import Model QTPO.

Set Implicit Arguments.

(* The statement that a decision respects admissibility. A promotion must
   name a profile that is a direct or a fallback candidate for the edge. A
   block satisfies it trivially, because blocking is always safe. *)
Definition promoted_admissible
    (st : control_state) (e : edge) (d : decision) : Prop :=
  match d with
  | PromoteDirect p => direct_candidateb st e p = true
  | PromoteHybridFallback p => fallback_candidateb st e p = true
  | Block _ => True
  end.

(* Projection of direct_candidateb onto its class gate. *)
Lemma direct_candidate_class :
  forall st e p,
    direct_candidateb st e p = true ->
    class_le (effective_requirement st e) (profile_class p).
Proof.
  intros st e p H.
  unfold direct_candidateb in H.
  destruct (endpoint_supportb st e p) eqn:Hs; simpl in H; try discriminate.
  destruct (threat_okb p) eqn:Ht; simpl in H; try discriminate.
  destruct (class_leb (effective_requirement st e) (profile_class p))
    eqn:Hclass; simpl in H; try discriminate.
  apply class_leb_true; exact Hclass.
Qed.

(* A fallback candidate has declared class Hybrid. *)
Lemma fallback_candidate_hybrid :
  forall st e p,
    fallback_candidateb st e p = true ->
    profile_class p = Hybrid.
Proof.
  intros st e p H.
  unfold fallback_candidateb in H.
  destruct (endpoint_supportb st e p) eqn:Hs; simpl in H; try discriminate.
  destruct (threat_okb p) eqn:Ht; simpl in H; try discriminate.
  destruct (class_eqb (profile_class p) Hybrid) eqn:Hclass;
    simpl in H; try discriminate.
  apply class_eqb_true in Hclass; exact Hclass.
Qed.

(* A fallback candidate has the hybrid-robust metadata flag set. *)
Lemma fallback_candidate_robust :
  forall st e p,
    fallback_candidateb st e p = true ->
    profile_hybrid_robust p = true.
Proof.
  intros st e p H.
  unfold fallback_candidateb in H.
  destruct (endpoint_supportb st e p) eqn:Hs; simpl in H; try discriminate.
  destruct (threat_okb p) eqn:Ht; simpl in H; try discriminate.
  destruct (class_eqb (profile_class p) Hybrid) eqn:Hclass;
    simpl in H; try discriminate.
  destruct (profile_hybrid_robust p) eqn:Hrobust;
    simpl in H; try discriminate.
  reflexivity.
Qed.

(* A fallback candidate exists only if a policy on the edge permits hybrid. *)
Lemma fallback_candidate_policy_permits :
  forall st e p,
    fallback_candidateb st e p = true ->
    policy_permits_hybrid st e = true.
Proof.
  intros st e p H.
  unfold fallback_candidateb in H.
  destruct (endpoint_supportb st e p) eqn:Hs; simpl in H; try discriminate.
  destruct (threat_okb p) eqn:Ht; simpl in H; try discriminate.
  destruct (class_eqb (profile_class p) Hybrid) eqn:Hclass;
    simpl in H; try discriminate.
  destruct (profile_hybrid_robust p) eqn:Hrobust;
    simpl in H; try discriminate.
  destruct (policy_permits_hybrid st e) eqn:Hpermit;
    simpl in H; try discriminate.
  reflexivity.
Qed.

(* G1 (Theorem 1, admissibility). Every decision that select produces is
   admissible. The proof splits on gate_failure, then on the direct search
   and the fallback search, and uses first_satisfying_some to move from the
   profile that was found to the predicate it satisfies. *)
Theorem G1_promoted_admissible :
  forall st e,
    promoted_admissible st e (select st e).
Proof.
  intros st e.
  unfold select, promoted_admissible.
  destruct (gate_failure st e) eqn:Hgate; simpl; auto.
  destruct (first_satisfying (direct_candidateb st e) (st_profiles st))
    eqn:Hdirect.
  - apply first_satisfying_some in Hdirect as [_ Hcand].
    exact Hcand.
  - destruct (policy_permits_hybrid st e) eqn:Hpermit; simpl; auto.
    destruct (first_satisfying (fallback_candidateb st e) (st_profiles st))
      eqn:Hfallback; simpl; auto.
    apply first_satisfying_some in Hfallback as [_ Hcand].
    exact Hcand.
Qed.

(* G2 (Theorem 1, class preservation). A direct promotion never offers a
   class below the effective requirement of the edge. *)
Theorem G2_security_class_preserved :
  forall st e p,
    select st e = PromoteDirect p ->
    class_le (effective_requirement st e) (profile_class p).
Proof.
  intros st e p Hselect.
  pose proof (G1_promoted_admissible st e) as Hgood.
  rewrite Hselect in Hgood.
  simpl in Hgood.
  apply direct_candidate_class in Hgood.
  exact Hgood.
Qed.

(* Fallback case of Theorem 1. A promotion through the fallback is a hybrid
   profile recorded as robust, and it is licensed by an attached policy. *)
Theorem G3_hybrid_metadata_preserved :
  forall st e p,
    select st e = PromoteHybridFallback p ->
    profile_class p = Hybrid /\
    profile_hybrid_robust p = true /\
    policy_permits_hybrid st e = true.
Proof.
  intros st e p Hselect.
  pose proof (G1_promoted_admissible st e) as Hgood.
  rewrite Hselect in Hgood.
  simpl in Hgood.
  split.
  - apply fallback_candidate_hybrid in Hgood. exact Hgood.
  - split.
    + apply fallback_candidate_robust in Hgood. exact Hgood.
    + apply fallback_candidate_policy_permits in Hgood. exact Hgood.
Qed.

(* G4 (Theorem 2, fail-closed promotion). If the graph is unavailable, if the
   evidence is unavailable, stale or contradictory, or if the rollout or the
   mesh is not ready, then select blocks the edge. The hypothesis is stated with
   gates_okb. The proof unfolds it and walks through the same six tests as
   gate_failure. *)
Theorem G4_fail_closed :
  forall st e,
    gates_okb st e = false ->
    blockedb (select st e) = true.
Proof.
  intros st e Hbad.
  unfold gates_okb, provenance_okb, rollout_okb in Hbad.
  unfold select, gate_failure, blockedb.
  destruct (st_graph_available st) eqn:Hgraph; simpl in *; try reflexivity.
  destruct (fact_available (edge_provenance e)) eqn:Havailable;
    simpl in *; try reflexivity.
  destruct (fact_fresh (edge_provenance e)) eqn:Hfresh;
    simpl in *; try reflexivity.
  destruct (fact_conflict (edge_provenance e)) eqn:Hconflict;
    simpl in *; try reflexivity.
  destruct (rollout_feasible (edge_rollout e)) eqn:Hrollout;
    simpl in *; try reflexivity.
  destruct (mesh_ready (edge_rollout e)) eqn:Hmesh;
    simpl in *; try reflexivity.
  discriminate.
Qed.
