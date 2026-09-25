package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"quantumtrustkg/controller/pkg/types"
)

// sparqlResults is the part of the SPARQL 1.1 JSON results format that the
// queries need. Each binding maps a variable name to a typed value, and an
// unbound OPTIONAL variable is absent from the map.
type sparqlResults struct {
	Results struct {
		Bindings []map[string]struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}

// CandidateProtocols reads the candidate profiles of an edge from the graph,
// together with the threat metadata that gates (ii) and (iii) and the fallback
// use. The edge ID is sanitised into an IRI local name. The threat fields are
// OPTIONAL. A profile with no recorded status has empty metadata. A
// missing security category defaults to hybrid and a missing or malformed
// overhead defaults to 0. Limitation: the query is a pattern over one edge, so
// several candidates with different categories on the same edge can yield
// mixed rows, since the write side stores the category on the edge and not on
// the profile.
func (c *Client) CandidateProtocols(ctx context.Context, edge types.CommunicationEdge) ([]types.ProtocolProfile, error) {
	edgeIRI := sanitizeIRI(edge.ID)
	query := fmt.Sprintf(`
SELECT ?name ?securityCategory ?overheadScore ?family ?proofStatus ?deprecated ?advisory ?hybridRobust
WHERE {
  :%s :candidateProfile ?name .
  OPTIONAL { :%s :candidateSecurityCategory ?securityCategory . }
  OPTIONAL { :%s :candidateOverheadScore ?overheadScore . }
  OPTIONAL { ?name :primitiveFamily ?family . }
  OPTIONAL { ?name :proofStatus ?proofStatus . }
  OPTIONAL { ?name :deprecated ?deprecated . }
  OPTIONAL { ?advisoryNode :targetsProfile ?name ; :advisoryId ?advisory . }
  OPTIONAL { ?name :hybridRobust ?hybridRobust . }
}`, edgeIRI, edgeIRI, edgeIRI)

	raw, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	var results sparqlResults
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, err
	}

	profiles := make([]types.ProtocolProfile, 0, len(results.Results.Bindings))
	for _, binding := range results.Results.Bindings {
		score := 0.0
		if rawScore, ok := binding["overheadScore"]; ok {
			if parsed, err := strconv.ParseFloat(rawScore.Value, 64); err == nil {
				score = parsed
			}
		}
		category := types.SecurityHybrid
		if rawCategory, ok := binding["securityCategory"]; ok && rawCategory.Value != "" {
			category = types.SecurityCategory(rawCategory.Value)
		}
		profile := types.ProtocolProfile{
			Name:             binding["name"].Value,
			SecurityCategory: category,
			OverheadScore:    score,
		}
		// Threat metadata for gates (ii) and (iii) and the fallback robustness flag.
		if v, ok := binding["family"]; ok {
			profile.Family = v.Value
		}
		if v, ok := binding["proofStatus"]; ok {
			profile.ProofStatus = v.Value
		}
		if v, ok := binding["deprecated"]; ok {
			profile.Deprecated = v.Value == "true"
		}
		if v, ok := binding["advisory"]; ok {
			profile.Advisory = v.Value
		}
		if v, ok := binding["hybridRobust"]; ok {
			profile.HybridRobust = v.Value == "true"
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// ActivePolicies reads the policies attached to an edge from the graph. It
// returns each policy name with its required class, which feeds the computation
// of Req(e). The AllowHybrid field is not read from the graph. A policy
// obtained here therefore never permits the hybrid fallback by itself.
func (c *Client) ActivePolicies(ctx context.Context, edge types.CommunicationEdge) ([]types.Policy, error) {
	edgeIRI := sanitizeIRI(edge.ID)
	query := fmt.Sprintf(`
SELECT ?name ?requiredLevel
WHERE {
  :%s :policyName ?name .
  :%s :policyRequiredLevel ?requiredLevel .
}`, edgeIRI, edgeIRI)

	raw, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	var results sparqlResults
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, err
	}

	policies := make([]types.Policy, 0, len(results.Results.Bindings))
	for _, binding := range results.Results.Bindings {
		policies = append(policies, types.Policy{
			Name:                     binding["name"].Value,
			RequiredSecurityCategory: types.SecurityCategory(binding["requiredLevel"].Value),
		})
	}
	return policies, nil
}
