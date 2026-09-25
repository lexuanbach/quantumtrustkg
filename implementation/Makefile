# Directory-level build entry for the QuantumTrustKG artifact (ICSOC 2026, P4).
# The targets below reproduce the evaluation of Sect. 4 (RQ1 to RQ3) and the
# artifact checks. Every command takes an explicit seed (20260509, the seed
# stated in the evaluation) and writes into experiments/outputs/processed/.
#
#   controller-tests    unit tests for the gates behind G1 to G4 and the mesh/status path
#   baseline-compare    Table 1 (H2 harness, 200-service meshes, 30 repetitions),
#                       the Online Boutique ablation and the real-topology run
#   weight-sweep        the +-50% weight and hysteresis sweep of Sect. 5
#   staleness-sweep     Fig. 2(b), fail-closed behaviour under mass staleness
#   scale-stress        parser and filter cost at 500/1000/2000 services
#   decision-trace      one explained edge decision (Checkout to Payment)
#   pq-handshake, wire-tls  measured handshake cost and wire trials for RQ3
#   paper-results       replay of the recorded H1 and modelled CSVs
#   coq                 the Coq/Rocq proofs of the guarantees (formal/coq)
#
# Some targets need optional tools (kind, OpenSSL with ML-KEM, coqc). Those
# steps record an unavailable or skipped row instead of failing.
PYTHON ?= python3
GO ?= go

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)
SCRIPTS := $(ROOT)/implementation/experiments/scripts
PROCESSED := $(ROOT)/implementation/experiments/outputs/processed
FIXTURES := $(ROOT)/implementation/semantics/fixtures
GENERATORS := $(ROOT)/implementation/tools/generators

.PHONY: help evidence paper-results scale-stress baseline-compare weight-sweep staleness-sweep datamodel-benchmark pq-handshake decision-trace pq-tls wire-tls controller-harness controller-tests coq clean

help:
	@echo "Targets (each one overwrites its committed output files):"
	@echo "  controller-tests    Go unit tests of the controller"
	@echo "  baseline-compare    H2 comparison: Table 1, Alibaba rows of Fig. 2(a), Online Boutique ablation"
	@echo "  weight-sweep        H2 weight and hysteresis sweep (Sect. 5)"
	@echo "  staleness-sweep     Mass staleness, synthetic and Alibaba 200-service meshes (Fig. 2(b))"
	@echo "  scale-stress        H3 parser/filter stress at 500/1000/2000 services (15 iterations)"
	@echo "  decision-trace      Online Boutique Checkout->Payment trace JSON"
	@echo "  paper-results       Replay of the recorded H1 and modelled values (not a re-measurement)"
	@echo "  datamodel-benchmark SQLite vs networkx vs rdflib admissibility join (needs rdflib, networkx)"
	@echo "  pq-handshake        OpenSSL TLS 1.3 handshake medians, 300 per group (needs OpenSSL 3.5+)"
	@echo "  wire-tls            30 client/server trials per group with negotiated group read back"
	@echo "  pq-tls              OpenSSL availability probe (not used by the paper)"
	@echo "  controller-harness  Status rows of the kind scale/churn harness (writes rows, runs nothing)"
	@echo "  coq                 Coq/Rocq proofs of G1-G4 (needs Rocq 9)"
	@echo "  evidence            Status report of all checks (run-evidence-summary.py)"
	@echo "  clean               Deletes the committed result files"

evidence:
	$(PYTHON) $(SCRIPTS)/run-evidence-summary.py

paper-results:
	$(PYTHON) $(SCRIPTS)/run-paper-results.py

controller-tests:
	$(MAKE) -C $(ROOT)/implementation/controller test

scale-stress:
	$(PYTHON) $(GENERATORS)/generate_scale_fixture.py --services 500 --seed 20260509 --edge-factor 2.0 --out $(FIXTURES)/scale-500.ttl
	$(PYTHON) $(GENERATORS)/generate_scale_fixture.py --services 1000 --seed 20260509 --edge-factor 2.0 --out $(FIXTURES)/scale-1000.ttl
	$(PYTHON) $(GENERATORS)/generate_scale_fixture.py --services 2000 --seed 20260509 --edge-factor 2.0 --out $(FIXTURES)/scale-2000.ttl
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/scale-stress \
	  --fixtures ../semantics/fixtures/scale-500.ttl,../semantics/fixtures/scale-1000.ttl,../semantics/fixtures/scale-2000.ttl \
	  --iterations 15 \
	  --out ../experiments/outputs/processed/scale-stress-results.csv

baseline-compare:
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/baseline-compare \
	  --seed 20260509 --reps 30 \
	  --out ../experiments/outputs/processed/baseline-comparison.csv
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/baseline-compare \
	  --scenario online-boutique --seed 20260509 --reps 30 \
	  --out ../experiments/outputs/processed/online-boutique-ablation.csv
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/baseline-compare \
	  --scenario real-topology --seed 20260509 --reps 30 \
	  --topodir ../experiments/fixtures/real-topologies/alibaba \
	  --out ../experiments/outputs/processed/baseline-comparison-real-alibaba.csv

weight-sweep:
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/baseline-compare \
	  --scenario weight-sweep --seed 20260509 --reps 30 \
	  --out ../experiments/outputs/processed/weight-sweep-results.csv

staleness-sweep:
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/staleness-sweep \
	  --seed 20260509 --reps 30 --services 200 \
	  --fractions 0,0.05,0.10,0.25,0.50,0.75,0.90 \
	  --out ../experiments/outputs/processed/staleness-sweep-results.csv
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/staleness-sweep \
	  --scenario real-topology --topodir ../experiments/fixtures/real-topologies/alibaba \
	  --seed 20260509 --reps 30 --services 200 \
	  --fractions 0,0.05,0.10,0.25,0.50,0.75,0.90 \
	  --out ../experiments/outputs/processed/staleness-sweep-real-alibaba.csv

datamodel-benchmark:
	$(PYTHON) $(SCRIPTS)/run-datamodel-benchmark.py \
	  --fixture $(FIXTURES)/scale-1000.ttl --repeats 5 \
	  --out $(PROCESSED)/datamodel-benchmark.csv

pq-handshake:
	$(PYTHON) $(SCRIPTS)/run-pq-tls-handshake.py --handshakes 300 \
	  --out $(PROCESSED)/pq-tls-handshake-results.csv

decision-trace:
	$(PYTHON) $(GENERATORS)/generate_online_boutique_fixture.py --out $(FIXTURES)/online-boutique.ttl
	cd $(ROOT)/implementation/controller && $(GO) run ./cmd/decision-trace \
	  --fixture online-boutique.ttl \
	  --source Checkout \
	  --destination Payment \
	  --out ../experiments/outputs/processed/online-boutique-decision-trace.json

pq-tls:
	$(PYTHON) $(SCRIPTS)/run-pq-tls-benchmark.py --out $(PROCESSED)/pq-tls-benchmark-results.csv

wire-tls:
	$(PYTHON) $(SCRIPTS)/run-wire-tls-validation.py --out $(PROCESSED)/wire-tls-validation.csv

controller-harness:
	$(PYTHON) $(SCRIPTS)/run-controller-scale-churn.py \
	  --scale-out $(PROCESSED)/controller-scale-results.csv \
	  --churn-out $(PROCESSED)/controller-churn-results.csv

coq:
	$(MAKE) -C $(ROOT)/formal/coq

clean:
	rm -f $(PROCESSED)/*.csv $(PROCESSED)/*.json $(PROCESSED)/*.md
