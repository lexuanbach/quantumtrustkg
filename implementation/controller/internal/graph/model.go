package graph

import "quantumtrustkg/controller/pkg/types"

// GraphSnapshot groups the edges, policies and profiles read from the graph in
// one call. It is a plain container. No function in this package fills it yet,
// and the runtime reads policies and profiles one edge at a time instead.
type GraphSnapshot struct {
	Edges     []types.CommunicationEdge
	Policies  []types.Policy
	Protocols []types.ProtocolProfile
}
