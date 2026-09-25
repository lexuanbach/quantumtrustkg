// Applying synthesized mesh objects (Sect. 3, implementation).
//
// The Applier persists every manifest under <OutputDir>/<namespace>/ and, in
// kubectl mode, applies it with `kubectl apply -f`. The mode is chosen by the
// environment variable QTKG_APPLY_MODE (default: filesystem). After all
// resources of an edge have been stored and applied, a marker file
// <edge>.applied is written. In filesystem mode the Applier doubles as the
// health observer. It reports Healthy when the .applied marker exists, Unhealthy
// when an .unhealthy marker exists and Unknown otherwise. That behaviour supports
// the fixture-mode tests. In a cluster the manager replaces it with
// ClusterHealthObserver (health.go), which checks the live objects and pod
// readiness.
//
// Validation runs before anything is written. Every resource needs kind, name and
// namespace, a non-empty manifest that contains its own kind and a unique
// identifier, which means a malformed batch changes nothing.

package mesh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"quantumtrustkg/controller/pkg/types"
)

// ApplyMode selects where manifests go.
type ApplyMode string

const (
	ApplyModeFilesystem ApplyMode = "filesystem"
	ApplyModeKubectl    ApplyMode = "kubectl"
)

// Applier persists synthesized manifests and can optionally push them to a live
// cluster through kubectl.
type Applier struct {
	OutputDir  string
	Mode       ApplyMode
	KubectlBin string
}

// EdgeHealth is the readiness reading for an edge. Unknown means the evidence is
// missing, and the controller then keeps the edge in Applying.
type EdgeHealth string

const (
	EdgeHealthUnknown   EdgeHealth = "unknown"
	EdgeHealthHealthy   EdgeHealth = "healthy"
	EdgeHealthUnhealthy EdgeHealth = "unhealthy"
)

// NewApplier returns an Applier writing to outputDir. The mode comes from
// QTKG_APPLY_MODE.
func NewApplier(outputDir string) *Applier {
	mode := ApplyModeFilesystem
	if strings.EqualFold(os.Getenv("QTKG_APPLY_MODE"), string(ApplyModeKubectl)) {
		mode = ApplyModeKubectl
	}
	return &Applier{
		OutputDir:  outputDir,
		Mode:       mode,
		KubectlBin: "kubectl",
	}
}

// ApplyResources validates, stores and (in kubectl mode) applies the resources,
// then marks each affected edge as applied. It stops at the first error and
// does not mark the edge in that case.
func (a *Applier) ApplyResources(ctx context.Context, resources []Resource) error {
	if err := validateResources(resources); err != nil {
		return err
	}
	seenEdges := map[string]string{}
	for _, resource := range resources {
		path, err := a.persist(resource)
		if err != nil {
			return err
		}
		if a.Mode == ApplyModeKubectl {
			if err := a.applyWithKubectl(ctx, path); err != nil {
				return err
			}
		}
		if resource.EdgeID != "" {
			seenEdges[resource.Namespace+"/"+resource.EdgeID] = resource.Namespace
		}
	}
	for key, namespace := range seenEdges {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		edgeID := parts[1]
		if err := a.markApplied(namespace, edgeID); err != nil {
			return err
		}
	}
	return nil
}

// ApplyResources applies resources with a default filesystem applier writing to
// deploy/istio/generated.
func ApplyResources(resources []Resource) error {
	return NewApplier(filepath.Join("deploy", "istio", "generated")).ApplyResources(context.Background(), resources)
}

// validateResources rejects incomplete, mismatched or duplicate resources.
func validateResources(resources []Resource) error {
	seen := make(map[string]bool, len(resources))
	for _, resource := range resources {
		if resource.Kind == "" || resource.Name == "" || resource.Namespace == "" {
			return fmt.Errorf("invalid mesh resource metadata")
		}
		if strings.TrimSpace(resource.Manifest) == "" {
			return fmt.Errorf("empty manifest for %s", resource.Identifier())
		}
		if !strings.Contains(resource.Manifest, "kind: "+resource.Kind) {
			return fmt.Errorf("manifest kind mismatch for %s", resource.Identifier())
		}
		id := resource.Identifier()
		if seen[id] {
			return fmt.Errorf("duplicate resource %s", id)
		}
		seen[id] = true
	}
	return nil
}

// persist writes one manifest to disk and returns its path.
func (a *Applier) persist(resource Resource) (string, error) {
	if strings.TrimSpace(a.OutputDir) == "" {
		return "", fmt.Errorf("mesh output directory is empty")
	}
	dir := filepath.Join(a.OutputDir, resource.Namespace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, sanitizeName(strings.ToLower(resource.Kind)+"-"+resource.Name)+".yaml")
	if err := os.WriteFile(path, []byte(resource.Manifest), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// applyWithKubectl runs kubectl apply on a stored manifest and returns its
// combined output in the error.
func (a *Applier) applyWithKubectl(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, a.KubectlBin, "apply", "-f", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply %s: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ObserveEdgeHealth reads the marker files of an edge. It stands in for a real
// readiness probe in fixture mode. An unhealthy marker takes precedence over an
// applied marker.
func (a *Applier) ObserveEdgeHealth(edge types.CommunicationEdge) EdgeHealth {
	namespace := edge.Namespace
	edgeID := edge.ID
	if strings.TrimSpace(a.OutputDir) == "" || edgeID == "" {
		return EdgeHealthUnknown
	}
	edgeDir := filepath.Join(a.OutputDir, namespace)
	unhealthyPath := filepath.Join(edgeDir, sanitizeName(edgeID)+".unhealthy")
	if _, err := os.Stat(unhealthyPath); err == nil {
		return EdgeHealthUnhealthy
	}
	appliedPath := filepath.Join(edgeDir, sanitizeName(edgeID)+".applied")
	if _, err := os.Stat(appliedPath); err == nil {
		return EdgeHealthHealthy
	}
	return EdgeHealthUnknown
}

// MarkEdgeUnhealthy plants the unhealthy marker. Tests use it to simulate a
// workload that is not Ready.
func (a *Applier) MarkEdgeUnhealthy(namespace, edgeID, reason string) error {
	edgeDir := filepath.Join(a.OutputDir, namespace)
	if err := os.MkdirAll(edgeDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(edgeDir, sanitizeName(edgeID)+".unhealthy")
	return os.WriteFile(path, []byte(reason), 0o644)
}

// markApplied writes the applied marker of an edge.
func (a *Applier) markApplied(namespace, edgeID string) error {
	edgeDir := filepath.Join(a.OutputDir, namespace)
	if err := os.MkdirAll(edgeDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(edgeDir, sanitizeName(edgeID)+".applied")
	return os.WriteFile(path, []byte("applied"), 0o644)
}
