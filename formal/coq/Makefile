# The sources import From Stdlib and need Rocq 9. Rocq 9 installs "rocq compile",
# and some installs also provide a coqc 9.x wrapper. Use "rocq compile" when it is
# on PATH, otherwise coqc. Override with make COQC=... if needed.
COQC ?= $(if $(shell command -v rocq 2>/dev/null),rocq compile,coqc)
COQFLAGS := -Q . QuantumTrustKG

VFILES := Model.v QTPO.v Guarantees.v Examples.v
VOFILES := $(VFILES:.v=.vo)

.PHONY: all clean

all: $(VOFILES)

Model.vo: Model.v
	$(COQC) $(COQFLAGS) Model.v

QTPO.vo: QTPO.v Model.vo
	$(COQC) $(COQFLAGS) QTPO.v

Guarantees.vo: Guarantees.v Model.vo QTPO.vo
	$(COQC) $(COQFLAGS) Guarantees.v

Examples.vo: Examples.v Model.vo QTPO.vo Guarantees.vo
	$(COQC) $(COQFLAGS) Examples.v

clean:
	rm -f *.vo *.vos *.vok *.glob .*.aux
