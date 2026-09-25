// Command staleness-sweep is the failure-injection experiment behind Fig. 2(b)
// of the paper (RQ2, Sect. 4.2). It measures how QTPO and its provenance-blind
// ablation behave when a controlled share of the edges carry stale or
// contradictory facts, and it checks the fail-closed guarantee (Thm. 2, G4) on
// an experiment of that shape.
//
// The harness reuses the seeded synthetic mesh model of cmd/baseline-compare and
// the real orchestration.SelectProfile path. Every edge starts with fresh
// provenance. injectStaleness then corrupts an exact fraction of the edges, half
// stale and half contradictory, chosen by a seeded permutation. The default sweep
// is 0, 5, 10, 25, 50, 75 and 90% over 30 repetitions of a 200-service mesh. The
// sub-seed of repetition r is seed + 7919*r + services. For each fraction it
// reports two methods on identical copies of the corrupted mesh:
//
//	QTKG  QTPO with the provenance gate. Unsafe promotions must stay at zero at
//	      every fraction. Rising staleness only turns promotions into blocks.
//	NoKG  the same filters and ranking without the freshness and contradiction
//	      gate. Its unsafe rate follows the injected fraction, because it cannot
//	      tell a corrupted fact from a good one.
//
// The compliance column is the declared-label compliance defined in Sect. 4. It
// falls with the corruption level because QTPO refuses evidence it cannot trust,
// and it does not measure the cryptography on the wire.
//
// Scenarios and result files:
//
//	synthetic       200-service synthetic mesh   staleness-sweep-results.csv
//	real-topology   the sampled Alibaba topology with the requested number of
//	                services (--services, --topodir)   staleness-sweep-real-alibaba.csv
//
// The harness needs no Fuseki, Kubernetes or Istio, which means it is deterministic from
// the seed (20260509 in the paper).
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/pkg/types"
)

// rank maps a security class to an integer: 0 classical, 1 hybrid, 2 quantum-safe.
func rank(c types.SecurityCategory) int {
	switch c {
	case types.SecurityQuantumSafe:
		return 2
	case types.SecurityHybrid:
		return 1
	default:
		return 0
	}
}

// classForRank is the inverse of rank.
func classForRank(r int) types.SecurityCategory {
	switch {
	case r >= 2:
		return types.SecurityQuantumSafe
	case r == 1:
		return types.SecurityHybrid
	default:
		return types.SecurityClassical
	}
}

// genEdge is a generated edge with the ground truth the evaluators need. It is
// the same structure as in cmd/baseline-compare. The dstService field is only
// filled in here, because no service-level baseline runs in this sweep.
type genEdge struct {
	edge        types.CommunicationEdge
	deprecated  map[string]bool
	dstService  string
	support     int
	provBad     bool
	req         int
	allowHybrid bool
}

// profilesUpTo returns one profile per class up to tier, with unique names per edge.
func profilesUpTo(tier, idx int) []types.ProtocolProfile {
	all := []types.ProtocolProfile{
		{Name: fmt.Sprintf("classical-%d", idx), SecurityCategory: types.SecurityClassical, OverheadScore: 0.8},
		{Name: fmt.Sprintf("hybrid-%d", idx), SecurityCategory: types.SecurityHybrid, OverheadScore: 1.1, HybridRobust: true},
		{Name: fmt.Sprintf("quantum-%d", idx), SecurityCategory: types.SecurityQuantumSafe, OverheadScore: 1.4},
	}
	out := make([]types.ProtocolProfile, 0, tier+1)
	for _, p := range all {
		if rank(p.SecurityCategory) <= tier {
			out = append(out, p)
		}
	}
	return out
}

// generateMesh mirrors the mesh model of cmd/baseline-compare (capability tiers,
// one to three inbound edges per service, regulated and partner edges). The
// difference is that every edge keeps fresh provenance, because the corruption
// is injected separately at an exact fraction by injectStaleness.
func generateMesh(n int, seed int64) []genEdge {
	rng := rand.New(rand.NewSource(seed))
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	tier := make([]int, n)
	for i := range tier {
		r := rng.Float64()
		switch {
		case r < 0.08:
			tier[i] = 0
		case r < 0.45:
			tier[i] = 1
		default:
			tier[i] = 2
		}
	}

	pickSource := func(dst, maxTier int) int {
		for tries := 0; tries < 8; tries++ {
			s := rng.Intn(n)
			if s != dst && tier[s] <= maxTier {
				return s
			}
		}
		s := rng.Intn(n)
		if s == dst {
			s = (s + 1) % n
		}
		return s
	}

	edges := make([]genEdge, 0, 2*n)
	idx := 0
	for dst := 0; dst < n; dst++ {
		fanIn := 1 + rng.Intn(3)
		forcePartner := tier[dst] == 2 && rng.Float64() < 0.40
		for k := 0; k < fanIn; k++ {
			idx++
			var src int
			var boundary string
			var req int

			isPartnerEdge := forcePartner && k == 0
			switch {
			case isPartnerEdge:
				src = pickSource(dst, 1)
				boundary, req = "partner", 1
			default:
				src = pickSource(dst, 2)
				srcT, dstT := tier[src], tier[dst]
				sup := srcT
				if dstT < sup {
					sup = dstT
				}
				switch {
				case sup == 2 && rng.Float64() < 0.45:
					boundary, req = "regulated", 2
				case sup >= 1 && rng.Float64() < 0.5:
					boundary, req = "internal", 1
				default:
					boundary, req = "internal", 0
				}
			}

			srcTier, dstTier := tier[src], tier[dst]
			support := srcTier
			if dstTier < support {
				support = dstTier
			}
			if support < req && rng.Float64() < 0.5 {
				support = req
			}
			candidates := profilesUpTo(support, idx)
			allowHybrid := rng.Float64() < 0.6

			e := types.CommunicationEdge{
				ID:                    fmt.Sprintf("edge-%05d", idx),
				TrustBoundary:         boundary,
				RequiredSecurityLevel: classForRank(req),
				CandidateProfiles:     candidates,
				Policies: []types.Policy{{
					Name:        "transition",
					AllowHybrid: allowHybrid,
				}},
				Provenance: types.EdgeProvenance{
					Source:     "synthetic",
					ObservedAt: now.Add(-time.Minute),
					FreshUntil: now.Add(5 * time.Minute),
				},
			}
			edges = append(edges, genEdge{
				edge:        e,
				deprecated:  map[string]bool{},
				dstService:  fmt.Sprintf("svc-%04d", dst),
				support:     support,
				provBad:     false,
				req:         req,
				allowHybrid: allowHybrid,
			})
		}
	}
	return edges
}

// meshFromTopology builds a mesh over a real service-dependency topology, given
// as index pairs from the Alibaba trace subset. Only the topology is real. The
// requirements come from the same calibrated model as generateMesh, and every
// edge starts fresh so that injectStaleness alone sets the corruption level.
func meshFromTopology(topo [][2]int, seed int64) []genEdge {
	rng := rand.New(rand.NewSource(seed))
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	n := 0
	for _, e := range topo {
		if e[0]+1 > n {
			n = e[0] + 1
		}
		if e[1]+1 > n {
			n = e[1] + 1
		}
	}
	tier := make([]int, n)
	regulated := make([]bool, n)
	for i := range tier {
		switch r := rng.Float64(); {
		case r < 0.08:
			tier[i] = 0
		case r < 0.45:
			tier[i] = 1
		default:
			tier[i] = 2
		}
		regulated[i] = rng.Float64() < 0.30
	}
	edges := make([]genEdge, 0, len(topo))
	for idx, e := range topo {
		src, dst := e[0], e[1]
		support := tier[src]
		if tier[dst] < support {
			support = tier[dst]
		}
		var req int
		var boundary string
		switch {
		case regulated[dst] && support == 2:
			boundary, req = "regulated", 2
		case regulated[dst]:
			boundary, req = "partner", 1
		case support >= 1 && rng.Float64() < 0.5:
			boundary, req = "internal", 1
		default:
			boundary, req = "internal", 0
		}
		if support < req && rng.Float64() < 0.5 {
			support = req
		}
		candidates := profilesUpTo(support, idx)
		allowHybrid := rng.Float64() < 0.6
		ce := types.CommunicationEdge{
			ID:                    fmt.Sprintf("edge-%05d", idx),
			TrustBoundary:         boundary,
			RequiredSecurityLevel: classForRank(req),
			CandidateProfiles:     candidates,
			Policies:              []types.Policy{{Name: "transition", AllowHybrid: allowHybrid}},
			Provenance: types.EdgeProvenance{
				Source:     "real-topology",
				ObservedAt: now.Add(-time.Minute),
				FreshUntil: now.Add(5 * time.Minute),
			},
		}
		edges = append(edges, genEdge{
			edge:        ce,
			deprecated:  map[string]bool{},
			dstService:  fmt.Sprintf("svc-%d", dst),
			support:     support,
			provBad:     false,
			req:         req,
			allowHybrid: allowHybrid,
		})
	}
	return edges
}

// loadTopologies reads the *.edges files ("src dst" service names) in dir, remaps
// names to dense indices and returns a map from service count to edge list.
// Files whose name contains "full" hold the unsampled parent graph and are
// skipped.
func loadTopologies(dir string) map[int][][2]int {
	out := map[int][][2]int{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.edges"))
	for _, fn := range files {
		if strings.Contains(filepath.Base(fn), "full") {
			continue
		}
		data, err := os.ReadFile(fn)
		if err != nil {
			continue
		}
		idxOf := map[string]int{}
		var topo [][2]int
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) != 2 {
				continue
			}
			for _, name := range f {
				if _, ok := idxOf[name]; !ok {
					idxOf[name] = len(idxOf)
				}
			}
			topo = append(topo, [2]int{idxOf[f[0]], idxOf[f[1]]})
		}
		if len(idxOf) > 0 {
			out[len(idxOf)] = topo
		}
	}
	return out
}

// injectStaleness marks exactly round(fraction*len(edges)) edges as corrupted.
// A seeded permutation picks the subset, which means the same fraction and seed always hit
// the same edges. A chosen edge with an even index is made stale, and one with an
// odd index gets a contradiction note.
func injectStaleness(edges []genEdge, fraction float64, seed int64, now time.Time) {
	n := len(edges)
	target := int(float64(n)*fraction + 0.5)
	if target > n {
		target = n
	}
	order := rng_perm(n, seed)
	for i := 0; i < target; i++ {
		g := &edges[order[i]]
		g.provBad = true
		if order[i]%2 == 0 {
			g.edge.Provenance.FreshUntil = now.Add(-time.Minute) // stale
			g.edge.Provenance.ConflictNote = ""
		} else {
			g.edge.Provenance.ConflictNote = "injected contradictory capability" // contradictory
		}
	}
}

// rng_perm returns a seeded random permutation of 0..n-1.
func rng_perm(n int, seed int64) []int {
	return rand.New(rand.NewSource(seed)).Perm(n)
}

// outcome counts how a method handled the edges of one mesh.
type outcome struct {
	compliant       int
	blockedSafe     int
	unsafePromotion int
	managed         int
}

// pct returns x as a percentage of the managed edges.
func (o outcome) pct(x int) float64 {
	if o.managed == 0 {
		return 0
	}
	return 100 * float64(x) / float64(o.managed)
}

// metadataValid is the controller's pre-selection check, which is the freshness
// and contradiction test of gate (iv).
func metadataValid(g genEdge, now time.Time) bool {
	if g.edge.Provenance.FreshUntil.IsZero() || now.After(g.edge.Provenance.FreshUntil) {
		return false
	}
	if g.edge.Provenance.ConflictNote != "" {
		return false
	}
	return true
}

// classifyPromotion says whether a promotion is compliant. It is if its class
// reaches Req(e), or if it is a hybrid profile chosen by a fallback that the
// edge's policy permits.
func classifyPromotion(chosen types.ProtocolProfile, g genEdge, permittedFallback bool) bool {
	if rank(chosen.SecurityCategory) >= g.req {
		return true
	}
	return permittedFallback && chosen.SecurityCategory == types.SecurityHybrid && g.allowHybrid
}

// profileByName finds a profile in a candidate list.
func profileByName(ps []types.ProtocolProfile, name string) (types.ProtocolProfile, bool) {
	for _, p := range ps {
		if p.Name == name {
			return p, true
		}
	}
	return types.ProtocolProfile{}, false
}

// evalQTPO runs the provenance-gated selection. An edge with stale or
// contradictory facts is blocked before selection (fail closed).
func evalQTPO(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		if !metadataValid(g, now) {
			o.blockedSafe++
			continue
		}
		sel, ok := orchestration.SelectProfile(g.edge, g.deprecated)
		if !ok {
			o.blockedSafe++
			continue
		}
		chosen, found := profileByName(g.edge.CandidateProfiles, sel.DesiredProfile)
		if !found {
			o.blockedSafe++
			continue
		}
		if classifyPromotion(chosen, g, sel.Reason == "policy-permitted-hybrid-fallback") {
			o.compliant++
		} else {
			o.unsafePromotion++
		}
	}
	return o
}

// evalNoKG runs the same filters and ranking without the provenance check. A
// promotion that rests on stale or contradictory facts counts as unsafe.
func evalNoKG(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		sel, ok := orchestration.SelectProfile(g.edge, g.deprecated)
		if !ok {
			o.blockedSafe++
			continue
		}
		chosen, found := profileByName(g.edge.CandidateProfiles, sel.DesiredProfile)
		if !found {
			o.blockedSafe++
			continue
		}
		if g.provBad {
			o.unsafePromotion++
			continue
		}
		if classifyPromotion(chosen, g, sel.Reason == "policy-permitted-hybrid-fallback") {
			o.compliant++
		} else {
			o.unsafePromotion++
		}
	}
	return o
}

// parseFractions parses the comma-separated --fractions list. A bad entry ends
// the program.
func parseFractions(s string) []float64 {
	var out []float64
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad fraction %q: %v\n", tok, err)
			os.Exit(1)
		}
		out = append(out, v)
	}
	return out
}

func main() {
	out := flag.String("out", "", "CSV output path (default stdout)")
	seed := flag.Int64("seed", 20260509, "deterministic seed")
	reps := flag.Int("reps", 30, "repetitions per fraction (distinct sub-seeds)")
	services := flag.Int("services", 200, "mesh size")
	fracsFlag := flag.String("fractions", "0,0.05,0.10,0.25,0.50,0.75,0.90", "stale fractions to sweep")
	scenario := flag.String("scenario", "synthetic", "synthetic | real-topology")
	topodir := flag.String("topodir", "", "directory of real *.edges topology files (real-topology scenario)")
	flag.Parse()

	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	fractions := parseFractions(*fracsFlag)

	// Real-topology mode runs the sweep over the Alibaba dependency graph with
	// the requested number of services instead of a synthetic mesh.
	var realTopo [][2]int
	if *scenario == "real-topology" {
		topos := loadTopologies(*topodir)
		t, ok := topos[*services]
		if !ok {
			fmt.Fprintf(os.Stderr, "no real topology with %d services in %s\n", *services, *topodir)
			os.Exit(1)
		}
		realTopo = t
	}

	methods := []struct {
		key string
		fn  func([]genEdge, time.Time) outcome
	}{
		{"QTKG", evalQTPO},
		{"NoKG", evalNoKG},
	}

	rows := [][]string{{
		"stale_fraction", "method", "services",
		"compliance_pct_median", "blocked_safe_pct_median",
		"unsafe_promotion_pct", "unsafe_promotions", "managed_edges", "reps",
	}}

	for _, frac := range fractions {
		type agg struct {
			comp    []float64
			blocked []float64
			unsafe  int
			managed int
		}
		results := map[string]*agg{}
		for _, m := range methods {
			results[m.key] = &agg{}
		}
		for r := 0; r < *reps; r++ {
			subSeed := *seed + int64(r)*7919 + int64(*services)
			var edges []genEdge
			if *scenario == "real-topology" {
				edges = meshFromTopology(realTopo, subSeed)
			} else {
				edges = generateMesh(*services, subSeed)
			}
			injectStaleness(edges, frac, subSeed, now)
			for _, m := range methods {
				// Both methods see the same corrupted mesh.
				o := m.fn(edges, now)
				a := results[m.key]
				a.comp = append(a.comp, o.pct(o.compliant))
				a.blocked = append(a.blocked, o.pct(o.blockedSafe))
				a.unsafe += o.unsafePromotion
				a.managed += o.managed
			}
		}
		for _, m := range methods {
			a := results[m.key]
			sort.Float64s(a.comp)
			sort.Float64s(a.blocked)
			medComp := a.comp[len(a.comp)/2]
			medBlocked := a.blocked[len(a.blocked)/2]
			unsafePct := 100 * float64(a.unsafe) / float64(a.managed)
			rows = append(rows, []string{
				strconv.FormatFloat(frac, 'f', 2, 64),
				m.key,
				strconv.Itoa(*services),
				strconv.FormatFloat(medComp, 'f', 2, 64),
				strconv.FormatFloat(medBlocked, 'f', 2, 64),
				strconv.FormatFloat(unsafePct, 'f', 2, 64),
				strconv.Itoa(a.unsafe),
				strconv.Itoa(a.managed),
				strconv.Itoa(*reps),
			})
		}
	}

	var w *csv.Writer
	if *out == "" {
		w = csv.NewWriter(os.Stdout)
	} else {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		w = csv.NewWriter(f)
	}
	if err := w.WriteAll(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	w.Flush()
}
