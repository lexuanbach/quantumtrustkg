// This file builds communication edges from the Turtle fixtures under
// implementation/semantics/fixtures. The fixtures replace a live Fuseki in the
// unit tests and in tools such as cmd/decision-trace and cmd/scale-stress, so
// the edges they produce feed the real QTPO code path.
//
// The parser is line based. It recognises the subject kinds Service, Policy,
// Protocol, Communication and Observation, and the predicates that the
// fixtures use. It is not a general Turtle parser. Edge construction follows
// Sect. 3. It attaches the policies that govern the edge, computes the required
// class Req(e) as the strongest of the attached policy classes, the boundary
// baseline and the PCI inference for regulated edges, and keeps as candidates
// the profiles that both endpoints advertise. A service with no advertised
// profile supports nothing, which is the fail-closed reading of missing
// capability evidence. Provenance for every edge is the single Observation of
// the fixture, with its source, its observedAt time and its freshUntil deadline
// and an optional conflict note.
//
// Limitation: the fixture path implements the strongest-class rule and does not
// implement the collapse of overriding policies by selector specificity that
// Sect. 3 mentions. The order of edges and of candidate profiles comes from Go
// map iteration and is not specified. Callers that need a fixed order sort the
// result. Because RankQTPO resolves score ties in favour of the first
// candidate, an exact tie between two candidates could be broken differently
// between runs.
package graph

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"quantumtrustkg/controller/pkg/types"
)

// fixtureModel is the intermediate result of parsing one Turtle fixture. The
// maps are keyed by the local name of the subject. observedAt, freshUntil,
// observedBy and conflictNote come from the Observation of the fixture and
// apply to every edge in it. namespace is fixed to finance.
type fixtureModel struct {
	policies        map[string]types.Policy
	profiles        map[string]types.ProtocolProfile
	edges           map[string]*types.CommunicationEdge
	serviceProfiles map[string]map[string]bool
	servicePolicies map[string][]string
	observedAt      time.Time
	freshUntil      time.Time
	observedBy      string
	conflictNote    string
	namespace       string
}

// BuildEdgesFromTTL parses the fixture and returns every communication edge in
// it, fully built with policies, required class, candidates and provenance. The
// order of the result is unspecified.
func BuildEdgesFromTTL(raw []byte) ([]types.CommunicationEdge, error) {
	model, err := parseFixture(raw)
	if err != nil {
		return nil, err
	}

	edges := make([]types.CommunicationEdge, 0, len(model.edges))
	for id := range model.edges {
		edge, err := buildEdge(model, id)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

// BuildFinanceDemoEdgeFromTTL parses the fixture and returns the single edge
// with the given ID, or an error when the ID is absent. It supports only the
// patterns that the repository's Turtle fixtures use.
func BuildFinanceDemoEdgeFromTTL(raw []byte, targetID string) (types.CommunicationEdge, error) {
	model, err := parseFixture(raw)
	if err != nil {
		return types.CommunicationEdge{}, err
	}

	return buildEdge(model, targetID)
}

// buildEdge assembles one edge from the parsed model. It attaches policies in
// three ways. It adds every policy bound to the edge's trust boundary and then
// the policies that the source and the destination service are subject to.
// The required class and its reason are then computed from all attached
// policies by inferEdgeRequirement. The candidate set is the profiles that
// both endpoints advertise. A default hybrid requirement applies when nothing
// sets one. The new edge is Pending and its LastEvaluatedAt is the
// observation time of the fixture.
func buildEdge(model *fixtureModel, targetID string) (types.CommunicationEdge, error) {
	edgeRef, ok := model.edges[targetID]
	if !ok {
		return types.CommunicationEdge{}, fmt.Errorf("edge %s not found in fixture", targetID)
	}

	edge := *edgeRef
	edge.Namespace = model.namespace
	edge.Provenance = types.EdgeProvenance{
		Source:       model.observedBy,
		ObservedAt:   model.observedAt,
		FreshUntil:   model.freshUntil,
		Confidence:   "high",
		ConflictNote: model.conflictNote,
	}

	for _, policy := range model.policies {
		if policy.TrustBoundaryPolicy == edge.TrustBoundary {
			edge.Policies = appendUniquePolicy(edge.Policies, policy)
		}
	}

	for _, policyName := range model.servicePolicies[edge.SourceService] {
		if policy, ok := model.policies[policyName]; ok {
			edge.Policies = appendUniquePolicy(edge.Policies, policy)
			edge.RequiredSecurityLevel = strongerCategory(edge.RequiredSecurityLevel, policy.RequiredSecurityCategory)
		}
	}
	for _, policyName := range model.servicePolicies[edge.DestinationService] {
		if policy, ok := model.policies[policyName]; ok {
			edge.Policies = appendUniquePolicy(edge.Policies, policy)
		}
	}
	edge.RequiredSecurityLevel, edge.RequirementReason = inferEdgeRequirement(edge)

	edge.SourceCapabilities = endpointCapabilities(model, edge.SourceService)
	edge.DestinationCapabilities = endpointCapabilities(model, edge.DestinationService)
	for name, profile := range model.profiles {
		if supportsBoth(model, edge.SourceService, edge.DestinationService, name) {
			edge.CandidateProfiles = append(edge.CandidateProfiles, profile)
		}
	}

	if edge.RequiredSecurityLevel == "" {
		edge.RequiredSecurityLevel = types.SecurityHybrid
	}
	edge.State = types.AssignmentPending
	edge.LastEvaluatedAt = model.observedAt
	return edge, nil
}

// inferEdgeRequirement computes Req(e) and a comma-separated reason string. The
// requirement is the strongest of three sources. The first is the required
// class of each attached policy, where a policy weaker than what has been
// collected so far is skipped and adds no reason. The second is the PCI
// inference, which forces quantum_safe on a regulated edge under a PCI policy.
// The third is the baseline of the trust boundary, as boundary_requirement in
// formal/coq/Model.v. If nothing applies the result is hybrid with the reason
// default:hybrid. Taking the maximum means a permissive policy cannot weaken a
// stricter one.
func inferEdgeRequirement(edge types.CommunicationEdge) (types.SecurityCategory, string) {
	required := types.SecurityCategory("")
	reasons := make([]string, 0, 4)

	for _, policy := range edge.Policies {
		if policy.RequiredSecurityCategory != "" {
			if rankCategory(policy.RequiredSecurityCategory) < rankCategory(required) {
				continue
			}
			required = strongerCategory(required, policy.RequiredSecurityCategory)
			reasons = append(reasons, "policy:"+policy.Name)
		}
	}

	if triggersRegulatedQuantumSafe(edge) {
		required = strongerCategory(required, types.SecurityQuantumSafe)
		reasons = append(reasons, "inference:pci-regulated-edge")
	}

	if boundary := boundaryCategory(edge.TrustBoundary); boundary != "" {
		required = strongerCategory(required, boundary)
		reasons = append(reasons, "boundary:"+edge.TrustBoundary)
	}

	if required == "" {
		required = types.SecurityHybrid
		reasons = append(reasons, "default:hybrid")
	}

	return required, strings.Join(reasons, ",")
}

// appendUniquePolicy appends the policy unless one with the same name is present.
func appendUniquePolicy(existing []types.Policy, policy types.Policy) []types.Policy {
	for _, current := range existing {
		if current.Name == policy.Name {
			return existing
		}
	}
	return append(existing, policy)
}

// triggersRegulatedQuantumSafe holds on a regulated edge when an attached
// policy lists the PCI-DSS obligation or has pci in its name. It lets a PCI
// policy raise the requirement to quantum_safe even if the policy itself asks
// for less.
func triggersRegulatedQuantumSafe(edge types.CommunicationEdge) bool {
	if edge.TrustBoundary != "regulated" {
		return false
	}
	for _, policy := range edge.Policies {
		for _, obligation := range policy.Compliance {
			if obligation == "PCI-DSS" {
				return true
			}
		}
		if strings.Contains(strings.ToLower(policy.Name), "pci") {
			return true
		}
	}
	return false
}

// parseFixture reads the fixture line by line. A line that starts a subject
// and declares its kind selects the parser for the following property lines
// until the next subject. Prefix, comment and blank lines are skipped. The
// namespace defaults to finance.
func parseFixture(raw []byte) (*fixtureModel, error) {
	lines := strings.Split(string(raw), "\n")
	model := &fixtureModel{
		policies:        make(map[string]types.Policy),
		profiles:        make(map[string]types.ProtocolProfile),
		edges:           make(map[string]*types.CommunicationEdge),
		serviceProfiles: make(map[string]map[string]bool),
		servicePolicies: make(map[string][]string),
		namespace:       "finance",
	}

	var current string
	var currentKind string

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "@prefix") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, ":") && strings.Contains(line, " a :") {
			name := strings.TrimPrefix(strings.SplitN(line, " ", 2)[0], ":")
			current = name
			switch {
			case strings.Contains(line, " a :Policy"):
				currentKind = "policy"
				model.policies[current] = types.Policy{Name: current, Namespace: model.namespace}
			case strings.Contains(line, " a :Protocol"):
				currentKind = "protocol"
				model.profiles[current] = types.ProtocolProfile{Name: current}
			case strings.Contains(line, " a :Communication"):
				currentKind = "edge"
				model.edges[current] = &types.CommunicationEdge{ID: current}
			case strings.Contains(line, " a :Observation"):
				currentKind = "observation"
			case strings.Contains(line, " a :Service"):
				currentKind = "service"
				model.serviceProfiles[current] = make(map[string]bool)
				model.servicePolicies[current] = nil
			default:
				currentKind = ""
			}
			continue
		}
		if current == "" {
			continue
		}

		switch currentKind {
		case "policy":
			parsePolicyLine(model, current, line)
		case "protocol":
			parseProtocolLine(model, current, line)
		case "edge":
			parseEdgeLine(model, current, line)
		case "observation":
			parseObservationLine(model, line)
		case "service":
			parseServiceLine(model, current, line)
		}
	}

	return model, nil
}

// parseServiceLine reads :subjectTo (the policy a service is subject to) and
// :supportsProfile (the capability evidence of the service, a comma-separated
// list of profiles).
func parseServiceLine(model *fixtureModel, current, line string) {
	if strings.Contains(line, ":subjectTo") {
		resource := extractResourceFromPredicate(line, ":subjectTo")
		if resource != "" {
			model.servicePolicies[current] = append(model.servicePolicies[current], resource)
		}
	}
	if strings.Contains(line, ":supportsProfile") {
		for _, resource := range extractResources(line) {
			if model.serviceProfiles[current] == nil {
				model.serviceProfiles[current] = make(map[string]bool)
			}
			model.serviceProfiles[current][resource] = true
		}
	}
}

// parsePolicyLine reads :requiresLevel. The class also determines the boundary
// the policy is bound to, with quantum_safe bound to regulated and the other
// classes to internal. The policy named PCIPolicy is given the PCI-DSS
// obligation and AllowHybrid. It therefore permits the guarded fallback in the
// fixtures. That is a property of this parser and differs from the pci_policy
// of formal/coq/Examples.v, which forbids the fallback.
func parsePolicyLine(model *fixtureModel, current, line string) {
	if strings.Contains(line, ":requiresLevel") {
		value := extractQuoted(line)
		policy := model.policies[current]
		policy.RequiredSecurityCategory = types.SecurityCategory(value)
		policy.TrustBoundaryPolicy = map[string]string{
			"quantum_safe": "regulated",
			"hybrid":       "internal",
			"classical":    "internal",
		}[value]
		if current == "PCIPolicy" {
			policy.Compliance = []string{"PCI-DSS"}
			policy.AllowHybrid = true
		}
		model.policies[current] = policy
	}
}

// parseProtocolLine reads the class and the overhead of a profile and the
// threat metadata of gates (ii) and (iii) (primitive family, proof status,
// deprecation, active advisory) and the hybridRobust flag.
func parseProtocolLine(model *fixtureModel, current, line string) {
	profile := model.profiles[current]
	if strings.Contains(line, ":hasSecurityCategory") {
		profile.SecurityCategory = types.SecurityCategory(extractQuoted(line))
	}
	if strings.Contains(line, ":hasOverheadScore") {
		value := extractQuoted(line)
		if score, err := strconv.ParseFloat(value, 64); err == nil {
			profile.OverheadScore = score
		}
	}
	// Threat metadata read by gates (ii) and (iii) and the fallback robustness flag.
	if strings.Contains(line, ":primitiveFamily") {
		profile.Family = extractQuoted(line)
	}
	if strings.Contains(line, ":proofStatus") {
		profile.ProofStatus = extractQuoted(line)
	}
	if strings.Contains(line, ":deprecated") {
		profile.Deprecated = strings.EqualFold(literalValue(line, ":deprecated"), "true")
	}
	if strings.Contains(line, ":activeAdvisory") {
		profile.Advisory = extractQuoted(line)
	}
	if strings.Contains(line, ":hybridRobust") {
		profile.HybridRobust = strings.EqualFold(literalValue(line, ":hybridRobust"), "true")
	}
	model.profiles[current] = profile
}

// literalValue returns the object of a predicate. A quoted literal wins, and
// otherwise the bare token after the predicate is returned. Both true and
// "true" are accepted.
func literalValue(line, predicate string) string {
	if quoted := extractQuoted(line); quoted != "" {
		return quoted
	}
	idx := strings.Index(line, predicate)
	if idx < 0 {
		return ""
	}
	value := strings.TrimSpace(line[idx+len(predicate):])
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ";")
	return strings.TrimSpace(value)
}

// parseEdgeLine reads the endpoints, the boundary the edge crosses and the
// profile currently assigned to the edge, which becomes ActiveProfile.
func parseEdgeLine(model *fixtureModel, current, line string) {
	edge := model.edges[current]
	if strings.Contains(line, ":sourceService") {
		edge.SourceService = extractResource(line)
	}
	if strings.Contains(line, ":destinationService") {
		edge.DestinationService = extractResource(line)
	}
	if strings.Contains(line, ":crossesBoundary") {
		edge.TrustBoundary = extractQuoted(line)
	}
	if strings.Contains(line, ":assignedProtocol") {
		edge.ActiveProfile = extractResource(line)
	}
	model.edges[current] = edge
}

// parseObservationLine reads the provenance of the fixture: the observer, the
// observation time and the freshness deadline as RFC 3339 timestamps, and an
// optional conflict note. An unparsable timestamp leaves the time zero, and a
// zero freshUntil later fails the freshness gate.
func parseObservationLine(model *fixtureModel, line string) {
	if strings.Contains(line, ":observedBy") {
		model.observedBy = extractQuoted(line)
	}
	if strings.Contains(line, ":observedAt") {
		if ts, err := time.Parse(time.RFC3339, extractQuoted(line)); err == nil {
			model.observedAt = ts
		}
	}
	if strings.Contains(line, ":freshUntil") {
		if ts, err := time.Parse(time.RFC3339, extractQuoted(line)); err == nil {
			model.freshUntil = ts
		}
	}
	if strings.Contains(line, ":conflictNote") {
		model.conflictNote = extractQuoted(line)
	}
}

// extractQuoted returns the first double-quoted string on the line, or "".
func extractQuoted(line string) string {
	start := strings.Index(line, "\"")
	if start == -1 {
		return ""
	}
	end := strings.Index(line[start+1:], "\"")
	if end == -1 {
		return ""
	}
	return line[start+1 : start+1+end]
}

// extractResource returns the local name after the last colon of the line.
func extractResource(line string) string {
	idx := strings.LastIndex(line, ":")
	if idx == -1 {
		return ""
	}
	value := strings.TrimSpace(line[idx+1:])
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ";")
	return strings.TrimSpace(value)
}

// extractResources returns the local names in a :supportsProfile object list.
func extractResources(line string) []string {
	idx := strings.Index(line, ":supportsProfile")
	if idx == -1 {
		return nil
	}
	value := strings.TrimSpace(line[idx+len(":supportsProfile"):])
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ";")
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, ":")
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// extractResourceFromPredicate returns the local name that follows the predicate.
func extractResourceFromPredicate(line, predicate string) string {
	idx := strings.Index(line, predicate)
	if idx == -1 {
		return ""
	}
	value := strings.TrimSpace(line[idx+len(predicate):])
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ";")
	value = strings.TrimPrefix(strings.TrimSpace(value), ":")
	return strings.TrimSpace(value)
}

// supportsBoth reports whether both endpoints advertise the profile. It fails
// closed. An endpoint with no advertised profile supports nothing, in line with
// supports_profileb in the Coq model.
func supportsBoth(model *fixtureModel, src, dst, profile string) bool {
	srcProfiles := model.serviceProfiles[src]
	dstProfiles := model.serviceProfiles[dst]
	if len(srcProfiles) == 0 || len(dstProfiles) == 0 {
		return false
	}
	return srcProfiles[profile] && dstProfiles[profile]
}

// endpointCapabilities returns the capability evidence of a service from its
// :supportsProfile set, with the profiles sorted by name. Known is false when
// the service advertises none, and CompatibleProfiles then reports the edge as
// missing capability. Fixtures carry no capability timestamps. Stale is
// never set here.
func endpointCapabilities(model *fixtureModel, service string) types.EndpointCapabilities {
	set := model.serviceProfiles[service]
	if len(set) == 0 {
		return types.EndpointCapabilities{}
	}
	profiles := make([]string, 0, len(set))
	for name := range set {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)
	return types.EndpointCapabilities{Known: true, Profiles: profiles, Source: "graph:supportsProfile"}
}

// strongerCategory returns the higher of two classes and returns a on equal rank.
func strongerCategory(a, b types.SecurityCategory) types.SecurityCategory {
	if rankCategory(b) > rankCategory(a) {
		return b
	}
	return a
}

// rankCategory is the class order used to compare requirements. Unknown or empty
// classes rank 0.
func rankCategory(category types.SecurityCategory) int {
	switch category {
	case types.SecurityQuantumSafe:
		return 3
	case types.SecurityHybrid:
		return 2
	case types.SecurityClassical:
		return 1
	default:
		return 0
	}
}

// boundaryCategory is the baseline class of a trust boundary. Regulated
// boundaries need quantum_safe and internal, partner and cross-zone boundaries
// need hybrid, as boundary_requirement in formal/coq/Model.v. An unlabelled or
// unknown boundary adds no baseline.
func boundaryCategory(boundary string) types.SecurityCategory {
	switch strings.ToLower(strings.TrimSpace(boundary)) {
	case "regulated", "external-regulated":
		return types.SecurityQuantumSafe
	case "partner", "external", "cross-zone", "internal":
		return types.SecurityHybrid
	default:
		return ""
	}
}
