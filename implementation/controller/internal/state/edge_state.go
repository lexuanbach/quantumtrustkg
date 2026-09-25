package state

import "quantumtrustkg/controller/pkg/types"

// EdgeStore is an in-memory cache of the last known state of each edge, keyed by
// edge ID. The runtime reads the committed active profile from it. That is how a
// blocked edge, for example one whose graph is unavailable, keeps its previous
// assignment (the fail-closed behaviour of Theorem 2). It is not persistent and
// not safe for concurrent use.
type EdgeStore struct {
	Edges map[string]types.CommunicationEdge
}

// NewEdgeStore returns an empty store.
func NewEdgeStore() *EdgeStore {
	return &EdgeStore{Edges: make(map[string]types.CommunicationEdge)}
}

// Upsert inserts the edge or replaces the entry with the same ID.
func (s *EdgeStore) Upsert(edge types.CommunicationEdge) {
	s.Edges[edge.ID] = edge
}

// Get returns the stored edge and whether the ID was present.
func (s *EdgeStore) Get(id string) (types.CommunicationEdge, bool) {
	edge, ok := s.Edges[id]
	return edge, ok
}
