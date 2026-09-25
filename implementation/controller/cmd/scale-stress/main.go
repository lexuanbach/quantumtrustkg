// Command scale-stress measures the cost of the semantic core at 500, 1000 and
// 2000 services. It feeds the H3 semantic-core scalability table of the extended
// version. The camera-ready paper does not tabulate it.
//
// For each generated Turtle fixture it parses the file into communication edges
// (graph.BuildEdgesFromTTL), applies a class-only admissibility test and builds
// the status strings. Each phase is timed over --iterations runs after a forced
// garbage collection, and the median is reported. The command therefore measures
// parsing and filtering only. It excludes Fuseki, Kubernetes watches, Istio churn
// and the full QTPO gate set, which means the numbers are not the in-cluster selection
// latency of Sect. 4.1.
//
// Output: one CSV row per fixture (scale-stress-results.csv when run through
// make scale-stress). Timing depends on the host. Only the edge, triple and
// candidate counts are deterministic, because the fixtures come from
// tools/generators/generate_scale_fixture.py with a fixed seed.

package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/pkg/types"
)

// fixtureStats holds the counts and the median timings for one fixture.
// QueryBuildMS is the parse time. RuleFilterMS is the class filter time.
// ControllerMS is the sum of both plus the status materialisation, which
// approximates one controller pass over the fixture.
type fixtureStats struct {
	Services      int
	Edges         int
	Triples       int
	QueryBuildMS  float64
	RuleFilterMS  float64
	ControllerMS  float64
	AllocatedMB   float64
	Fixture       string
	Iterations    int
	Promoted      int
	Blocked       int
	CandidateSeen int
}

func main() {
	fixtureFlag := flag.String("fixtures", "", "comma-separated fixture paths")
	outFlag := flag.String("out", "", "CSV output path")
	iterations := flag.Int("iterations", 7, "measurement iterations")
	flag.Parse()

	if strings.TrimSpace(*fixtureFlag) == "" {
		fatalf("--fixtures is required")
	}
	if *iterations < 3 {
		fatalf("--iterations must be at least 3")
	}

	paths := strings.Split(*fixtureFlag, ",")
	rows := make([]fixtureStats, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		stats, err := runFixture(path, *iterations)
		if err != nil {
			fatalf("%s: %v", path, err)
		}
		rows = append(rows, stats)
	}

	var out *os.File
	var err error
	if strings.TrimSpace(*outFlag) == "" {
		out = os.Stdout
	} else {
		if err := os.MkdirAll(filepath.Dir(*outFlag), 0o755); err != nil {
			fatalf("create output directory: %v", err)
		}
		out, err = os.Create(*outFlag)
		if err != nil {
			fatalf("create output: %v", err)
		}
		defer out.Close()
	}

	if err := writeCSV(out, rows); err != nil {
		fatalf("write csv: %v", err)
	}
}

// runFixture parses and filters one fixture iterations times and returns the
// median timings. AllocatedMB is the heap in use after the last iteration, a
// coarse memory indicator.
func runFixture(path string, iterations int) (fixtureStats, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fixtureStats{}, err
	}

	services, triples := countServicesAndTriples(raw)
	edges, err := graph.BuildEdgesFromTTL(raw)
	if err != nil {
		return fixtureStats{}, err
	}

	buildSamples := make([]float64, 0, iterations)
	ruleSamples := make([]float64, 0, iterations)
	controllerSamples := make([]float64, 0, iterations)
	var promoted, blocked, candidates int

	for i := 0; i < iterations; i++ {
		runtime.GC()

		start := time.Now()
		parsedEdges, err := graph.BuildEdgesFromTTL(raw)
		if err != nil {
			return fixtureStats{}, err
		}
		buildMS := elapsedMS(start)

		start = time.Now()
		promoted, blocked, candidates = filterEdges(parsedEdges)
		ruleMS := elapsedMS(start)

		start = time.Now()
		materializeStatus(parsedEdges)
		controllerMS := buildMS + ruleMS + elapsedMS(start)

		buildSamples = append(buildSamples, buildMS)
		ruleSamples = append(ruleSamples, ruleMS)
		controllerSamples = append(controllerSamples, controllerMS)
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return fixtureStats{
		Services:      services,
		Edges:         len(edges),
		Triples:       triples,
		QueryBuildMS:  median(buildSamples),
		RuleFilterMS:  median(ruleSamples),
		ControllerMS:  median(controllerSamples),
		AllocatedMB:   float64(mem.Alloc) / 1024.0 / 1024.0,
		Fixture:       path,
		Iterations:    iterations,
		Promoted:      promoted,
		Blocked:       blocked,
		CandidateSeen: candidates,
	}, nil
}

// countServicesAndTriples counts lines that declare a Service and approximates
// the number of triples: one per statement line, with the comma-separated
// supportsProfile objects counted individually. It reads the Turtle text
// without a parser, which is enough for the generated fixtures.
func countServicesAndTriples(raw []byte) (int, int) {
	services := 0
	triples := 0
	for _, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "@prefix") {
			continue
		}
		if strings.Contains(line, " a :Service") {
			services++
		}
		if strings.Contains(line, ":supportsProfile") {
			triples += bytes.Count([]byte(line), []byte(",")) + 1
			continue
		}
		triples++
	}
	return services, triples
}

// filterEdges applies a class-only test. An edge counts as promoted when some
// candidate profile reaches the required class, and as blocked otherwise. It
// does not run the freshness, threat or rollout gates of QTPO. It returns the
// promoted count, the blocked count and the number of candidate profiles seen.
func filterEdges(edges []types.CommunicationEdge) (int, int, int) {
	promoted := 0
	blocked := 0
	candidates := 0
	for _, edge := range edges {
		candidates += len(edge.CandidateProfiles)
		admissible := false
		for _, profile := range edge.CandidateProfiles {
			if rank(profile.SecurityCategory) >= rank(edge.RequiredSecurityLevel) {
				admissible = true
				break
			}
		}
		if admissible {
			promoted++
		} else {
			blocked++
		}
	}
	return promoted, blocked, candidates
}

// materializeStatus builds one status string per edge, standing in for the cost
// of writing AssignmentStatus objects. The first candidate is used and no
// ranking is done.
func materializeStatus(edges []types.CommunicationEdge) []string {
	status := make([]string, 0, len(edges))
	for _, edge := range edges {
		profile := "blocked"
		if len(edge.CandidateProfiles) > 0 {
			profile = edge.CandidateProfiles[0].Name
		}
		status = append(status, edge.ID+":"+profile+":"+string(edge.RequiredSecurityLevel))
	}
	return status
}

// rank orders the classes as classical 1, hybrid 2, quantum-safe 3. An unknown
// class ranks 0.
func rank(category types.SecurityCategory) int {
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

// median returns the median of samples without modifying the slice.
func median(samples []float64) float64 {
	ordered := append([]float64(nil), samples...)
	sort.Float64s(ordered)
	mid := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[mid]
	}
	return (ordered[mid-1] + ordered[mid]) / 2
}

// elapsedMS returns the time since start in milliseconds.
func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

// writeCSV writes the header and one row per fixture. The scale column repeats
// the service count.
func writeCSV(out *os.File, rows []fixtureStats) error {
	writer := csv.NewWriter(out)
	defer writer.Flush()

	header := []string{
		"scale",
		"services",
		"communication_edges",
		"triples",
		"query_build_ms",
		"rule_filter_ms",
		"controller_est_ms",
		"alloc_mb",
		"promoted_edges",
		"blocked_edges",
		"candidate_profiles_seen",
		"iterations",
		"fixture",
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		record := []string{
			strconv.Itoa(row.Services),
			strconv.Itoa(row.Services),
			strconv.Itoa(row.Edges),
			strconv.Itoa(row.Triples),
			fmt.Sprintf("%.3f", row.QueryBuildMS),
			fmt.Sprintf("%.3f", row.RuleFilterMS),
			fmt.Sprintf("%.3f", row.ControllerMS),
			fmt.Sprintf("%.3f", row.AllocatedMB),
			strconv.Itoa(row.Promoted),
			strconv.Itoa(row.Blocked),
			strconv.Itoa(row.CandidateSeen),
			strconv.Itoa(row.Iterations),
			row.Fixture,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	return writer.Error()
}

// fatalf prints the message to standard error and exits with status 1.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
