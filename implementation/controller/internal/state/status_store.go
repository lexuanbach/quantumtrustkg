package state

import "quantumtrustkg/controller/pkg/types"

// StatusStore is an in-memory record of the latest AssignmentStatus of each edge,
// keyed by edge ID. It holds the same information that the controller writes to
// the AssignmentStatus resource. Tests and the runtime can read an outcome
// without a cluster round trip. It is not persistent and not safe for
// concurrent use.
type StatusStore struct {
	Statuses map[string]types.AssignmentStatus
}

// NewStatusStore returns an empty store.
func NewStatusStore() *StatusStore {
	return &StatusStore{Statuses: make(map[string]types.AssignmentStatus)}
}

// Upsert inserts the status or replaces the entry with the same ID.
func (s *StatusStore) Upsert(status types.AssignmentStatus) {
	s.Statuses[status.ID] = status
}

// Get returns the stored status and whether the ID was present.
func (s *StatusStore) Get(id string) (types.AssignmentStatus, bool) {
	status, ok := s.Statuses[id]
	return status, ok
}
