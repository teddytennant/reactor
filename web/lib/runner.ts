// A DetonationRunner feeds ReactorEvents to the console through one rendering
// path, whether they come live off the engine's SSE bus or from a bundled
// fixture played back with realistic cadence. The UI never needs to know which.

import { isReactorEvent, type ReactorEvent } from "./events";
import { engineURL } from "./engine";

export interface RunnerCallbacks {
  onEvent: (ev: ReactorEvent) => void;
  onDone?: () => void;
  onError?: (message: string) => void;
}

export interface DetonationRunner {
  stop(): void;
}

const SSE_KINDS = [
  "lifecycle",
  "scan",
  "wire",
  "transcript",
  "behavioral",
  "signal",
  "analyst",
  "verdict",
  "message",
];

/**
 * LiveRunner subscribes to the engine's SSE stream. Each `data:` line is one
 * Event; the SSE event name is Event.kind (CONTRACT.md), so we listen on every
 * known kind plus the unnamed `message` fallback. The `verdict` event ends the
 * run; heartbeats (`ping`) are ignored.
 *
 * URL goes through engineURL so a Vercel-hosted UI streams straight from the
 * engine origin (NEXT_PUBLIC_ENGINE_URL) instead of through the edge rewrite.
 */
export function startLive(detonationId: string, cb: RunnerCallbacks): DetonationRunner {
  const url = engineURL(`/api/events?detonation=${encodeURIComponent(detonationId)}`);
  const es = new EventSource(url);
  let closed = false;

  const close = () => {
    if (closed) return;
    closed = true;
    es.close();
  };

  const handle = (e: MessageEvent) => {
    if (!e.data) return;
    try {
      const parsed = JSON.parse(e.data);
      if (!isReactorEvent(parsed)) return;
      cb.onEvent(parsed);
      if (parsed.kind === "verdict") {
        // Let the banner settle, then close the stream.
        setTimeout(() => {
          close();
          cb.onDone?.();
        }, 400);
      }
    } catch {
      /* ignore malformed frame */
    }
  };

  for (const kind of SSE_KINDS) es.addEventListener(kind, handle as EventListener);

  es.onerror = () => {
    // EventSource auto-reconnects; only surface an error if we never got going.
    if (es.readyState === EventSource.CLOSED && !closed) {
      close();
      cb.onError?.("event stream closed");
    }
  };

  return { stop: close };
}

// ---- Replay cadence -------------------------------------------------------

// Delay (ms) to wait BEFORE emitting an event, tuned to the DEMO.md beat sheet:
// sessions tick ~700ms apart; the session-4 signals land with a beat between
// them; the verdict banner arrives after a decisive pause.
function delayFor(ev: ReactorEvent): number {
  switch (ev.kind) {
    case "scan":
      return ev.scan?.done ? 420 : 230;
    case "lifecycle": {
      const phase = ev.lifecycle?.phase;
      switch (phase) {
        case "queued":
          return 250;
        case "provisioning":
          return 360;
        case "chamber_ready":
          return 460;
        case "bait_planted":
        case "sink_up":
          return 300;
        case "installing":
          return 380;
        case "session_start":
          return 720; // the tick — passage of time is the argument
        case "session_end":
          return 300;
        case "analyzing":
          return 560;
        case "destroying":
          return 360;
        case "destroyed":
          return 260;
        default:
          return 300;
      }
    }
    case "wire":
      return ev.wire?.method === "tools/list" ? 150 : 190;
    case "transcript":
      return 230;
    case "behavioral":
      return 180;
    case "signal": {
      // A beat before each red row lands; extra weight on the first.
      const t = ev.signal?.type;
      if (t === "rug_pull") return 900;
      if (t === "context_exfil") return 1000;
      if (t === "benign_profile") return 500;
      return 700;
    }
    case "analyst":
      return 480;
    case "verdict":
      return 950; // hold, then the banner
    default:
      return 250;
  }
}

/**
 * ReplayRunner walks a fixture array, emitting each event after its cadence
 * delay. Fixtures stay schema-pure (a plain Event[]); pacing lives here.
 */
export function startReplay(
  events: ReactorEvent[],
  cb: RunnerCallbacks,
  opts: { speed?: number } = {},
): DetonationRunner {
  const speed = opts.speed && opts.speed > 0 ? opts.speed : 1;
  let i = 0;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;

  const step = () => {
    if (stopped) return;
    if (i >= events.length) {
      cb.onDone?.();
      return;
    }
    const ev = events[i++];
    timer = setTimeout(() => {
      if (stopped) return;
      cb.onEvent(ev);
      step();
    }, Math.max(40, delayFor(ev) / speed));
  };

  // Kick off on a microtask so callers can subscribe first.
  timer = setTimeout(step, 60);

  return {
    stop() {
      stopped = true;
      if (timer) clearTimeout(timer);
    },
  };
}
