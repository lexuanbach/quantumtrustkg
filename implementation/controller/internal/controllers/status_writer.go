// Status write-back for AssignmentStatus and MigrationPlan (Sect. 3,
// implementation).
//
// After each edge decision the runtime hands the result to a StatusWriter. The
// Kubernetes implementation stores the desired profile, the active profile, the
// state and the reason in the AssignmentStatus object of the edge (creating it on
// first sight), and sets status.phase, observedGeneration and lastEvaluatedAt
// through the status subresource. This is the auditable status record described
// in Sect. 3. Who may write it is left to RBAC. Limitation: deploy/rbac.yaml
// binds a single ClusterRole to the controller, which means the publisher, policy author
// and status writer separation named in Sect. 3 is not expressed in the shipped
// manifests.
// MigrationPlan status carries the phase, the number of completed edges, the last
// batch size and the last error.
//
// NoopStatusWriter is used in one-shot runs and unit tests where no API server is
// present. Both writers treat a nil client as a no-op, which means a mis-wired runtime
// never crashes a reconcile.

package controllers

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"quantumtrustkg/controller/pkg/types"
)

// StatusWriter persists the decision for one edge.
type StatusWriter interface {
	WriteAssignmentStatus(ctx context.Context, status types.AssignmentStatus) error
}

// MigrationWriter persists the rollout progress of one plan.
type MigrationWriter interface {
	WriteMigrationStatus(ctx context.Context, plan MigrationPlanSpec, progress MigrationProgress) error
}

// NoopStatusWriter discards all writes.
type NoopStatusWriter struct{}

func (NoopStatusWriter) WriteAssignmentStatus(_ context.Context, _ types.AssignmentStatus) error {
	return nil
}

func (NoopStatusWriter) WriteMigrationStatus(_ context.Context, _ MigrationPlanSpec, _ MigrationProgress) error {
	return nil
}

// KubernetesStatusWriter writes through the controller-runtime client.
type KubernetesStatusWriter struct {
	Client ctrlclient.Client
}

// NewKubernetesStatusWriter wraps client in a status writer.
func NewKubernetesStatusWriter(client ctrlclient.Client) *KubernetesStatusWriter {
	return &KubernetesStatusWriter{Client: client}
}

// WriteAssignmentStatus creates or updates the AssignmentStatus of the edge.
// The spec fields are updated first and the status subresource second, because
// the API server ignores status fields on an ordinary update. A status without
// namespace or ID is rejected before any call is made.
func (w *KubernetesStatusWriter) WriteAssignmentStatus(ctx context.Context, status types.AssignmentStatus) error {
	if w == nil || w.Client == nil {
		return nil
	}
	if err := validateStatusForWrite(status); err != nil {
		return err
	}

	obj := assignmentObject()
	key := ctrlclient.ObjectKey{Namespace: status.Namespace, Name: status.ID}
	err := w.Client.Get(ctx, key, obj)
	if err != nil {
		if ctrlclient.IgnoreNotFound(err) != nil {
			return err
		}
		obj = assignmentObject()
		obj.SetName(status.ID)
		obj.SetNamespace(status.Namespace)
		if err := setAssignmentSpec(obj, status); err != nil {
			return err
		}
		if err := setAssignmentStatus(obj, status, 1); err != nil {
			return err
		}
		return w.Client.Create(ctx, obj)
	}

	if err := setAssignmentSpec(obj, status); err != nil {
		return err
	}
	if err := w.Client.Update(ctx, obj); err != nil {
		return err
	}
	generation := obj.GetGeneration()
	if generation == 0 {
		generation = 1
	}
	if err := setAssignmentStatus(obj, status, generation); err != nil {
		return err
	}
	return w.Client.Status().Update(ctx, obj)
}

// WriteMigrationStatus updates the status of an existing MigrationPlan. A plan
// that no longer exists is not an error.
func (w *KubernetesStatusWriter) WriteMigrationStatus(ctx context.Context, plan MigrationPlanSpec, progress MigrationProgress) error {
	if w == nil || w.Client == nil {
		return nil
	}
	if plan.Name == "" || plan.Namespace == "" {
		return fmt.Errorf("migration status requires namespace and name")
	}

	obj := migrationObject()
	key := ctrlclient.ObjectKey{Namespace: plan.Namespace, Name: plan.Name}
	if err := w.Client.Get(ctx, key, obj); err != nil {
		return ctrlclient.IgnoreNotFound(err)
	}

	generation := obj.GetGeneration()
	if generation == 0 {
		generation = 1
	}
	status := map[string]any{
		"phase":              progress.LastPhase,
		"lastTransitionTime": nowRFC3339(),
		"observedGeneration": generation,
		"completedEdges":     int64(len(progress.CompletedEdges)),
		"lastBatchSize":      int64(progress.LastBatchSize),
	}
	if progress.LastError != "" {
		status["lastError"] = progress.LastError
	}
	if err := unstructured.SetNestedMap(obj.Object, status, "status"); err != nil {
		return err
	}
	return w.Client.Status().Update(ctx, obj)
}

// setAssignmentSpec stores the decision fields in spec, which is where the
// AssignmentStatus CRD keeps them (see api/crds/assignmentstatuses.yaml).
func setAssignmentSpec(obj *unstructured.Unstructured, status types.AssignmentStatus) error {
	spec := map[string]any{
		"sourceService":         status.SourceService,
		"destinationService":    status.DestinationService,
		"desiredProfile":        status.DesiredProfile,
		"activeProfile":         status.ActiveProfile,
		"requiredSecurityLevel": string(status.RequiredSecurityLevel),
		"requirementReason":     status.RequirementReason,
		"state":                 string(status.State),
		"reason":                status.Reason,
	}
	return unstructured.SetNestedMap(obj.Object, spec, "spec")
}

// setAssignmentStatus fills the status subresource. generation is copied into
// observedGeneration.
func setAssignmentStatus(obj *unstructured.Unstructured, status types.AssignmentStatus, generation int64) error {
	fields := map[string]any{
		"observedGeneration": generation,
		"lastEvaluatedAt":    status.LastEvaluatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"phase":              string(status.State),
	}
	return unstructured.SetNestedMap(obj.Object, fields, "status")
}

// validateStatusForWrite checks the identity fields required to address the object.
func validateStatusForWrite(status types.AssignmentStatus) error {
	if status.ID == "" || status.Namespace == "" {
		return fmt.Errorf("assignment status requires namespace and id")
	}
	return nil
}

// nowRFC3339 returns the current UTC time in RFC 3339 format.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
