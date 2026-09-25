// Tests of the SPARQL client against a stubbed HTTP transport (Sect. 3,
// graph-backed evidence).
//
// The controller reads candidate protocols and writes assignments through the
// Fuseki endpoints <dataset>/query and <dataset>/update. These tests replace the
// HTTP transport, which means no Fuseki instance is needed. They check the request path and
// body (the edge ID appears in the query, and the desired profile appears in the
// update) and that a SPARQL JSON result is decoded into a ProtocolProfile with the
// quantum_safe class.

package unit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/pkg/types"
)

// roundTripFunc adapts a function to http.RoundTripper. Other test files use it
// as well.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TestGraphClientCandidateProtocols checks the query path, the edge ID in the
// body and the decoding of one binding.
func TestGraphClientCandidateProtocols(t *testing.T) {
	client := graph.NewClient("http://graph.example/dataset")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/dataset/query" {
				t.Fatalf("expected /dataset/query, got %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), "authToPayment") {
				t.Fatal("expected edge id in query body")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
  "results": {
    "bindings": [
      {
        "name": {"type":"literal","value":"QuantumSafeProfile"},
        "securityCategory": {"type":"literal","value":"quantum_safe"},
        "overheadScore": {"type":"literal","value":"3.5"}
      }
    ]
  }
}`)),
				Header: make(http.Header),
			}, nil
		}),
	}

	profiles, err := client.CandidateProtocols(context.Background(), types.CommunicationEdge{ID: "authToPayment"})
	if err != nil {
		t.Fatalf("expected query success, got %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 protocol profile, got %d", len(profiles))
	}
	if profiles[0].Name != "QuantumSafeProfile" || profiles[0].SecurityCategory != types.SecurityQuantumSafe {
		t.Fatalf("unexpected profile %+v", profiles[0])
	}
}

// TestGraphClientUpsertEdgeIssuesUpdate checks that a promoted edge is written
// with an update request that names the desired profile.
func TestGraphClientUpsertEdgeIssuesUpdate(t *testing.T) {
	client := graph.NewClient("http://graph.example/dataset")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/dataset/update" {
				t.Fatalf("expected /dataset/update, got %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), "QuantumSafeProfile") {
				t.Fatal("expected desired profile in update body")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := client.UpsertEdge(context.Background(), types.CommunicationEdge{
		ID:             "authToPayment",
		DesiredProfile: "QuantumSafeProfile",
		State:          types.AssignmentPromoted,
		Reason:         "assignment-applied",
	})
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
}
