// Package graph is the access layer to the Quantum Trust Graph of Sect. 3
// (Graph-backed evidence). The graph holds facts as RDF in Apache Jena Fuseki,
// and this package reads them with SPARQL and writes assignment outcomes back.
// It has three parts. The client in this file speaks the SPARQL protocol over
// HTTP. queries.go and writers.go contain the SPARQL text for reading candidate
// profiles and policies and for writing assignments. fixtures.go parses the
// Turtle fixtures that drive the tests and the evaluation harnesses without a
// running Fuseki. validation.go applies the evidence checks that need only the
// edge record, namely freshness and contradiction.
//
// Every fact carries a source, an observation time and a freshness window, and
// a stale or contradicted fact blocks a new promotion (gate (iv)). Kubernetes
// remains the source of truth, and the paper argues that a diverged graph read
// therefore cannot cause a downgrade.
//
// Limitations. The update statements in writers.go use the namespace
// https://quantumtrustkg.example/schema#, whereas the fixtures and the ontology
// under implementation/semantics use http://quantumtrustkg.io/ontology#. The
// query text in queries.go declares no PREFIX. The Turtle parser in fixtures.go
// is line based and understands only the patterns that the fixtures use.
package graph

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a Fuseki-compatible SPARQL endpoint over HTTP. Endpoint is
// the dataset URL, and the client appends /query and /update when they are not
// already present. Requests time out after five seconds. There is no retry, so
// the caller decides how to treat an error. For a decision the caller treats an
// unreachable graph as unavailable, which blocks promotion.
type Client struct {
	Endpoint   string
	HTTPClient *http.Client
}

// NewClient returns a client for the given dataset URL with a 5 s timeout.
func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Ping checks that the endpoint answers a GET with a status below 300. It backs
// the graph-availability gate, the field st_graph_available of the Coq model.
// An empty endpoint is an error.
func (c *Client) Ping(ctx context.Context) error {
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("graph endpoint is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("graph endpoint ping failed: %s", resp.Status)
	}
	return nil
}

// Query runs a SPARQL SELECT and returns the raw application/sparql-results+json
// body. It sends the query as a form-encoded POST.
func (c *Client) Query(ctx context.Context, sparql string) ([]byte, error) {
	values := url.Values{}
	values.Set("query", sparql)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.queryEndpoint(), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")

	return c.do(req)
}

// Update runs a SPARQL update, with the statement sent as a form-encoded POST.
// The response body is discarded.
func (c *Client) Update(ctx context.Context, sparql string) error {
	values := url.Values{}
	values.Set("update", sparql)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.updateEndpoint(), strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	_, err = c.do(req)
	return err
}

// do sends the request, reads the whole body and turns any status of 300 or
// above into an error that includes the trimmed body.
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("graph request failed: %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

// queryEndpoint is the URL of the query service.
func (c *Client) queryEndpoint() string {
	return appendPath(c.Endpoint, "query")
}

// updateEndpoint is the URL of the update service.
func (c *Client) updateEndpoint() string {
	return appendPath(c.Endpoint, "update")
}

// appendPath adds suffix to base unless base already ends with it.
func appendPath(base, suffix string) string {
	if strings.HasSuffix(base, "/"+suffix) {
		return base
	}
	return strings.TrimRight(base, "/") + "/" + suffix
}
