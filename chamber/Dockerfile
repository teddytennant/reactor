# Reactor chamber image (SPEC §5.4, §12.7).
#
# One image, baked once, spawned per detonation from a Daytona snapshot. It
# holds everything the disposable side needs: SGLang + the Qwen victim weights
# on CUDA, Node/uv to run real MCP servers under test, strace/inotify collectors,
# and the prebuilt Reactor chamber binaries (victim, wire, sink, collect).
#
# Build the binaries first on the host (static, CGO_OFF) so this image never
# needs a Go/Rust toolchain:
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/victim ./cmd/victim
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/wire   ./cmd/wire
#   (cd crates && cargo build --release)  # -> reactor-sink, reactor-collect
# then: docker build -f chamber/Dockerfile -t reactor-chamber:qwen36 .
#
# Weights are baked in, never pulled at demo time (SPEC §11, DEMO §5): a 27B FP8
# cold pull on venue wifi is a dead demo.

# CUDA base matching the RTX Pro 6000 on Daytona. Pin the exact tag at build.
FROM nvidia/cuda:12.6.2-cudnn-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONUNBUFFERED=1 \
    HF_HUB_ENABLE_HF_TRANSFER=1 \
    REACTOR_VICTIM_BASE=http://127.0.0.1:8000/v1 \
    REACTOR_VICTIM_MODEL=Qwen/Qwen3.6-27B-FP8

# --- system + node + uv + collectors ---
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 python3-pip python3-venv git curl ca-certificates \
      strace inotify-tools coreutils \
 && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
 && apt-get install -y --no-install-recommends nodejs \
 && curl -LsSf https://astral.sh/uv/install.sh | sh \
 && rm -rf /var/lib/apt/lists/*
ENV PATH="/root/.local/bin:${PATH}"

# --- SGLang (pin a version that ships the Qwen3 tool/reasoning parsers) ---
# Pin the exact SGLang version at build; the launch line lives in entrypoint.sh.
RUN pip3 install --no-cache-dir "sglang[all]==0.4.6" || \
    echo "NOTE: pin sglang to a version supporting --tool-call-parser qwen3_coder"

# --- bake the victim weights (FP8 safetensors on CUDA; never GGUF, SPEC §5.4) ---
# Uncomment at real build time; kept optional so the image builds without the
# ~30GB pull during development. models.lock records the pinned revision.
# RUN python3 -c "from huggingface_hub import snapshot_download; \
#     snapshot_download('Qwen/Qwen3.6-27B-FP8', local_dir='/weights/qwen3.6-27b-fp8')"

# --- Reactor chamber binaries (prebuilt on host) ---
WORKDIR /reactor
COPY dist/victim dist/wire /reactor/bin/
COPY crates/target/release/reactor-sink crates/target/release/reactor-collect /reactor/bin/
COPY chamber/entrypoint.sh /reactor/entrypoint.sh
COPY models.lock /reactor/models.lock
RUN chmod +x /reactor/bin/* /reactor/entrypoint.sh

ENV PATH="/reactor/bin:${PATH}"

# The victim holds no real credentials and the sandbox owns no outbound route
# except the sink. Nothing here should ever contain a real secret (SPEC §10).
EXPOSE 8000
ENTRYPOINT ["/reactor/entrypoint.sh"]
