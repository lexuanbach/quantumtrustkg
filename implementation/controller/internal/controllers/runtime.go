// AssignmentRuntime: the per-edge control loop of Fig. 1 (Sect. 3).
//
// The runtime turns a set of communication edges into committed assignments. For
// each edge, processEdges runs the following steps, and every step that cannot
// be completed leaves the last committed profile in place (Thm. 2, G4).
//
//   1. Graph check. If the Fuseki graph does not answer within two seconds and
//      FailClosedOnGraphUnavailable is set (the manager sets it), the edge is
//      marked blocked and keeps its active profile.
//   2. Rollout window. A MigrationPlan can defer the edge to a later wave, which
//      is gate (v) of the QTPO filter.
//   3. Evidence. Policies and capability records are merged into the edge.
//      The required class Req(e) is recomputed and the candidate set is
//      restricted to profiles both endpoints advertise (C_e of Alg. 1).
//      Missing, stale or disjoint capability evidence blocks the edge.
//   4. Selection. orchestration.SelectProfileWith applies the filter gates and
//      ranks the survivors by cost (Alg. 1).
//   5. Synthesis. mesh.SynthesizeAssignment emits a PeerAuthentication and a
//      DestinationRule for the selected profile, and the Applier stores or
//      applies them.
//   6. Readiness. The profile is promoted to active only after the health
//      observer reports both workloads Ready. Until then the edge is Applying
//      and the reconciler requeues (see requeue.go).
//   7. Pre-commit check. orchestration.CheckPromotion re-evaluates admissibility
//      independently of the selector. It mirrors the Coq predicate field for
//      field and is differentially tested against it (tests/unit). A
//      disagreement blocks the promotion.
//   8. Commit. The active profile is recorded, the status is written back and
//      the graph is updated.
//
// Limitation: the controller keeps its last committed profile per edge in
// memory (state.EdgeStore) and in the AssignmentStatus objects. Signed
// capability claims are not yet bound to workload identities (Sect. 3).

package controllers

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/internal/mesh"
	"quantumtrustkg/controller/internal/observability"
	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/internal/state"
	"quantumtrustkg/controller/pkg/types"
)

// AssignmentRuntime holds the collaborators of the control loop: the graph
// client and asset loader, the mesh applier, the cluster inventory, the
// operator-resource cache, the status writers, the health observer and the
// in-memory edge and status stores. NewAssignmentRuntime builds the
// fixture-mode configuration. StartManager replaces the inventory, writers and
// health observer with their cluster-backed versions.
type AssignmentRuntime struct {
	Graph                        *graph.Client
	Loader                       *graph.AssetLoader
	Mesh                         *mesh.Applier
	Inventory                    InventorySource
	Resources                    *ResourceState
	StatusWriter                 StatusWriter
	MigrationWriter              MigrationWriter
	FailClosedOnGraphUnavailable bool
	HealthObserver               interface {
		ObserveEdgeHealth(edge types.CommunicationEdge) mesh.EdgeHealth
	}
	Store       *state.EdgeStore
	StatusStore *state.StatusStore
	// ScoreConfig overrides the ranking weights and hysteresis. Nil selects
	// orchestration.DefaultScoreConfig, the operating point stated in Sect. 3.
	// Prop. 1 says the weights cannot affect safety, which means they can be retuned
	// through this field without re-checking safety (Sect. 5).
	ScoreConfig *orchestration.ScoreConfig
}

// NewAssignmentRuntime builds a runtime for fixture-mode reconciliation. The
// generated Istio manifests go to <assetsRoot>/deploy/istio/generated, the
// status writers are no-ops and no cluster inventory is configured.
func NewAssignmentRuntime(graphEndpoint, assetsRoot string) *AssignmentRuntime {
	applier := mesh.NewApplier(filepath.Join(assetsRoot, "deploy", "istio", "generated"))
	return &AssignmentRuntime{
		Graph:           graph.NewClient(graphEndpoint),
		Loader:          graph.NewAssetLoader(assetsRoot),
		Mesh:            applier,
		Inventory:       nil,
		Resources:       NewResourceState(),
		StatusWriter:    NoopStatusWriter{},
		MigrationWriter: NoopStatusWriter{},
		HealthObserver:  applier,
		Store:           state.NewEdgeStore(),
		StatusStore:     state.NewStatusStore(),
	}
}

// ReconcileDemoEdge evaluates the finance fixture and returns the edge
// authToPayment, the single edge used in the demonstration runs.
func (r *AssignmentRuntime) ReconcileDemoEdge(ctx context.Context) (types.CommunicationEdge, error) {
	edges, err := r.ReconcileFixtureEdges(ctx, "finance-scenario.ttl")
	if err != nil {
		return types.CommunicationEdge{}, err
	}
	for _, edge := range edges {
		if edge.ID == "authToPayment" {
			return edge, nil
		}
	}
	return types.CommunicationEdge{}, fmt.Errorf("demo edge authToPayment not found")
}

// ReconcileFixtureEdges evaluates every edge of a fixture file.
func (r *AssignmentRuntime) ReconcileFixtureEdges(ctx context.Context, fixtureName string) ([]types.CommunicationEdge, error) {
	edges, err := r.loadFixtureEdges(fixtureName)
	if err != nil {
		return nil, err
	}
	return r.processEdges(ctx, edges)
}

// ReconcileClusterNamespace rebuilds the edges of a namespace from the cluster
// inventory, mirrors them into the graph when it is reachable and evaluates
// them. A failed graph sync is logged and does not stop the evaluation.
func (r *AssignmentRuntime) ReconcileClusterNamespace(ctx context.Context, namespace string) ([]types.CommunicationEdge, error) {
	if r.Inventory == nil {
		return nil, fmt.Errorf("cluster inventory is not configured")
	}
	edges, err := r.Inventory.BuildEdges(ctx, namespace)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range edges {
		edges[i] = r.applyRuntimeResources(edges[i], now)
	}
	if r.graphAvailable(ctx) {
		if err := r.Graph.SyncClusterSnapshot(ctx, edges); err != nil {
			log.Printf("warning: graph cluster sync failed: %v\n", err)
		}
	}
	return r.processEdges(ctx, edges)
}

// ReconcileFixtureEdgeByServices evaluates the single fixture edge from
// sourceService to destinationService. This is the incremental path used when
// one AssignmentStatus changes.
func (r *AssignmentRuntime) ReconcileFixtureEdgeByServices(ctx context.Context, fixtureName, sourceService, destinationService string) (types.CommunicationEdge, error) {
	edges, err := r.loadFixtureEdges(fixtureName)
	if err != nil {
		return types.CommunicationEdge{}, err
	}
	filtered := make([]types.CommunicationEdge, 0, 1)
	for _, edge := range edges {
		if edge.SourceService == sourceService && edge.DestinationService == destinationService {
			filtered = append(filtered, edge)
		}
	}
	if len(filtered) == 0 {
		return types.CommunicationEdge{}, fmt.Errorf("edge %s->%s not found in fixture %s", sourceService, destinationService, fixtureName)
	}
	results, err := r.processEdges(ctx, filtered)
	if err != nil {
		return types.CommunicationEdge{}, err
	}
	return results[0], nil
}

// loadFixtureEdges checks that the semantic assets are readable (ontology,
// candidate-protocol query, SHACL constraints) and then parses the fixture into
// edges. The assets are loaded for validation only. The edge build itself is
// done by graph.BuildEdgesFromTTL.
func (r *AssignmentRuntime) loadFixtureEdges(fixtureName string) ([]types.CommunicationEdge, error) {
	if _, err := r.Loader.LoadOntology(); err != nil {
		return nil, err
	}
	if _, err := r.Loader.LoadQuery("candidate_protocols.rq"); err != nil {
		return nil, err
	}
	if _, err := r.Loader.LoadConstraint("assignment.shacl.ttl"); err != nil {
		return nil, err
	}
	fixture, err := r.Loader.LoadFixture(fixtureName)
	if err != nil {
		return nil, err
	}

	edges, err := graph.BuildEdgesFromTTL(fixture)
	if err != nil {
		return nil, err
	}
	return edges, nil
}

// processEdges runs the loop described in the file header over edges and
// returns the evaluated edges. A validation failure or an edge with no
// selectable profile aborts the pass with an error after recording the blocked
// status. Limitation: the remaining edges of that pass are then evaluated on
// the next event, and not in the same call. Other outcomes (deferred, applying,
// failed, blocked by the pre-commit check, promoted) are returned in the
// result and processing continues.
func (r *AssignmentRuntime) processEdges(ctx context.Context, edges []types.CommunicationEdge) ([]types.CommunicationEdge, error) {
	graphAvailable := r.graphAvailable(ctx)
	r.confirmMigrationHealth()
	rolloutWindows := r.computeRolloutWindows(edges)
	planResults := make(map[string][]types.CommunicationEdge)
	results := make([]types.CommunicationEdge, 0, len(edges))
	for _, edge := range edges {
		observability.LogReconcileStart(edge.ID)

		now := time.Now().UTC()
		var evidenceRevision uint64
		if r.Resources != nil {
			evidenceRevision = r.Resources.SnapshotRevision()
		}
		edge = r.applyRuntimeResources(edge, now)
		if !graphAvailable && r.FailClosedOnGraphUnavailable {
			edge = r.markGraphUnavailable(ctx, edge, now)
			r.collectMigrationResult(planResults, edge)
			results = append(results, edge)
			continue
		}
		if !r.edgeAllowedInCurrentRollout(edge, rolloutWindows) {
			edge = orchestration.MarkDeferred(edge, "awaiting-rollout-window")
			edge.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, edge)
			r.collectMigrationResult(planResults, edge)
			observability.EmitAssignmentEvent("assignment_deferred", edge.ID, edge.Reason)
			results = append(results, edge)
			continue
		}
		if graphAvailable {
			edge = r.hydrateEdgeFromGraph(ctx, edge)
		}
		edge.ActiveProfile = r.committedActive(edge)

		// C_e of Alg. 1: the profiles both endpoints advertise, computed from
		// fresh capability evidence. A missing or stale record and an empty
		// intersection all block the edge. The intersection always replaces
		// the candidate list, which means a profile only one side supports is never
		// considered. The block reason names the capability failure when there
		// is one, and otherwise the validation error.
		compatible, capabilityFailure := orchestration.CompatibleProfiles(edge)
		edge.CandidateProfiles = compatible
		validationErr := graph.ValidateEdge(edge, now)
		if validationErr != nil && capabilityFailure != "" && validationErr.Error() == "no-candidate-profiles" {
			validationErr = fmt.Errorf("%s", capabilityFailure)
		}
		if validationErr == nil && capabilityFailure != "" {
			validationErr = fmt.Errorf("%s", capabilityFailure)
		}
		if err := validationErr; err != nil {
			edge = state.TransitionAssignment(edge, types.AssignmentBlocked, err.Error())
			edge.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, edge)
			r.collectMigrationResult(planResults, edge)
			r.flushMigrationStatuses(ctx, planResults)
			observability.EmitAssignmentEvent("assignment_blocked", edge.ID, edge.Reason)
			return nil, fmt.Errorf("validate edge %s: %w", edge.ID, err)
		}

		deprecated := r.deprecatedProfiles()
		selected, ok := orchestration.SelectProfileWith(edge, deprecated, r.scoreConfig())
		if !ok {
			selected.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, selected)
			r.collectMigrationResult(planResults, selected)
			r.flushMigrationStatuses(ctx, planResults)
			observability.EmitAssignmentEvent("assignment_blocked", selected.ID, selected.Reason)
			return nil, fmt.Errorf("qtpo could not select a profile for %s", edge.ID)
		}

		resources := mesh.SynthesizeAssignment(selected)
		if err := r.Mesh.ApplyResources(ctx, resources); err != nil {
			if plan, ok := r.applicableMigration(selected); ok && plan.RollbackOnError {
				selected = orchestration.MarkRollback(selected, "rollback-triggered")
			} else {
				selected = state.TransitionAssignment(selected, types.AssignmentFailed, "mesh-apply-failed")
			}
			selected.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, selected)
			r.collectMigrationResult(planResults, selected)
			r.flushMigrationStatuses(ctx, planResults)
			observability.EmitAssignmentEvent("assignment_failed", selected.ID, selected.Reason)
			return nil, err
		}

		// Both mesh objects must exist and both workloads must be Ready before
		// the profile is recorded as active. Until then the edge stays Applying
		// and keeps its previous active profile. An unhealthy reading fails the
		// edge or, when the plan asks for it, rolls it back.
		health := mesh.EdgeHealthUnknown
		if r.HealthObserver != nil {
			health = r.HealthObserver.ObserveEdgeHealth(selected)
		}
		switch health {
		case mesh.EdgeHealthUnhealthy:
			if plan, ok := r.applicableMigration(selected); ok && plan.RollbackOnError {
				selected = orchestration.MarkRollback(selected, "observed-unhealthy")
			} else {
				selected = state.TransitionAssignment(selected, types.AssignmentFailed, "workload-not-ready")
			}
			selected.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, selected)
			r.collectMigrationResult(planResults, selected)
			observability.EmitAssignmentEvent("assignment_failed", selected.ID, selected.Reason)
			results = append(results, selected)
			continue
		case mesh.EdgeHealthHealthy:
		default:
			selected = state.TransitionAssignment(selected, types.AssignmentApplying, "awaiting-workload-readiness")
			selected.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, selected)
			r.collectMigrationResult(planResults, selected)
			observability.EmitAssignmentEvent("assignment_applying", selected.ID, selected.Reason)
			results = append(results, selected)
			continue
		}

		// Independent pre-commit admissibility check. CheckPromotion mirrors the
		// Coq predicate and does not reuse the selector's code path. If the two
		// disagree, the edge is blocked and the previous profile stays active.
		// PromotionFacts uses the edge's provenance for freshness and conflict.
		if r.Resources != nil && r.Resources.SnapshotRevision() != evidenceRevision {
			selected = state.TransitionAssignment(selected, types.AssignmentBlocked, "evidence-snapshot-changed")
			selected.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, selected)
			r.collectMigrationResult(planResults, selected)
			observability.EmitAssignmentEvent("assignment_blocked", selected.ID, selected.Reason)
			results = append(results, selected)
			continue
		}
		profile, _ := findProfile(selected.CandidateProfiles, selected.DesiredProfile)
		facts := orchestration.PromotionFacts{
			GraphAvailable:  graphAvailable || !r.FailClosedOnGraphUnavailable,
			FactAvailable:   !selected.Provenance.FreshUntil.IsZero(),
			FactFresh:       !selected.Provenance.FreshUntil.IsZero() && !now.After(selected.Provenance.FreshUntil),
			FactConflict:    selected.Provenance.ConflictNote != "",
			RolloutFeasible: r.edgeAllowedInCurrentRollout(selected, rolloutWindows),
			MeshReady:       health == mesh.EdgeHealthHealthy,
		}
		if admissible, reason := orchestration.CheckPromotion(selected, profile, facts, deprecated); !admissible {
			selected = state.TransitionAssignment(selected, types.AssignmentBlocked, "admissibility-check-failed:"+reason)
			selected.LastEvaluatedAt = now
			r.recordEdgeStatus(ctx, selected)
			r.collectMigrationResult(planResults, selected)
			observability.EmitAssignmentEvent("assignment_blocked", selected.ID, selected.Reason)
			results = append(results, selected)
			continue
		}

		selected.ActiveProfile = selected.DesiredProfile
		selected = state.TransitionAssignment(selected, types.AssignmentPromoted, "assignment-applied")
		selected.LastEvaluatedAt = now
		if plan, ok := r.applicableMigration(selected); ok && r.Resources != nil {
			if !r.Resources.CompletedEdges(plan)[selected.ID] {
				r.Resources.MarkMigrationApplied(plan, selected.ID)
			}
		}
		r.recordEdgeStatus(ctx, selected)
		r.collectMigrationResult(planResults, selected)

		if err := r.Graph.UpsertEdge(ctx, selected); err != nil {
			log.Printf("warning: graph upsert failed: %v\n", err)
		}

		observability.EmitAssignmentEvent("assignment_promoted", selected.ID, selected.Reason)
		results = append(results, selected)
	}
	r.flushMigrationStatuses(ctx, planResults)
	return results, nil
}

// markGraphUnavailable blocks an edge because the graph could not be reached.
// If the edge already has a committed active profile, that profile is kept and
// the reason says so. This is the fail-closed case of Thm. 2: no new
// promotion, no downgrade.
func (r *AssignmentRuntime) markGraphUnavailable(ctx context.Context, edge types.CommunicationEdge, now time.Time) types.CommunicationEdge {
	reason := "graph-unavailable"
	if stored, ok := r.Store.Get(edge.ID); ok && stored.ActiveProfile != "" {
		edge.ActiveProfile = stored.ActiveProfile
		edge.DesiredProfile = stored.DesiredProfile
		if edge.DesiredProfile == "" {
			edge.DesiredProfile = stored.ActiveProfile
		}
		reason = "graph-unavailable-preserved-active"
	}
	edge = state.TransitionAssignment(edge, types.AssignmentBlocked, reason)
	edge.LastEvaluatedAt = now
	r.recordEdgeStatus(ctx, edge)
	observability.EmitAssignmentEvent("assignment_blocked", edge.ID, edge.Reason)
	return edge
}

// committedActive returns the last committed active profile of an edge. The
// stored status wins over the value carried by the edge, because the store
// holds what was last promoted.
func (r *AssignmentRuntime) committedActive(edge types.CommunicationEdge) string {
	if r.Store != nil {
		if stored, ok := r.Store.Get(edge.ID); ok {
			return stored.ActiveProfile
		}
	}
	return edge.ActiveProfile
}

// scoreConfig returns the ranking configuration, which is the operating point
// of Sect. 3 unless ScoreConfig is set.
func (r *AssignmentRuntime) scoreConfig() orchestration.ScoreConfig {
	if r.ScoreConfig != nil {
		return *r.ScoreConfig
	}
	return orchestration.DefaultScoreConfig
}

// deprecatedProfiles returns the runtime deprecation and advisory names (profile
// or family names) that gates (ii) and (iii) consult in addition to the
// metadata carried on each profile.
func (r *AssignmentRuntime) deprecatedProfiles() map[string]bool {
	out := map[string]bool{}
	if r.Resources != nil {
		for name := range r.Resources.DeprecatedProfiles() {
			out[name] = true
		}
	}
	return out
}

// findProfile looks up a profile by name in a candidate list.
func findProfile(profiles []types.ProtocolProfile, name string) (types.ProtocolProfile, bool) {
	for _, p := range profiles {
		if p.Name == name {
			return p, true
		}
	}
	return types.ProtocolProfile{}, false
}

// confirmMigrationHealth turns pending-health edges of every migration plan
// into completed ones once the observer reports them healthy. An unhealthy
// reading, or an edge that failed, rolled back or was blocked, clears the
// pending flag and puts the plan in the Rollback phase. Completed edges are
// what advances the next rollout window.
func (r *AssignmentRuntime) confirmMigrationHealth() {
	if r.Resources == nil {
		return
	}
	for _, plan := range r.Resources.Migrations() {
		progress := r.Resources.MigrationProgress(plan)
		for edgeID := range progress.PendingHealthEdges {
			edge, ok := r.Store.Get(edgeID)
			if !ok {
				continue
			}
			observed := mesh.EdgeHealthUnknown
			if r.HealthObserver != nil {
				observed = r.HealthObserver.ObserveEdgeHealth(edge)
			}
			switch {
			case observed == mesh.EdgeHealthHealthy:
				r.Resources.MarkMigrationHealthy(plan, edgeID)
			case observed == mesh.EdgeHealthUnhealthy:
				r.Resources.ClearPendingHealth(plan, edgeID)
				r.Resources.SetMigrationPhase(plan, "Rollback", "observed-unhealthy", progress.LastBatchSize)
			case edge.State == types.AssignmentFailed || edge.State == types.AssignmentRollback || edge.State == types.AssignmentBlocked:
				r.Resources.ClearPendingHealth(plan, edgeID)
				r.Resources.SetMigrationPhase(plan, "Rollback", edge.Reason, progress.LastBatchSize)
			}
		}
	}
}

// computeRolloutWindows returns, for each plan, the set of edge IDs that may be
// promoted in this pass. A plan in Rollback with rollbackOnError set gets an
// empty window, which freezes it. Otherwise the window comes from
// orchestration.ComputeProgressiveRolloutWindow, given the completed edges.
func (r *AssignmentRuntime) computeRolloutWindows(edges []types.CommunicationEdge) map[string]map[string]bool {
	windows := make(map[string]map[string]bool)
	if r.Resources == nil {
		return windows
	}
	for _, migration := range r.Resources.Migrations() {
		targeted := make([]types.CommunicationEdge, 0)
		for _, edge := range edges {
			if migrationAppliesToEdge(migration, edge) {
				targeted = append(targeted, edge)
			}
		}
		if len(targeted) == 0 {
			continue
		}
		progress := r.Resources.MigrationProgress(migration)
		if progress.LastPhase == "Rollback" && migration.RollbackOnError {
			windows[migration.Name] = map[string]bool{}
			continue
		}
		completed := r.Resources.CompletedEdges(migration)
		windows[migration.Name] = orchestration.ComputeProgressiveRolloutWindow(targeted, orchestration.RolloutConfig{
			Strategy:            migration.Strategy,
			MaxUnavailableEdges: migration.MaxUnavailableEdges,
			RollbackOnError:     migration.RollbackOnError,
		}, completed)
	}
	return windows
}

// edgeAllowedInCurrentRollout is the rollout-feasibility test of gate (v).
// Edges no plan targets and edges under an "immediate" plan are always
// feasible.
func (r *AssignmentRuntime) edgeAllowedInCurrentRollout(edge types.CommunicationEdge, windows map[string]map[string]bool) bool {
	plan, ok := r.applicableMigration(edge)
	if !ok {
		return true
	}
	if plan.Strategy == "immediate" {
		return true
	}
	window := windows[plan.Name]
	return window[edge.ID]
}

// recordEdgeStatus stores the edge in the in-memory stores and writes its
// AssignmentStatus. A failed write is logged and does not change the decision.
func (r *AssignmentRuntime) recordEdgeStatus(ctx context.Context, edge types.CommunicationEdge) {
	r.Store.Upsert(edge)
	status := types.StatusFromEdge(edge)
	r.StatusStore.Upsert(status)
	if r.StatusWriter != nil {
		if err := r.StatusWriter.WriteAssignmentStatus(ctx, status); err != nil {
			log.Printf("warning: status writeback failed: %v\n", err)
		}
	}
}

// collectMigrationResult groups evaluated edges by the plan that targets them.
func (r *AssignmentRuntime) collectMigrationResult(planResults map[string][]types.CommunicationEdge, edge types.CommunicationEdge) {
	plan, ok := r.applicableMigration(edge)
	if !ok {
		return
	}
	planResults[plan.Name] = append(planResults[plan.Name], edge)
}

// flushMigrationStatuses summarizes each plan's edges into a phase and writes
// the plan status. A plan frozen in Rollback keeps that phase.
func (r *AssignmentRuntime) flushMigrationStatuses(ctx context.Context, planResults map[string][]types.CommunicationEdge) {
	if r.MigrationWriter == nil || r.Resources == nil {
		return
	}
	for _, plan := range r.Resources.Migrations() {
		edges := planResults[plan.Name]
		if len(edges) == 0 {
			continue
		}
		progress := r.Resources.MigrationProgress(plan)
		if !(progress.LastPhase == "Rollback" && plan.RollbackOnError) {
			progress.LastPhase, progress.LastError, progress.LastBatchSize = summarizeMigrationPhase(edges)
		}
		r.Resources.SetMigrationPhase(plan, progress.LastPhase, progress.LastError, progress.LastBatchSize)
		if err := r.MigrationWriter.WriteMigrationStatus(ctx, plan, r.Resources.MigrationProgress(plan)); err != nil {
			log.Printf("warning: migration status writeback failed: %v\n", err)
		}
	}
}

// applyRuntimeResources merges the operator resources into an edge before the
// decision: the matching policies, the recomputed required class, the
// capability evidence of both endpoints and the rollout note of a matching
// plan.
func (r *AssignmentRuntime) applyRuntimeResources(edge types.CommunicationEdge, now time.Time) types.CommunicationEdge {
	if r.Resources == nil {
		return edge
	}

	for _, policy := range r.Resources.Policies() {
		if policy.Namespace != "" && edge.Namespace != "" && policy.Namespace != edge.Namespace {
			continue
		}
		if selectorMatchesLabels(policy.Selector, edge.SourceService, serviceLabelSet(r.Resources, edge.Namespace, edge.SourceService)) ||
			selectorMatchesLabels(policy.Selector, edge.DestinationService, serviceLabelSet(r.Resources, edge.Namespace, edge.DestinationService)) {
			edge.Policies = mergePolicies(edge.Policies, []types.Policy{policy})
		}
	}
	edge.RequiredSecurityLevel, edge.RequirementReason = inferRuntimeRequirement(edge)

	// Capability records are authoritative for any workload they select. The
	// union of its fresh matching records is its advertised set, and a
	// workload whose matching records are all stale has no usable evidence.
	// A workload with no matching record keeps the evidence carried by the
	// edge from the graph or fixture. If there is none, the edge blocks.
	if src, ok := r.capabilityEvidence(edge.Namespace, edge.SourceService, now); ok {
		edge.SourceCapabilities = src
	}
	if dst, ok := r.capabilityEvidence(edge.Namespace, edge.DestinationService, now); ok {
		edge.DestinationCapabilities = dst
	}
	if edge.SourceCapabilities.Known && edge.DestinationCapabilities.Known {
		edge.CandidateProfiles = ensureCapabilityProfiles(edge.CandidateProfiles,
			profileSet(edge.SourceCapabilities.Profiles), profileSet(edge.DestinationCapabilities.Profiles))
	}

	for _, migration := range r.Resources.Migrations() {
		if migration.Namespace != "" && edge.Namespace != "" && migration.Namespace != edge.Namespace {
			continue
		}
		if selectorMatchesLabels(migration.TargetSelector, edge.SourceService, serviceLabelSet(r.Resources, edge.Namespace, edge.SourceService)) ||
			selectorMatchesLabels(migration.TargetSelector, edge.DestinationService, serviceLabelSet(r.Resources, edge.Namespace, edge.DestinationService)) {
			if migration.RollbackOnError && edge.Reason == "" {
				edge.Reason = "migration-strategy:" + migration.Strategy
			}
		}
	}

	return edge
}

// graphAvailable pings the graph with a two-second timeout. It is the
// availability fact that the fail-closed rule of Thm. 2 depends on.
func (r *AssignmentRuntime) graphAvailable(ctx context.Context) bool {
	if r.Graph == nil {
		return false
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := r.Graph.Ping(pingCtx); err != nil {
		return false
	}
	return true
}

// hydrateEdgeFromGraph replaces the edge's policy and candidate lists with the
// results of the SPARQL queries, when they return something. A failed or empty
// query leaves the fixture or inventory values in place. Missing evidence is
// caught later by the capability and freshness checks.
func (r *AssignmentRuntime) hydrateEdgeFromGraph(ctx context.Context, edge types.CommunicationEdge) types.CommunicationEdge {
	if r.Graph == nil {
		return edge
	}

	if policies, err := r.Graph.ActivePolicies(ctx, edge); err == nil && len(policies) > 0 {
		edge.Policies = mergePolicies(edge.Policies, policies)
		edge.RequiredSecurityLevel, edge.RequirementReason = inferRuntimeRequirement(edge)
	}

	if candidates, err := r.Graph.CandidateProtocols(ctx, edge); err == nil && len(candidates) > 0 {
		edge.CandidateProfiles = candidates
	}

	return edge
}

// mergePolicies unions two policy lists by name. Existing entries win.
func mergePolicies(existing, discovered []types.Policy) []types.Policy {
	merged := make([]types.Policy, 0, len(existing)+len(discovered))
	seen := make(map[string]bool, len(existing)+len(discovered))
	for _, policy := range existing {
		if seen[policy.Name] {
			continue
		}
		seen[policy.Name] = true
		merged = append(merged, policy)
	}
	for _, policy := range discovered {
		if seen[policy.Name] {
			continue
		}
		seen[policy.Name] = true
		merged = append(merged, policy)
	}
	return merged
}

// strongerCategory returns the higher of two security classes in the order
// classical < hybrid < quantum-safe.
func strongerCategory(current, candidate types.SecurityCategory) types.SecurityCategory {
	if rankCategory(candidate) > rankCategory(current) {
		return candidate
	}
	return current
}

// rankCategory maps a class to its order. An unknown class ranks below classical.
func rankCategory(category types.SecurityCategory) int {
	switch category {
	case types.SecurityQuantumSafe:
		return 3
	case types.SecurityHybrid:
		return 2
	case types.SecurityClassical:
		return 1
	default:
		return 0
	}
}

// boundaryCategory gives the class floor of a trust boundary: quantum-safe for
// "regulated", hybrid for "partner" and "internal", none otherwise.
func boundaryCategory(boundary string) types.SecurityCategory {
	switch boundary {
	case "regulated":
		return types.SecurityQuantumSafe
	case "partner", "internal":
		return types.SecurityHybrid
	default:
		return ""
	}
}

// inferRuntimeRequirement computes Req(e) and a reason string. It takes the
// strongest class among the matching policies, the trust-boundary floor and,
// for a regulated edge under a PCI-DSS obligation, quantum-safe. With no input
// at all it defaults to hybrid. Taking the strongest class is what keeps a
// permissive rule from weakening a stricter one (Sect. 3). The maximum is taken
// over all matched policies and the boundary floor. The code does not order
// policies by selector specificity.
func inferRuntimeRequirement(edge types.CommunicationEdge) (types.SecurityCategory, string) {
	required := types.SecurityCategory("")
	reasons := make([]string, 0, 4)
	for _, policy := range edge.Policies {
		if policy.RequiredSecurityCategory != "" {
			required = strongerCategory(required, policy.RequiredSecurityCategory)
			reasons = append(reasons, "policy:"+policy.Name)
		}
	}
	if edge.TrustBoundary == "regulated" && hasPCIObligation(edge.Policies) {
		required = strongerCategory(required, types.SecurityQuantumSafe)
		reasons = append(reasons, "inference:pci-regulated-edge")
	}
	if boundary := boundaryCategory(edge.TrustBoundary); boundary != "" {
		required = strongerCategory(required, boundary)
		reasons = append(reasons, "boundary:"+edge.TrustBoundary)
	}
	if required == "" {
		required = types.SecurityHybrid
		reasons = append(reasons, "default:hybrid")
	}
	return required, strings.Join(reasons, ",")
}

// hasPCIObligation reports whether a policy lists the PCI-DSS obligation or has
// "pci" in its name.
func hasPCIObligation(policies []types.Policy) bool {
	for _, policy := range policies {
		for _, obligation := range policy.Compliance {
			if obligation == "PCI-DSS" {
				return true
			}
		}
		if strings.Contains(strings.ToLower(policy.Name), "pci") {
			return true
		}
	}
	return false
}

// selectorMatchesLabels reports whether every key of a non-empty selector is
// present in the labels with the same value. An empty selector matches nothing,
// so a policy or plan without a selector never applies. The app and service
// labels default to the service name. The labels map may be modified.
func selectorMatchesLabels(selector map[string]string, service string, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	if labels == nil {
		labels = map[string]string{}
	}
	if _, ok := labels["app"]; !ok {
		labels["app"] = service
	}
	if _, ok := labels["service"]; !ok {
		labels["service"] = service
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

// applicableMigration returns the first migration plan that targets the edge.
func (r *AssignmentRuntime) applicableMigration(edge types.CommunicationEdge) (MigrationPlanSpec, bool) {
	if r.Resources == nil {
		return MigrationPlanSpec{}, false
	}
	for _, migration := range r.Resources.Migrations() {
		if migrationAppliesToEdge(migration, edge) {
			return migration, true
		}
	}
	return MigrationPlanSpec{}, false
}

// migrationAppliesToEdge reports whether a plan's selector matches either
// endpoint of the edge in the same namespace. It matches on default labels
// only, because it is called without the label cache.
func migrationAppliesToEdge(migration MigrationPlanSpec, edge types.CommunicationEdge) bool {
	if migration.Namespace != "" && edge.Namespace != "" && migration.Namespace != edge.Namespace {
		return false
	}
	return selectorMatchesLabels(migration.TargetSelector, edge.SourceService, serviceLabelSet(nil, edge.Namespace, edge.SourceService)) ||
		selectorMatchesLabels(migration.TargetSelector, edge.DestinationService, serviceLabelSet(nil, edge.Namespace, edge.DestinationService))
}

// serviceLabelSet returns the cached labels of a Service, or the default app
// and service labels when no cache is given.
func serviceLabelSet(resources *ResourceState, namespace, service string) map[string]string {
	if resources == nil {
		return map[string]string{"app": service, "service": service}
	}
	return resources.ServiceLabels(namespace, service)
}

// summarizeMigrationPhase reduces the states of a plan's edges to one phase
// (Rollback, Failed, Verifying, StagedWaiting, Waiting or Completed), the last
// error reason and the number of edges in the batch. The order of the cases is
// the priority of the phases.
func summarizeMigrationPhase(edges []types.CommunicationEdge) (phase string, lastError string, batchSize int) {
	batchSize = len(edges)
	deferred := 0
	promoted := 0
	verifying := 0
	rollback := 0
	failed := 0
	blocked := 0
	for _, edge := range edges {
		switch edge.State {
		case types.AssignmentPending:
			if edge.Reason == "awaiting-rollout-window" {
				deferred++
			}
		case types.AssignmentApplying:
			verifying++
		case types.AssignmentPromoted:
			promoted++
			if edge.Reason == "assignment-applied" {
				verifying++
			}
		case types.AssignmentRollback:
			rollback++
			lastError = edge.Reason
		case types.AssignmentFailed:
			failed++
			lastError = edge.Reason
		case types.AssignmentBlocked:
			blocked++
			lastError = edge.Reason
		}
	}
	switch {
	case rollback > 0:
		return "Rollback", lastError, batchSize
	case failed > 0 || blocked > 0:
		return "Failed", lastError, batchSize
	case verifying > 0 && deferred > 0:
		return "Verifying", "", batchSize
	case verifying > 0:
		return "Verifying", "", batchSize
	case deferred > 0 && promoted > 0:
		return "StagedWaiting", "", batchSize
	case deferred > 0:
		return "Waiting", "", batchSize
	default:
		return "Completed", "", batchSize
	}
}

// profileSet converts a name list to a set.
func profileSet(profiles []string) map[string]bool {
	set := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		set[profile] = true
	}
	return set
}

// capabilityEvidence collects the capability records that select a workload.
// ok is false when no record selects it. If records match but none is fresh at
// now, the result is Known=false with Stale=true, which the runtime treats as
// missing evidence. Otherwise it lists the union of the fresh records' profiles
// and their names as the source. This is the freshness input of gate (iv).
func (r *AssignmentRuntime) capabilityEvidence(namespace, service string, now time.Time) (types.EndpointCapabilities, bool) {
	if r.Resources == nil {
		return types.EndpointCapabilities{}, false
	}
	labels := serviceLabelSet(r.Resources, namespace, service)
	matched := false
	fresh := false
	seen := map[string]bool{}
	profiles := []string{}
	sources := []string{}
	for _, capability := range r.Resources.Capabilities() {
		if capability.Namespace != "" && namespace != "" && capability.Namespace != namespace {
			continue
		}
		if !selectorMatchesLabels(capability.WorkloadSelector, service, labels) {
			continue
		}
		matched = true
		if !capability.IsFresh(now) {
			continue
		}
		fresh = true
		sources = append(sources, capability.Name)
		for _, p := range capability.Profiles {
			if !seen[p] {
				seen[p] = true
				profiles = append(profiles, p)
			}
		}
	}
	if !matched {
		return types.EndpointCapabilities{}, false
	}
	if !fresh {
		return types.EndpointCapabilities{Known: false, Stale: true, Source: "crd"}, true
	}
	return types.EndpointCapabilities{Known: true, Profiles: profiles, Source: "crd:" + strings.Join(sources, ",")}, true
}

// ensureCapabilityProfiles adds a candidate profile for each name that both
// endpoints advertise and that the edge does not list yet. It leaves the list
// unchanged if either side is empty.
func ensureCapabilityProfiles(existing []types.ProtocolProfile, sourceSet, destinationSet map[string]bool) []types.ProtocolProfile {
	if len(sourceSet) == 0 || len(destinationSet) == 0 {
		return existing
	}
	known := make(map[string]bool, len(existing))
	for _, profile := range existing {
		known[profile.Name] = true
	}
	augmented := append([]types.ProtocolProfile{}, existing...)
	for profile := range sourceSet {
		if !destinationSet[profile] || known[profile] {
			continue
		}
		augmented = append(augmented, inferredProfile(profile))
		known[profile] = true
	}
	return augmented
}

// inferredProfile builds a ProtocolProfile from a name using the same naming
// rule and overhead scores as inventoryProfile.
func inferredProfile(name string) types.ProtocolProfile {
	profile := types.ProtocolProfile{Name: name, OverheadScore: 2.0}
	switch {
	case strings.Contains(strings.ToLower(name), "quantum"):
		profile.SecurityCategory = types.SecurityQuantumSafe
		profile.OverheadScore = 2.5
	case strings.Contains(strings.ToLower(name), "hybrid"):
		profile.SecurityCategory = types.SecurityHybrid
		profile.OverheadScore = 1.5
	default:
		profile.SecurityCategory = types.SecurityClassical
		profile.OverheadScore = 1.0
	}
	return profile
}
