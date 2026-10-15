# Memory-capped Go entrypoints for constrained machines.
#
# This repo's primary task runner is welder (`welder run build`, `welder run test`
# — see welder.yaml); CI uses it on full-size runners and nothing here replaces it.
# These targets exist for the other case: building or testing this module inside a
# container, VM or CI sandbox with limited RAM and (usually) no swap.
#
# Why it needs its own entrypoint: this module links the Pulumi provider SDKs,
# which are among the most memory-hungry Go packages in existence to compile. A
# default `go build ./...` runs GOMAXPROCS package compiles at once, and each
# `go tool compile` additionally parallelises its own backend (`-c`), which
# multiplies peak RSS because several functions' SSA state is live at the same
# time. Single compiles in this module have been observed above 6 GB, and two
# concurrent ones will exhaust a 12 GB machine and be killed by the OOM killer.
#
# The kill is silent and easy to misread: you get `signal: killed`, or an exit
# status of 137/143 with no compiler diagnostic at all. It looks like a hang, a
# toolchain bug or a flake. It is memory. See CLAUDE.md.
#
# Measured on this module (Go 1.x, linux/amd64, caps below applied):
#
#   go vet ./...          2.3 GB peak    ~3 min    <- `check`, the only one that
#                                                     comfortably fits 12 GB
#   link ./cmd/sc        12.9 GB peak    ~2 min    <- `build`, compiles cached
#   go build ./...       14.2 GB peak   ~76 min    <- `build-all`, from cold
#
# Note what that says: the caps bound the COMPILE phase, but the LINK of
# ./cmd/sc needs ~13 GB on its own and neither GOMAXPROCS nor GOMEMLIMIT
# reduces it (the linker is largely serial already, and its footprint is not
# Go heap). So on a machine under ~16 GB, do NOT link — use `check` as your
# gate and let CI produce the binaries.
#
# Every cap below is a soft default — override it on a machine with the RAM:
#
#   make test GO_MAXPROCS=8 GO_TEST_PARALLEL=4
#   make build-all GO_MEMLIMIT=24GiB
#
# GO_MEMLIMIT sets GOMEMLIMIT, which makes the Go runtime GC harder instead of
# growing; it is a complement, not a substitute, since it does not bound the
# compiler's non-heap arenas. GO_MAXPROCS sets GOMAXPROCS and is the load-bearing
# one: it caps BOTH concurrent package compiles and each compile's internal
# backend parallelism.
#
# The variables are deliberately named GO_* rather than GO* so that a GOMAXPROCS
# already present in the environment cannot silently defeat the cap: with
# `GOMAXPROCS ?= 1`, make imports the environment value and `?=` keeps it, so a
# shell exporting GOMAXPROCS=8 would get an 8-way build from a Makefile whose
# whole purpose is to serialise. Setting the real variables explicitly on each
# command line below makes the inherited value irrelevant.

GO_MAXPROCS      ?= 1
GO_MEMLIMIT      ?= 6GiB
GO_TEST_PARALLEL ?= 1
GOLANGCI         ?= $(CURDIR)/bin/golangci-lint
GOFUMPT          ?= $(CURDIR)/bin/gofumpt
DIST             ?= $(CURDIR)/dist

# Applied to every go invocation below.
CAP = GOMAXPROCS=$(GO_MAXPROCS) GOMEMLIMIT=$(GO_MEMLIMIT)

.PHONY: help build build-all check test test-race lint fmt tidy clean

help:
	@echo "Memory-capped Go entrypoints (see the comments at the top of this file)."
	@echo
	@echo "  make check       go vet ./... — START HERE. ~2.3 GB, type-checks tests too"
	@echo "  make build       link the sc binary into dist/ — needs ~13 GB, see above"
	@echo "  make test        go test ./... serialised, with GOMEMLIMIT"
	@echo "  make build-all   go build ./... serialised — ~14 GB peak, ~76 min cold"
	@echo "  make test-race   go test -race ./... UNCAPPED — needs a large machine"
	@echo "  make lint        golangci-lint (bin/, provisioned by 'welder run tools')"
	@echo "  make fmt tidy    gofumpt -w ./ ; go mod tidy"
	@echo
	@echo "Caps: GOMAXPROCS=$(GO_MAXPROCS) GOMEMLIMIT=$(GO_MEMLIMIT) -p $(GO_TEST_PARALLEL)"
	@echo "Override on a big machine, e.g.: make test GO_MAXPROCS=8 GO_TEST_PARALLEL=4"

# Narrower than build-all, but NOT cheap: the link alone peaks near 13 GB and
# the caps do not reduce it. Prefer `check` unless you actually need a binary.
# CI builds per-command rather than ./... (see .github/workflows/push.yaml).
build:
	$(CAP) go build -trimpath -buildvcs=false -o $(DIST)/sc ./cmd/sc

# Preferred full-repo gate. `go vet` compiles test packages as well, so it
# catches the broken-fake/stub errors `go build ./...` silently misses, and it
# is cheaper than building every package's object code.
check:
	$(CAP) go vet -p $(GO_TEST_PARALLEL) ./...

test:
	$(CAP) go test -p $(GO_TEST_PARALLEL) ./...

# The full compile AND link of every main package: the most expensive target
# here (~14 GB, ~76 min cold even serialised). Rarely what you want locally.
build-all:
	$(CAP) go build -p $(GO_TEST_PARALLEL) ./...

# Opt-in. -race is the single largest memory multiplier (shadow memory is
# roughly 5-10x), so it is deliberately left uncapped and out of `test`.
test-race:
	go test -race ./...

lint:
	$(GOLANGCI) run --fast --timeout 5m

fmt:
	$(GOFUMPT) -l -w ./

tidy:
	go mod tidy

clean:
	rm -rf $(DIST)/sc
