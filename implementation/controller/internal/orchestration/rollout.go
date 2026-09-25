package orchestration

import (
	"sort"

	"quantumtrustkg/controller/pkg/types"
)

// RolloutConfig is the rollout strategy of a staged migration plan, one of the
// operator resources described in Sect. 3 (Implementation). Strategy is
// canary, staged or any other value, which means all edges at once.
// MaxUnavailableEdges bounds the number of edges that change in one step of a
// staged plan, and RollbackOnError asks the runtime to mark a failed edge for
// rollback instead of failing it.
type RolloutConfig struct {
	Strategy            string
	MaxUnavailableEdges int64
	RollbackOnError     bool
}

// MarkBlocked records that the edge is blocked with the given reason. A
// blocked edge keeps its last committed active profile, because only the
// State and Reason fields change. This is the fail-closed outcome of Theorem 2.
func MarkBlocked(edge types.CommunicationEdge, reason string) types.CommunicationEdge {
	edge.State = types.AssignmentBlocked
	edge.Reason = reason
	return edge
}

// MarkDeferred records that the edge waits for its turn in the rollout window.
// It is a pending state and not a failure.
func MarkDeferred(edge types.CommunicationEdge, reason string) types.CommunicationEdge {
	edge.State = types.AssignmentPending
	edge.Reason = reason
	return edge
}

// MarkRollback records that the edge is being rolled back to its previous
// assignment.
func MarkRollback(edge types.CommunicationEdge, reason string) types.CommunicationEdge {
	edge.State = types.AssignmentRollback
	edge.Reason = reason
	return edge
}

// ComputeRolloutWindow returns the set of edge IDs that may change in this
// reconciliation, with no edge marked as completed yet. It supports the
// rollout feasibility gate (v).
func ComputeRolloutWindow(edges []types.CommunicationEdge, cfg RolloutConfig) map[string]bool {
	return ComputeProgressiveRolloutWindow(edges, cfg, nil)
}

// ComputeProgressiveRolloutWindow returns the edge IDs allowed to change now.
// Edges are ordered by trust boundary from the least to the most sensitive
// (see edgeRiskRank), and the sort is stable so equal-risk edges keep their
// input order. Edges already completed under the plan always stay allowed, so
// a finished step is not undone. Up to the window size of further edges are
// added in risk order, which means a canary or staged plan reaches internal
// edges before partner and regulated ones. An empty input gives an empty set.
func ComputeProgressiveRolloutWindow(edges []types.CommunicationEdge, cfg RolloutConfig, completed map[string]bool) map[string]bool {
	allowed := make(map[string]bool, len(edges))
	if len(edges) == 0 {
		return allowed
	}

	window := rolloutWindowSize(len(edges), cfg)
	ranked := append([]types.CommunicationEdge(nil), edges...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return edgeRiskRank(ranked[i]) < edgeRiskRank(ranked[j])
	})
	for _, edge := range ranked {
		if completed[edge.ID] {
			allowed[edge.ID] = true
		}
	}
	added := 0
	for _, edge := range ranked {
		if completed[edge.ID] {
			continue
		}
		if added >= window {
			break
		}
		allowed[edge.ID] = true
		added++
	}
	return allowed
}

// rolloutWindowSize is the number of new edges admitted per step. A canary plan
// admits one. A staged plan admits MaxUnavailableEdges when that value is
// positive and smaller than the number of edges, and one otherwise. Any other
// strategy admits every edge. Limitation: a staged plan whose bound is at least
// the number of edges is therefore reduced to one edge per step.
func rolloutWindowSize(total int, cfg RolloutConfig) int {
	switch cfg.Strategy {
	case "canary":
		return 1
	case "staged":
		if cfg.MaxUnavailableEdges > 0 && int(cfg.MaxUnavailableEdges) < total {
			return int(cfg.MaxUnavailableEdges)
		}
		return 1
	default:
		return total
	}
}

// edgeRiskRank orders trust boundaries for rollout. Internal edges go first and
// regulated edges last. Unknown boundaries go after all named ones.
func edgeRiskRank(edge types.CommunicationEdge) int {
	switch edge.TrustBoundary {
	case "internal":
		return 1
	case "partner":
		return 2
	case "regulated":
		return 3
	default:
		return 4
	}
}
