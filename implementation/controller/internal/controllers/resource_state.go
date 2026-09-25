// In-memory store for the operator-supplied resources (Sect. 3,
// implementation).
//
// ResourceState is the controller's cache of the policy, capability and
// migration objects it has seen, the labels of the Services it has listed, the
// runtime deprecation notices, and the per-plan migration progress. The
// reconcilers write to it and AssignmentRuntime reads from it on every
// evaluation. All methods take the mutex, and every accessor returns a copy so a
// caller cannot mutate shared state. Kubernetes stays the source of truth. If the
// process restarts the cache is rebuilt from watch events, and the committed
// active profile of each edge is recovered from the AssignmentStatus objects and
// not from this file.
//
// The migration progress kept here backs the staged rollout: an edge first
// becomes pending-health after its profile is applied, and it counts as completed
// only after the health observer has seen both workloads Ready.

package controllers

import (
	"sync"

	"quantumtrustkg/controller/pkg/types"
)

// MigrationPlanSpec is the parsed spec of a MigrationPlan. Strategy is one of
// the values understood by orchestration.ComputeProgressiveRolloutWindow.
type MigrationPlanSpec struct {
	Name                string
	Namespace           string
	TargetSelector      map[string]string
	Strategy            string
	MaxUnavailableEdges int64
	RollbackOnError     bool
}

// MigrationProgress tracks one plan. CompletedEdges are edges whose new profile
// was confirmed healthy. PendingHealthEdges were applied but not yet confirmed.
// LastPhase, LastError and LastBatchSize are what WriteMigrationStatus reports.
type MigrationProgress struct {
	CompletedEdges     map[string]bool
	PendingHealthEdges map[string]bool
	LastPhase          string
	LastError          string
	LastBatchSize      int
}

// ResourceState is safe for concurrent use.
type ResourceState struct {
	mu            sync.RWMutex
	revision      uint64
	policies      map[string]types.Policy
	capabilities  map[string]types.CapabilityProfile
	migrations    map[string]MigrationPlanSpec
	progress      map[string]MigrationProgress
	serviceLabels map[string]map[string]string
	deprecated    map[string]string
}

// NewResourceState returns an empty store.
func NewResourceState() *ResourceState {
	return &ResourceState{
		policies:      make(map[string]types.Policy),
		capabilities:  make(map[string]types.CapabilityProfile),
		migrations:    make(map[string]MigrationPlanSpec),
		progress:      make(map[string]MigrationProgress),
		serviceLabels: make(map[string]map[string]string),
		deprecated:    make(map[string]string),
	}
}

// namespacedKey is the map key namespace/name shared by all resource maps.
func namespacedKey(namespace, name string) string {
	return namespace + "/" + name
}

// UpsertPolicy stores or replaces a policy by namespace and name.
func (s *ResourceState) UpsertPolicy(policy types.Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[namespacedKey(policy.Namespace, policy.Name)] = policy
	s.revision++
}

// UpsertCapability stores or replaces a capability record by namespace and name.
func (s *ResourceState) UpsertCapability(capability types.CapabilityProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capabilities[namespacedKey(capability.Namespace, capability.Name)] = capability
	s.revision++
}

// UpsertMigration stores or replaces a migration plan by namespace and name.
func (s *ResourceState) UpsertMigration(plan MigrationPlanSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrations[namespacedKey(plan.Namespace, plan.Name)] = plan
	s.revision++
}

// UpsertServiceLabels stores the labels of a Service and adds the synthetic
// labels app and service (both set to the Service name), which means selectors written
// against either name match.
func (s *ResourceState) UpsertServiceLabels(namespace, service string, labels map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make(map[string]string, len(labels)+2)
	for key, value := range labels {
		copied[key] = value
	}
	copied["app"] = service
	copied["service"] = service
	s.serviceLabels[namespacedKey(namespace, service)] = copied
	s.revision++
}

// UpsertDeprecation records a deprecation or active advisory against a profile
// or primitive family name. Gates (ii) and (iii) of the QTPO filter reject
// candidates that match.
func (s *ResourceState) UpsertDeprecation(name, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deprecated[name] = reason
	s.revision++
}

// RemoveDeprecation withdraws a recorded deprecation or advisory.
func (s *ResourceState) RemoveDeprecation(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.deprecated, name)
	s.revision++
}

// SnapshotRevision is a monotonic version of the controller's evidence cache.
// A reconciliation captures it before selection and compares it immediately
// before promotion. A mismatch means policy or capability evidence changed
// concurrently, so the promotion is retried from a fresh snapshot.
func (s *ResourceState) SnapshotRevision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

// DeprecatedProfiles returns a copy of the recorded deprecations and advisories
// keyed by name, with the reason as value.
func (s *ResourceState) DeprecatedProfiles() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.deprecated))
	for k, v := range s.deprecated {
		out[k] = v
	}
	return out
}

// Policies returns a snapshot of all stored policies in no particular order.
func (s *ResourceState) Policies() []types.Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.Policy, 0, len(s.policies))
	for _, policy := range s.policies {
		result = append(result, policy)
	}
	return result
}

// Capabilities returns a snapshot of all stored capability records in no
// particular order.
func (s *ResourceState) Capabilities() []types.CapabilityProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.CapabilityProfile, 0, len(s.capabilities))
	for _, capability := range s.capabilities {
		result = append(result, capability)
	}
	return result
}

// Migrations returns a snapshot of all stored migration plans in no particular
// order.
func (s *ResourceState) Migrations() []MigrationPlanSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]MigrationPlanSpec, 0, len(s.migrations))
	for _, migration := range s.migrations {
		result = append(result, migration)
	}
	return result
}

// MarkMigrationCompleted is an alias of MarkMigrationHealthy.
func (s *ResourceState) MarkMigrationCompleted(plan MigrationPlanSpec, edgeID string) {
	s.MarkMigrationHealthy(plan, edgeID)
}

// MarkMigrationApplied records that the edge's profile was applied and
// awaits a healthy observation.
func (s *ResourceState) MarkMigrationApplied(plan MigrationPlanSpec, edgeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := namespacedKey(plan.Namespace, plan.Name)
	progress := s.progress[key]
	if progress.PendingHealthEdges == nil {
		progress.PendingHealthEdges = make(map[string]bool)
	}
	progress.PendingHealthEdges[edgeID] = true
	s.progress[key] = progress
}

// MarkMigrationHealthy moves an edge from pending-health to completed.
func (s *ResourceState) MarkMigrationHealthy(plan MigrationPlanSpec, edgeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := namespacedKey(plan.Namespace, plan.Name)
	progress := s.progress[key]
	if progress.CompletedEdges == nil {
		progress.CompletedEdges = make(map[string]bool)
	}
	progress.CompletedEdges[edgeID] = true
	if progress.PendingHealthEdges != nil {
		delete(progress.PendingHealthEdges, edgeID)
	}
	s.progress[key] = progress
}

// CompletedEdges returns a copy of the completed-edge set of a plan.
func (s *ResourceState) CompletedEdges(plan MigrationPlanSpec) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := namespacedKey(plan.Namespace, plan.Name)
	progress := s.progress[key]
	result := make(map[string]bool, len(progress.CompletedEdges))
	for edgeID, done := range progress.CompletedEdges {
		result[edgeID] = done
	}
	return result
}

// SetMigrationPhase records the phase, last error and batch size to report.
func (s *ResourceState) SetMigrationPhase(plan MigrationPlanSpec, phase, lastError string, batchSize int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := namespacedKey(plan.Namespace, plan.Name)
	progress := s.progress[key]
	progress.LastPhase = phase
	progress.LastError = lastError
	progress.LastBatchSize = batchSize
	s.progress[key] = progress
}

// ClearPendingHealth drops an edge from the pending-health set without marking
// it completed, which is what a rollback does.
func (s *ResourceState) ClearPendingHealth(plan MigrationPlanSpec, edgeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := namespacedKey(plan.Namespace, plan.Name)
	progress := s.progress[key]
	if progress.PendingHealthEdges != nil {
		delete(progress.PendingHealthEdges, edgeID)
	}
	s.progress[key] = progress
}

// MigrationProgress returns a deep copy of the progress of a plan.
func (s *ResourceState) MigrationProgress(plan MigrationPlanSpec) MigrationProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := namespacedKey(plan.Namespace, plan.Name)
	progress := s.progress[key]
	copyProgress := MigrationProgress{
		CompletedEdges:     make(map[string]bool, len(progress.CompletedEdges)),
		PendingHealthEdges: make(map[string]bool, len(progress.PendingHealthEdges)),
		LastPhase:          progress.LastPhase,
		LastError:          progress.LastError,
		LastBatchSize:      progress.LastBatchSize,
	}
	for edgeID, done := range progress.CompletedEdges {
		copyProgress.CompletedEdges[edgeID] = done
	}
	for edgeID, pending := range progress.PendingHealthEdges {
		copyProgress.PendingHealthEdges[edgeID] = pending
	}
	return copyProgress
}

// ServiceLabels returns a copy of the labels of a Service. The app and service
// labels are always present.
func (s *ResourceState) ServiceLabels(namespace, service string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	labels := s.serviceLabels[namespacedKey(namespace, service)]
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	if _, ok := result["app"]; !ok {
		result["app"] = service
	}
	if _, ok := result["service"]; !ok {
		result["service"] = service
	}
	return result
}
