// Command decision-trace prints the explained QTPO decision for one edge of a
// fixture. By default it takes the Online Boutique edge from Checkout to Payment,
// the regulated (PCI) edge that requires a quantum-safe profile.
//
// It parses the Turtle fixture with graph.BuildEdgesFromTTL and calls
// orchestration.TraceSelection on the chosen edge with no deprecations. The trace
// lists each candidate profile with its class, its cost score and the first stage
// that rejected it. This is the machine-readable form of the "failed gate logged"
// behaviour of the filter step of Alg. 1 (Sect. 3).
//
// Output: JSON on standard output or in --out. The Makefile target
// decision-trace writes online-boutique-decision-trace.json, which is the
// worked-example evidence of the Online Boutique appendix of the extended version.
// The command is deterministic because the fixture is static.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/internal/orchestration"
)

// main resolves the fixture relative to ../semantics/fixtures, which means it must be run
// from implementation/controller.
func main() {
	fixture := flag.String("fixture", "online-boutique.ttl", "fixture file under ../semantics/fixtures")
	source := flag.String("source", "Checkout", "source service")
	destination := flag.String("destination", "Payment", "destination service")
	out := flag.String("out", "", "optional JSON output path")
	flag.Parse()

	raw, err := os.ReadFile(filepath.Join("..", "semantics", "fixtures", *fixture))
	if err != nil {
		fatalf("read fixture: %v", err)
	}
	edges, err := graph.BuildEdgesFromTTL(raw)
	if err != nil {
		fatalf("parse fixture: %v", err)
	}
	for _, edge := range edges {
		if edge.SourceService == *source && edge.DestinationService == *destination {
			trace := orchestration.TraceSelection(edge, map[string]bool{})
			payload, err := json.MarshalIndent(trace, "", "  ")
			if err != nil {
				fatalf("encode trace: %v", err)
			}
			if *out == "" {
				fmt.Println(string(payload))
				return
			}
			if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
				fatalf("create output directory: %v", err)
			}
			if err := os.WriteFile(*out, append(payload, '\n'), 0o644); err != nil {
				fatalf("write trace: %v", err)
			}
			return
		}
	}
	fatalf("edge %s -> %s not found in fixture %s", *source, *destination, *fixture)
}

// fatalf prints the message to standard error and exits with status 1.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
