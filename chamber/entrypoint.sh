#!/usr/bin/env bash
# Reactor chamber entrypoint (SPEC §5.4). Boots SGLang serving the pinned Qwen
# victim on 127.0.0.1:8000, then idles — the engine drives everything else
# (bait, sink, wire, victim, artifact) over the Daytona exec/fs API.
#
# Cold-start must be verified before the pitch (SPEC §11): a GPU sandbox +
# SGLang load that isn't warm is a dead demo. `reactor serve` pre-warms by
# provisioning from the snapshot and waiting for /v1/models to answer.
set -euo pipefail

WEIGHTS="${REACTOR_WEIGHTS:-/weights/qwen3.6-27b-fp8}"
MODEL_PATH="${REACTOR_MODEL_PATH:-Qwen/Qwen3.6-27B-FP8}"
if [ -d "$WEIGHTS" ]; then
  MODEL_PATH="$WEIGHTS"
fi

echo "reactor-chamber: models.lock ="
cat /reactor/models.lock 2>/dev/null | sed 's/^/  /' || true

# Launch skeleton straight from the Qwen model card (SPEC §5.4). Do not invent
# parser names; these are what Qwen documents for 3.6 on SGLang.
if command -v python3 >/dev/null && python3 -c "import sglang" 2>/dev/null; then
  echo "reactor-chamber: starting SGLang on 127.0.0.1:8000 …"
  python3 -m sglang.launch_server \
    --model-path "$MODEL_PATH" \
    --host 127.0.0.1 --port 8000 \
    --tp-size 1 \
    --mem-fraction-static 0.85 \
    --context-length 32768 \
    --reasoning-parser qwen3 \
    --tool-call-parser qwen3_coder \
    --random-seed 7 &
  SGLANG_PID=$!

  # Wait for the model endpoint (cold-start verification).
  for i in $(seq 1 120); do
    if curl -sf http://127.0.0.1:8000/v1/models >/dev/null 2>&1; then
      echo "reactor-chamber: victim model up (cold-start ${i}s)"
      break
    fi
    sleep 1
  done
else
  echo "reactor-chamber: SGLang not installed — CPU/dev image; victim runs hosted or sim"
fi

# Idle so the sandbox stays alive for the engine to drive.
echo "reactor-chamber: ready; awaiting engine control-plane commands"
exec sleep infinity
