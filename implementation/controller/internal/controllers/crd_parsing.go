// Typed views of the four QuantumTrustKG custom resources (Sect. 3,
// implementation).
//
// The controller reads the CRDs of implementation/api/crds through
// unstructured objects and converts them to the internal types, which means no generated
// client is needed. Each parse function validates the fields the selection logic
// depends on and fails on a missing required field. A record that does not parse
// is rejected instead of being replaced by a default, which keeps malformed input
// from widening what an edge may use. The only default is the 300 s freshness
// window of a capability record.
//
//   QuantumSecurityPolicy   -> types.Policy               (required class, selector)
//   CryptoCapabilityProfile -> types.CapabilityProfile    (profiles, source, freshness)
//   MigrationPlan           -> MigrationPlanSpec          (staged rollout)
//   AssignmentStatus        -> source and destination names only

package controllers

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"quantumtrustkg/controller/pkg/types"
)

// Group, version and kind of each resource. The group and version match the
// CRD manifests in implementation/api/crds.
var (
	policyGVK     = schema.GroupVersionKind{Group: "quantumtrustkg.io", Version: "v1alpha1", Kind: "QuantumSecurityPolicy"}
	capabilityGVK = schema.GroupVersionKind{Group: "quantumtrustkg.io", Version: "v1alpha1", Kind: "CryptoCapabilityProfile"}
	migrationGVK  = schema.GroupVersionKind{Group: "quantumtrustkg.io", Version: "v1alpha1", Kind: "MigrationPlan"}
	assignmentGVK = schema.GroupVersionKind{Group: "quantumtrustkg.io", Version: "v1alpha1", Kind: "AssignmentStatus"}
)

func policyObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(policyGVK)
	return obj
}

func capabilityObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(capabilityGVK)
	return obj
}

func migrationObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(migrationGVK)
	return obj
}

func assignmentObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(assignmentGVK)
	return obj
}

// parsePolicy reads spec.requiredSecurityCategory (mandatory), the label
// selector, allowHybrid, the compliance tags and trustBoundaryPolicy.
// allowHybrid is the switch behind the hybrid fallback of Sect. 3.
func parsePolicy(obj *unstructured.Unstructured) (types.Policy, error) {
	required, found, err := unstructured.NestedString(obj.Object, "spec", "requiredSecurityCategory")
	if err != nil || !found || required == "" {
		return types.Policy{}, fmt.Errorf("policy missing requiredSecurityCategory")
	}
	selector, _, err := unstructured.NestedStringMap(obj.Object, "spec", "selector")
	if err != nil {
		return types.Policy{}, err
	}
	allowHybrid, _, err := unstructured.NestedBool(obj.Object, "spec", "allowHybrid")
	if err != nil {
		return types.Policy{}, err
	}
	compliance, _, err := unstructured.NestedStringSlice(obj.Object, "spec", "compliance")
	if err != nil {
		return types.Policy{}, err
	}
	trustBoundary, _, err := unstructured.NestedString(obj.Object, "spec", "trustBoundaryPolicy")
	if err != nil {
		return types.Policy{}, err
	}
	return types.Policy{
		Name:                     obj.GetName(),
		Namespace:                obj.GetNamespace(),
		Selector:                 selector,
		RequiredSecurityCategory: types.SecurityCategory(required),
		AllowHybrid:              allowHybrid,
		Compliance:               compliance,
		TrustBoundaryPolicy:      trustBoundary,
	}, nil
}

// parseCapability reads the advertised algorithms (mandatory), profiles, source
// and freshness window. The observation time is taken from status.observedAt
// when present and valid. Otherwise it is the parse time, which means a record
// without an explicit timestamp is treated as observed now. The fixture and CRD
// paths therefore rely on the writer to stamp status.observedAt if stale
// evidence is to be detected.
func parseCapability(obj *unstructured.Unstructured) (types.CapabilityProfile, error) {
	selector, _, err := unstructured.NestedStringMap(obj.Object, "spec", "workloadSelector")
	if err != nil {
		return types.CapabilityProfile{}, err
	}
	algorithms, found, err := unstructured.NestedStringSlice(obj.Object, "spec", "algorithms")
	if err != nil || !found || len(algorithms) == 0 {
		return types.CapabilityProfile{}, fmt.Errorf("capability missing algorithms")
	}
	profiles, _, err := unstructured.NestedStringSlice(obj.Object, "spec", "profiles")
	if err != nil {
		return types.CapabilityProfile{}, err
	}
	source, _, err := unstructured.NestedString(obj.Object, "spec", "source")
	if err != nil {
		return types.CapabilityProfile{}, err
	}
	freshnessSeconds, found, err := unstructured.NestedInt64(obj.Object, "spec", "freshnessSeconds")
	if err != nil {
		return types.CapabilityProfile{}, err
	}
	if !found || freshnessSeconds <= 0 {
		freshnessSeconds = 300
	}
	observedAt := time.Now().UTC()
	if observedAtRaw, found, err := unstructured.NestedString(obj.Object, "status", "observedAt"); err == nil && found && observedAtRaw != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, observedAtRaw); parseErr == nil {
			observedAt = parsed
		}
	}
	return types.CapabilityProfile{
		Name:             obj.GetName(),
		Namespace:        obj.GetNamespace(),
		WorkloadSelector: selector,
		Algorithms:       algorithms,
		Profiles:         profiles,
		Source:           source,
		ObservedAt:       observedAt,
		FreshnessWindow:  time.Duration(freshnessSeconds) * time.Second,
	}, nil
}

// parseMigration reads the rollout strategy (mandatory), the target selector,
// maxUnavailableEdges and rollbackOnError.
func parseMigration(obj *unstructured.Unstructured) (MigrationPlanSpec, error) {
	targetSelector, _, err := unstructured.NestedStringMap(obj.Object, "spec", "targetSelector")
	if err != nil {
		return MigrationPlanSpec{}, err
	}
	strategy, found, err := unstructured.NestedString(obj.Object, "spec", "strategy")
	if err != nil || !found || strategy == "" {
		return MigrationPlanSpec{}, fmt.Errorf("migration missing strategy")
	}
	maxUnavailable, _, err := unstructured.NestedInt64(obj.Object, "spec", "maxUnavailableEdges")
	if err != nil {
		return MigrationPlanSpec{}, err
	}
	rollbackOnError, _, err := unstructured.NestedBool(obj.Object, "spec", "rollbackOnError")
	if err != nil {
		return MigrationPlanSpec{}, err
	}
	return MigrationPlanSpec{
		Name:                obj.GetName(),
		Namespace:           obj.GetNamespace(),
		TargetSelector:      targetSelector,
		Strategy:            strategy,
		MaxUnavailableEdges: maxUnavailable,
		RollbackOnError:     rollbackOnError,
	}, nil
}

// parseAssignmentTarget returns the source and destination service of an
// AssignmentStatus. Both are required.
func parseAssignmentTarget(obj *unstructured.Unstructured) (string, string, error) {
	source, found, err := unstructured.NestedString(obj.Object, "spec", "sourceService")
	if err != nil {
		return "", "", err
	}
	if !found || source == "" {
		return "", "", fmt.Errorf("assignment missing sourceService")
	}
	destination, found, err := unstructured.NestedString(obj.Object, "spec", "destinationService")
	if err != nil {
		return "", "", err
	}
	if !found || destination == "" {
		return "", "", fmt.Errorf("assignment missing destinationService")
	}
	return source, destination, nil
}
