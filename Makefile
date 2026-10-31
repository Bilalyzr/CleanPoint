# CleanPoint — local ransomware-recovery analysis simulator (defensive prototype)

GO      ?= go
PYTHON  ?= python
NPM     ?= npm
API     ?= http://127.0.0.1:8099
ADDR    ?= 127.0.0.1:8099

.PHONY: build build-web build-go test evaluate demo serve clean bootstrap benchmarks docker

build-web: ## build the M2 React/TypeScript console into web/console-dist
	cd web/console && (test -d node_modules || $(NPM) ci) && $(NPM) run build

build-go: ## build all Go binaries into ./bin (embeds web/console-dist)
	rm -f bin/*.exe~
	$(GO) build -o bin/cleanpoint.exe ./cmd/cli
	$(GO) build -o bin/cleanpoint-server.exe ./cmd/server
	$(GO) build -o bin/cleanpoint-simulator.exe ./cmd/simulator

build: ## build web console (if npm present) and all Go binaries into ./bin
	@if command -v $(NPM) >/dev/null 2>&1; then \
		$(MAKE) build-web; \
	else \
		echo "(npm not found; embedding prebuilt web/console-dist)"; \
	fi
	$(MAKE) build-go

bootstrap: build ## toolchain check + build
	$(GO) vet ./...
	@echo "bootstrap ok"

test: build ## run the full >=92-case automated suite
	scripts/test.sh

serve: build ## start the local API server
	CLEANPOINT_ADDR=$(ADDR) ./bin/cleanpoint-server.exe

demo: build ## one-command guided demonstration
	CLEANPOINT_API=$(API) scripts/demo.sh

simulate: build ## run one scenario against a live server (SCENARIO=, SEED=)
	CLEANPOINT_API=$(API) ./bin/cleanpoint.exe simulate --scenario $(SCENARIO) --seed $(SEED)

evaluate: build ## locked-corpus evaluation (release evidence)
	CLEANPOINT_API=$(API) ./bin/cleanpoint.exe evaluate --scenario locked --seeds 9,10 \
		--json --output results/eval-locked.json

benchmarks: build ## snapshot/restore/latency benchmarks with real numbers
	CLEANPOINT_API=$(API) scripts/benchmark.sh

docker: ## build the optional container image (same single process)
	docker build -t cleanpoint:local .

clean: ## remove build artifacts and local sandbox
	rm -rf bin sandbox
