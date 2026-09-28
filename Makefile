# qtldr — one-word targets; anything longer than a few lines lives in scripts/.
.DEFAULT_GOAL := help
.PHONY: help doctor setup build test lint fmt check golden capture demo ci clean

help: ## show this help
	@awk 'BEGIN {FS = ":.*?## "; printf "\n\033[1mUsage:\033[0m make \033[36m<target>\033[0m\n\n\033[1mTargets:\033[0m\n"} \
	     /^[a-z]+:.*?## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2} \
	     END {print ""}' $(MAKEFILE_LIST)

doctor: ## check required tools (go, git, crap4go, gocognit, gremlins, editor)
	@bash scripts/doctor.sh

setup: ## download modules and install crap4go + gocognit
	@bash scripts/setup.sh

build: ## build bin/qtldr
	@go build -o bin/qtldr ./cmd/qtldr

test: ## run all Go tests
	@go test ./...

lint: ## gofmt check, go vet, staticcheck, cognitive <= 15
	@bash scripts/lint.sh

fmt: ## format all Go code
	@gofmt -w cmd internal

check: ## qtldr check on qtldr itself (changed functions; runs their tests)
	@go run ./cmd/qtldr check --changed

golden: ## regenerate testdata/golden (review the git diff after!)
	@bash scripts/golden.sh

capture: ## re-run crap4go + gocognit on the fixture and save their output
	@bash scripts/capture.sh

demo: ## analyze the fixture with coverage; show applyTiered, worst, check
	@bash scripts/demo.sh

ci: ## everything CI runs: lint, tests, then qtldr check --all on itself
	@bash scripts/ci.sh

clean: ## remove bin/, snapshots and crap4go leftovers
	@bash scripts/clean.sh
