(* QTPO.v: the abstract selection function of QTPO (Algorithm 1, Sect. 3).

   Given a control state and an edge, select returns one of three outcomes:
   a direct promotion, a promotion through the guarded hybrid fallback, or a
   block with a reason. The definition follows the order of Algorithm 1.
   The edge-level gates are checked first and any failure blocks the edge
   with the reason of the first failed gate. Otherwise the first direct
   candidate in the profile list is promoted. If there is none and a policy
   permits it, the first fallback candidate is promoted. Otherwise the edge
   is blocked with NoAdmissibleProfile.

   What is abstracted. The paper ranks the admissible candidates by a
   weighted cost with a hysteresis margin (Sect. 3, Selection). This file
   picks the first admissible profile in list order and never reads
   profile_score. That is enough for the guarantees because they hold for
   every member of the admissible set. This matches Proposition 1
   (weight-independence), which the paper argues in prose and which is not
   stated as a Coq lemma here. The Go implementation of
   the ranking is in implementation/controller/internal/orchestration.

   Reading of block. The paper says that a blocked edge keeps its last
   committed assignment. The outcome Block here only records that no new
   promotion is made. Keeping the previous assignment is done by the
   controller and is not modelled. *)

From Stdlib Require Import Lists.List.
From QuantumTrustKG Require Import Model.
Import ListNotations.

Set Implicit Arguments.

(* Why an edge was blocked. The first six constructors name the failed
   edge-level gate and correspond to the graph-unavailable, fact-unavailable,
   stale-facts, conflicting-facts, rollout-blocked and mesh-not-ready reasons
   that the controller logs. NoAdmissibleProfile means that every gate on the
   edge passed and no profile cleared the profile-level gates. *)
Inductive block_reason : Type :=
| GraphUnavailable
| FactUnavailable
| StaleFacts
| ContradictoryFacts
| RolloutBlocked
| MeshMismatch
| NoAdmissibleProfile.

(* The result of QTPO for one edge, that is, pi(e) of Sect. 2. A promotion
   carries the chosen profile. *)
Inductive decision : Type :=
| PromoteDirect : profile -> decision
| PromoteHybridFallback : profile -> decision
| Block : block_reason -> decision.

(* First profile in the list that satisfies the predicate. It stands in for
   the ranking step, see the header. *)
Fixpoint first_satisfying
    (pred : profile -> bool) (profiles : list profile) : option profile :=
  match profiles with
  | [] => None
  | p :: rest =>
      if pred p
      then Some p
      else first_satisfying pred rest
  end.

(* Whatever first_satisfying returns is a member of the list and satisfies
   the predicate. Guarantee G1 is derived from this lemma. *)
Lemma first_satisfying_some :
  forall pred profiles p,
    first_satisfying pred profiles = Some p ->
    In p profiles /\ pred p = true.
Proof.
  intros pred profiles.
  induction profiles as [| h t IH]; intros p H; simpl in H.
  - discriminate.
  - destruct (pred h) eqn:Hh.
    + inversion H; subst. split.
      * left; reflexivity.
      * exact Hh.
    + apply IH in H as [Hin Hpred]. split.
      * right; exact Hin.
      * exact Hpred.
Qed.

(* The first failed edge-level gate, checked in the fixed order graph,
   availability, freshness, contradiction, rollout feasibility and mesh
   readiness. None means that all of them pass. This case analysis is
   equivalent to gates_okb from Model.v, and Guarantees.v relies on that
   when it proves G4. *)
Definition gate_failure (st : control_state) (e : edge)
  : option block_reason :=
  if st_graph_available st then
    if fact_available (edge_provenance e) then
      if fact_fresh (edge_provenance e) then
        if negb (fact_conflict (edge_provenance e)) then
          if rollout_feasible (edge_rollout e) then
            if mesh_ready (edge_rollout e)
            then None
            else Some MeshMismatch
          else Some RolloutBlocked
        else Some ContradictoryFacts
      else Some StaleFacts
    else Some FactUnavailable
  else Some GraphUnavailable.

(* QTPO for one edge (Algorithm 1). The hybrid fallback is tried only when
   no direct candidate exists and policy_permits_hybrid holds. The test of
   policy_permits_hybrid is repeated inside fallback_candidateb, which is
   redundant here and harmless. *)
Definition select (st : control_state) (e : edge) : decision :=
  match gate_failure st e with
  | Some reason => Block reason
  | None =>
      match first_satisfying (direct_candidateb st e) (st_profiles st) with
      | Some p => PromoteDirect p
      | None =>
          if policy_permits_hybrid st e then
            match first_satisfying (fallback_candidateb st e) (st_profiles st) with
            | Some p => PromoteHybridFallback p
            | None => Block NoAdmissibleProfile
            end
          else Block NoAdmissibleProfile
      end
  end.

(* True for both kinds of promotion. *)
Definition promotedb (d : decision) : bool :=
  match d with
  | PromoteDirect _ => true
  | PromoteHybridFallback _ => true
  | Block _ => false
  end.

(* True exactly for Block. G4 concludes with this predicate. *)
Definition blockedb (d : decision) : bool :=
  match d with
  | Block _ => true
  | _ => false
  end.
