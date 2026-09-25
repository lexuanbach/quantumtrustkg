package graph

import (
	"context"
	"fmt"
	"strings"

	"quantumtrustkg/controller/pkg/types"
)

// UpsertEdge writes the assignment outcome of an edge back into the graph. It
// sends one update that first deletes the previous desired and active profile,
// required class, requirement reason, state and reason of the edge and then
// inserts the new values. The write-back gives the graph an auditable record of
// the decision. No query in this package reads these facts back.
func (c *Client) UpsertEdge(ctx context.Context, edge types.CommunicationEdge) error {
	update := fmt.Sprintf(`
PREFIX : <https://quantumtrustkg.example/schema#>
DELETE WHERE {
  :%s :desiredProfile ?p .
  :%s :activeProfile ?ap .
  :%s :requiredSecurityLevel ?l .
  :%s :requirementReason ?rr .
  :%s :state ?s .
  :%s :reason ?r .
} ;
INSERT DATA {
  :%s :desiredProfile "%s" .
  :%s :activeProfile "%s" .
  :%s :requiredSecurityLevel "%s" .
  :%s :requirementReason "%s" .
  :%s :state "%s" .
  :%s :reason "%s" .
}`, sanitizeIRI(edge.ID), sanitizeIRI(edge.ID), sanitizeIRI(edge.ID), sanitizeIRI(edge.ID), sanitizeIRI(edge.ID), sanitizeIRI(edge.ID), sanitizeIRI(edge.ID), escapeLiteral(edge.DesiredProfile), sanitizeIRI(edge.ID), escapeLiteral(edge.ActiveProfile), sanitizeIRI(edge.ID), escapeLiteral(string(edge.RequiredSecurityLevel)), sanitizeIRI(edge.ID), escapeLiteral(edge.RequirementReason), sanitizeIRI(edge.ID), escapeLiteral(string(edge.State)), sanitizeIRI(edge.ID), escapeLiteral(edge.Reason))

	return c.Update(ctx, update)
}

// SyncClusterSnapshot inserts the edges of the cluster into the graph as facts.
// For each edge it writes the endpoints, the trust boundary, the observation
// source, the required class and its reason, the active profile, each candidate
// profile with its class and overhead, and each attached policy. The statement
// is an INSERT DATA and deletes nothing. It adds facts and does not replace
// older ones. It does nothing for an empty list.
func (c *Client) SyncClusterSnapshot(ctx context.Context, edges []types.CommunicationEdge) error {
	if len(edges) == 0 {
		return nil
	}

	var builder strings.Builder
	builder.WriteString("PREFIX : <https://quantumtrustkg.example/schema#>\nINSERT DATA {\n")
	for _, edge := range edges {
		builder.WriteString(fmt.Sprintf("  :%s :sourceService \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(edge.SourceService)))
		builder.WriteString(fmt.Sprintf("  :%s :destinationService \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(edge.DestinationService)))
		builder.WriteString(fmt.Sprintf("  :%s :trustBoundary \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(edge.TrustBoundary)))
		builder.WriteString(fmt.Sprintf("  :%s :observedFrom \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(edge.Provenance.Source)))
		builder.WriteString(fmt.Sprintf("  :%s :requiredSecurityLevel \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(string(edge.RequiredSecurityLevel))))
		builder.WriteString(fmt.Sprintf("  :%s :requirementReason \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(edge.RequirementReason)))
		builder.WriteString(fmt.Sprintf("  :%s :activeProfile \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(edge.ActiveProfile)))
		for _, profile := range edge.CandidateProfiles {
			builder.WriteString(fmt.Sprintf("  :%s :candidateProfile \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(profile.Name)))
			builder.WriteString(fmt.Sprintf("  :%s :candidateSecurityCategory \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(string(profile.SecurityCategory))))
			builder.WriteString(fmt.Sprintf("  :%s :candidateOverheadScore \"%0.3f\" .\n", sanitizeIRI(edge.ID), profile.OverheadScore))
		}
		for _, policy := range edge.Policies {
			builder.WriteString(fmt.Sprintf("  :%s :policyName \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(policy.Name)))
			builder.WriteString(fmt.Sprintf("  :%s :policyRequiredLevel \"%s\" .\n", sanitizeIRI(edge.ID), escapeLiteral(string(policy.RequiredSecurityCategory))))
		}
	}
	builder.WriteString("}\n")
	return c.Update(ctx, builder.String())
}

// sanitizeIRI maps an identifier to a legal IRI local name by replacing spaces,
// slashes, colons and hyphens with underscores.
func sanitizeIRI(value string) string {
	replacer := strings.NewReplacer(" ", "_", "/", "_", ":", "_", "-", "_")
	return replacer.Replace(value)
}

// escapeLiteral escapes double quotes so a value can sit inside a SPARQL string
// literal. It does not escape backslashes or line breaks.
func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}
