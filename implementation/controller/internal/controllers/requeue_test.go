package controllers

import (
	"sync"
	"testing"
	"time"

	"quantumtrustkg/controller/pkg/types"
)

func TestResultForAtSchedulesEvidenceExpiry(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	edges := []types.CommunicationEdge{{
		State:      types.AssignmentPromoted,
		Provenance: types.EdgeProvenance{FreshUntil: now.Add(17 * time.Second)},
	}}
	if got := resultForAt(edges, now).RequeueAfter; got != 17*time.Second {
		t.Fatalf("expected expiry requeue after 17s, got %s", got)
	}
}

func TestResultForAtUsesEarlierReadinessPoll(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	edges := []types.CommunicationEdge{{
		State:      types.AssignmentApplying,
		Provenance: types.EdgeProvenance{FreshUntil: now.Add(time.Minute)},
	}}
	if got := resultForAt(edges, now).RequeueAfter; got != readinessRequeue {
		t.Fatalf("expected readiness requeue after %s, got %s", readinessRequeue, got)
	}
}

func TestEvidenceRevisionDetectsConcurrentUpdate(t *testing.T) {
	state := NewResourceState()
	before := state.SnapshotRevision()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		state.UpsertDeprecation("ML-KEM-768", "new-advisory")
	}()
	wg.Wait()
	if after := state.SnapshotRevision(); after <= before {
		t.Fatalf("concurrent evidence update did not advance revision: before=%d after=%d", before, after)
	}
}
