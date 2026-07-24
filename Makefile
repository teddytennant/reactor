# Reactor — build the whole harness.
#
#   make build     Go binaries + Rust binaries
#   make go        Go binaries only (engine + chamber components + TUI)
#   make rust      Rust binaries only (sink + collector)
#   make test      Go + Rust tests
#   make run       build, then serve the engine on :8787
#   make demo      build, then detonate the star artifact with the sim victim
#   make ui        run the Next.js console (proxies to the engine)
#   make clean

BIN := bin
GOBINS := reactor victim wire sink simllm reactor-tui mutate
RUSTBINS := crates/target/release/reactor-sink crates/target/release/reactor-collect

.PHONY: build go rust test run demo ui eval clean lock

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

test:
	go test ./...
	@cd crates && cargo test --release 2>/dev/null || true

run: build
	./$(BIN)/reactor serve

demo: build
	./$(BIN)/reactor detonate art_notes_mcp --victim sim --sessions 5

eval: build
	REACTOR_ANALYST=deterministic ./$(BIN)/reactor serve & echo $$! > .engine.pid ; \
	sleep 1 ; \
	python3 eval/run.py ; \
	kill `cat .engine.pid` 2>/dev/null ; rm -f .engine.pid

ui:
	cd web && npm install && npm run dev

lock:
	@echo "pin model revisions into models.lock after a successful weights pull"

clean:
	rm -rf $(BIN) crates/target web/.next web/node_modules
