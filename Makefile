# Reactor — build the whole harness.
#
#   make build     Go binaries + Rust binaries
#   make go        Go binaries only (engine + chamber components + TUI)
#   make rust      Rust binaries only (sink + collector)
#   make test      unit tests: Go (race) + Rust + web + eval gate
#   make lint      gofmt + go vet + clippy + tsc
#   make zoo       verify the authored fixtures still attack as authored
#   make smoke     upload an archive and detonate it end to end
#   make ci        everything CI runs, including the full-zoo scorecard
#   make run       build, then serve the engine on :8787
#   make demo      build, then detonate the star artifact with the sim victim
#   make ui        run the Next.js console (proxies to the engine)
#   make clean

BIN := bin
GOBINS := reactor victim wire sink simllm reactor-tui mutate
RUSTBINS := crates/target/release/reactor-sink crates/target/release/reactor-collect

# CI and every local test path stay offline: the deterministic analyst and the
# sim victim are reproducible and cost nothing.
OFFLINE := REACTOR_ANALYST=deterministic REACTOR_VICTIM_BACKEND=sim

.PHONY: build go rust test test-go test-rust test-web test-eval lint zoo smoke ci run demo ui eval clean lock

build: go rust

go:
	@mkdir -p $(BIN)
	@for b in $(GOBINS); do \
		case $$b in \
			reactor) src=./cmd/reactor ;; \
			victim)  src=./cmd/victim ;; \
			wire)    src=./cmd/wire ;; \
			sink)    src=./cmd/sink ;; \
			simllm)  src=./cmd/simllm ;; \
			reactor-tui) src=./cmd/reactor-tui ;; \
			mutate)  src=./cmd/mutate ;; \
		esac ; \
		echo "  go build $$b" ; \
		go build -o $(BIN)/$$b $$src || exit 1 ; \
	done
	@echo "go binaries in $(BIN)/"

rust:
	@echo "  cargo build --release (sink + collector)"
	@cd crates && cargo build --release
	@echo "rust binaries in crates/target/release/"

test: test-go test-rust test-web test-eval

# -race: the engine fans chamber events onto a bus that the SSE handler, the CLI
# renderer and the report snapshot all read concurrently.
test-go:
	go test -race -count=1 ./...

test-rust:
	cd crates && cargo test --all

test-web:
	cd web && npm test

# The scorecard gate decides whether a detonation run counts as green, so its
# own logic is tested before it is trusted.
test-eval:
	python3 -m unittest discover -s eval -p '*_test.py'

lint:
	@unformatted="$$(gofmt -l ./cmd ./internal)" ; \
	if [ -n "$$unformatted" ]; then echo "not gofmt'd:" ; echo "$$unformatted" ; exit 1 ; fi
	go vet ./...
	cd crates && cargo clippy --all-targets --all-features -- -D warnings
	cd web && npm run typecheck

# Proves the authored attacks still attack: notes-mcp poisons exactly its 4th
# serve, benign controls stay byte-stable, trigger-mcp wakes only on its magic
# input. Never runs the dropper/stealer payloads — those are chamber-only.
zoo:
	bash zoo/verify.sh

ci: lint test zoo smoke eval

run: build
	./$(BIN)/reactor serve

demo: build
	./$(BIN)/reactor detonate art_notes_mcp --victim sim --sessions 5

# The full offline zoo, scored against eval/expected.json. Fails on any false
# quarantine or any miss that is not already documented there.
eval: build
	@$(call with_engine,python3 eval/run.py --check)

# Upload an archive and detonate it end to end — the path a person takes with
# their own artifact rather than one out of the zoo.
smoke: build
	@$(call with_engine,python3 eval/smoke_ingest.py)

# with_engine runs $(1) against a throwaway engine on :8787 and always reaps it.
define with_engine
$(OFFLINE) ./$(BIN)/reactor serve > .engine.log 2>&1 & echo $$! > .engine.pid ; \
for i in $$(seq 1 30); do \
	curl -fsS http://127.0.0.1:8787/api/health >/dev/null 2>&1 && break ; \
	sleep 1 ; \
done ; \
$(1) ; rc=$$? ; \
kill `cat .engine.pid` 2>/dev/null ; rm -f .engine.pid ; \
exit $$rc
endef

ui:
	cd web && npm install && npm run dev

lock:
	@echo "pin model revisions into models.lock after a successful weights pull"

clean:
	rm -rf $(BIN) crates/target web/.next web/node_modules
