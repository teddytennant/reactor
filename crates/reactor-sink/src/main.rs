//! reactor-sink — the contained egress sink (SPEC §4.3, §12.2).
//!
//! Every artifact process in the chamber has its egress pointed here (directly,
//! via `REACTOR_SINK_HTTP`, or transparently via `HTTP_PROXY`). Any beacon, C2
//! call, or credential POST lands in this log instead of on the internet. When
//! a planted canary appears in a request body, path, or headers, the sink emits
//! a typed `BehavioralEvent` naming which canary of what kind — that is the
//! "REACTOR-a1b2 hit the sink" gasp, turned into evidence.
//!
//! This is the spec-canonical Rust component: it parses untrusted request
//! bodies on the hot egress path, so memory safety and a single static binary
//! for the chamber image both matter. It emits exactly the `events.Event`
//! JSONL the Go engine tails, so it is a drop-in for the Go fallback sink.

use axum::{
    body::Bytes,
    extract::State,
    http::{HeaderMap, Method, StatusCode, Uri},
    response::IntoResponse,
    Router,
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeMap,
    fs::{File, OpenOptions},
    io::Write,
    net::SocketAddr,
    path::Path,
    sync::{Arc, Mutex},
    time::{SystemTime, UNIX_EPOCH},
};

#[derive(Clone, Deserialize)]
struct Canary {
    token: String,
    #[serde(default)]
    kind: String,
    #[serde(default)]
    label: String,
}

#[derive(Serialize, Default)]
struct Behavioral {
    op: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    source: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    host: String,
    #[serde(skip_serializing_if = "is_zero")]
    port: i64,
    #[serde(skip_serializing_if = "String::is_empty")]
    method: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    url_path: String,
    #[serde(skip_serializing_if = "is_zero")]
    body_bytes: i64,
    #[serde(skip_serializing_if = "String::is_empty")]
    body_sha256: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    canaries: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    canary_kinds: Vec<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    preview: String,
}

fn is_zero(v: &i64) -> bool {
    *v == 0
}

#[derive(Serialize)]
struct Event {
    kind: &'static str,
    ts_ms: i64,
    behavioral: Behavioral,
}

struct AppState {
    canaries: Vec<Canary>,
    log: Mutex<File>,
}

/// Substring-match every planted canary against `blob`. A literal search — no
/// parsing, no model — over the full request surface. Returns parallel token /
/// "kind:label" vectors.
fn match_canaries(canaries: &[Canary], blob: &str) -> (Vec<String>, Vec<String>) {
    let mut toks = Vec::new();
    let mut kinds = Vec::new();
    for c in canaries {
        if blob.contains(&c.token) {
            toks.push(c.token.clone());
            let mut k = c.kind.clone();
            if !c.label.is_empty() {
                k.push(':');
                k.push_str(&c.label);
            }
            kinds.push(k);
        }
    }
    (toks, kinds)
}

impl AppState {
    fn match_canaries(&self, blob: &str) -> (Vec<String>, Vec<String>) {
        match_canaries(&self.canaries, blob)
    }

    fn emit(&self, be: Behavioral) {
        let ev = Event { kind: "behavioral", ts_ms: now_ms(), behavioral: be };
        if let Ok(line) = serde_json::to_string(&ev) {
            if let Ok(mut f) = self.log.lock() {
                let _ = writeln!(f, "{line}");
                let _ = f.flush();
            }
        }
    }
}

#[tokio::main(flavor = "multi_thread", worker_threads = 2)]
async fn main() {
    let args = parse_args();
    let canaries = load_canaries(&args.canaries);
    let log = open_log(&args.log_dir);
    eprintln!(
        "reactor-sink (rust) http={} dns={} canaries={} — nothing egresses past here",
        args.http,
        args.dns,
        canaries.len()
    );
    let state = Arc::new(AppState { canaries, log: Mutex::new(log) });

    if !args.dns.is_empty() {
        tokio::spawn(serve_dns(args.dns.clone(), state.clone()));
    }

    let app = Router::new().fallback(handle).with_state(state);
    let addr: SocketAddr = args.http.parse().expect("bad --http addr");
    let listener = tokio::net::TcpListener::bind(addr).await.expect("bind http");
    axum::serve(listener, app).await.expect("serve");
}

/// The catch-all handler: a direct beacon POST, or a forward-proxy request whose
/// absolute URI carries the intended host. Either way it is logged and the
/// artifact gets a benign 200 so evasion-aware code believes it "worked" — while
/// nothing leaves the chamber.
///
/// GET/HEAD /healthz is the engine's readiness probe and is deliberately not
/// logged: a logged probe used to fire install_hook on every benign detonation.
async fn handle(
    State(state): State<Arc<AppState>>,
    method: Method,
    uri: Uri,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse {
    if method == Method::CONNECT {
        let host = uri.authority().map(|a| a.host().to_string()).unwrap_or_default();
        let mut be = Behavioral { op: "connect".into(), source: "sink".into(), method: "CONNECT".into(), host, port: 443, ..Default::default() };
        let (t, k) = state.match_canaries(&uri.to_string());
        be.canaries = t;
        be.canary_kinds = k;
        state.emit(be);
        return (StatusCode::FORBIDDEN, "reactor-sink: egress contained").into_response();
    }

    let host = uri
        .authority()
        .map(|a| a.host().to_string())
        .or_else(|| headers.get("host").and_then(|h| h.to_str().ok()).map(host_only))
        .unwrap_or_default();
    let port = uri.port_u16().map(|p| p as i64).unwrap_or(80);
    let url_path = uri.path().to_string();

    // Only the exact local probe is silent. A beacon POST to /healthz, or a
    // forward-proxy request whose path happens to match, still logs.
    if (method == Method::GET || method == Method::HEAD) && url_path == "/healthz" {
        return (StatusCode::OK, "ok").into_response();
    }

    let body_str = String::from_utf8_lossy(&body);
    let header_blob = headers
        .iter()
        .map(|(k, v)| format!("{}: {}", k, v.to_str().unwrap_or("")))
        .collect::<Vec<_>>()
        .join("\n");
    let surface = format!("{}\n{}\n{}", body_str, uri, header_blob);
    let (canaries, canary_kinds) = state.match_canaries(&surface);

    let be = Behavioral {
        op: "egress_http".into(),
        source: "sink".into(),
        host,
        port,
        method: method.to_string(),
        url_path,
        body_bytes: body.len() as i64,
        body_sha256: sha256_hex(&body),
        preview: preview(&body_str, 240),
        canaries,
        canary_kinds,
    };
    state.emit(be);

    (StatusCode::OK, "{\"ok\":true}").into_response()
}

/// Minimal mock DNS: resolve every A query to the sink and log the lookup, so a
/// beacon's *intended* hostname becomes evidence even though it never resolves
/// off-box.
async fn serve_dns(addr: String, state: Arc<AppState>) {
    let sock = match tokio::net::UdpSocket::bind(&addr).await {
        Ok(s) => s,
        Err(e) => {
            eprintln!("dns bind failed ({e}); continuing without dns");
            return;
        }
    };
    let mut buf = [0u8; 512];
    loop {
        let (n, from) = match sock.recv_from(&mut buf).await {
            Ok(x) => x,
            Err(_) => continue,
        };
        let name = parse_qname(&buf[..n]);
        let mut be = Behavioral { op: "egress_dns".into(), source: "dns".into(), host: name.clone(), ..Default::default() };
        let (t, k) = state.match_canaries(&name);
        be.canaries = t;
        be.canary_kinds = k;
        state.emit(be);
        if let Some(resp) = dns_answer(&buf[..n]) {
            let _ = sock.send_to(&resp, from).await;
        }
    }
}

// ---- DNS wire helpers (raw, no dependency) ----

fn parse_qname(pkt: &[u8]) -> String {
    if pkt.len() < 13 {
        return String::new();
    }
    let mut labels = Vec::new();
    let mut i = 12usize;
    while i < pkt.len() {
        let l = pkt[i] as usize;
        if l == 0 || i + 1 + l > pkt.len() {
            break;
        }
        labels.push(String::from_utf8_lossy(&pkt[i + 1..i + 1 + l]).to_string());
        i += 1 + l;
    }
    labels.join(".")
}

fn dns_answer(req: &[u8]) -> Option<Vec<u8>> {
    if req.len() < 12 {
        return None;
    }
    let mut i = 12usize;
    while i < req.len() && req[i] != 0 {
        i += req[i] as usize + 1;
    }
    i += 1 + 4; // null label + qtype + qclass
    if i > req.len() {
        return None;
    }
    let mut resp = req[..i].to_vec();
    resp[2] |= 0x80; // QR = response
    resp[6] = 0;
    resp[7] = 1; // one answer
    resp.extend_from_slice(&[0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x1e, 0x00, 0x04, 127, 0, 0, 1]);
    Some(resp)
}

// ---- small helpers ----

fn sha256_hex(b: &[u8]) -> String {
    let mut h = Sha256::new();
    h.update(b);
    hex::encode(h.finalize())
}

fn preview(s: &str, n: usize) -> String {
    let cleaned: String = s.chars().map(|c| if c == '\n' { ' ' } else { c }).collect();
    if cleaned.chars().count() > n {
        let truncated: String = cleaned.chars().take(n).collect();
        format!("{truncated}…")
    } else {
        cleaned
    }
}

fn host_only(h: &str) -> String {
    match h.rfind(':') {
        Some(i) if !h[i..].contains(']') => h[..i].to_string(),
        _ => h.to_string(),
    }
}

fn now_ms() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_millis() as i64).unwrap_or(0)
}

fn load_canaries(path: &str) -> Vec<Canary> {
    if path.is_empty() {
        return Vec::new();
    }
    match std::fs::read(path) {
        Ok(b) => serde_json::from_slice(&b).unwrap_or_default(),
        Err(_) => Vec::new(),
    }
}

fn open_log(dir: &str) -> File {
    let _ = std::fs::create_dir_all(dir);
    let p = Path::new(dir).join("sink.jsonl");
    OpenOptions::new().create(true).append(true).open(p).expect("open sink.jsonl")
}

struct Args {
    http: String,
    dns: String,
    log_dir: String,
    canaries: String,
}

fn parse_args() -> Args {
    let mut m: BTreeMap<String, String> = BTreeMap::new();
    let argv: Vec<String> = std::env::args().collect();
    let mut i = 1;
    while i < argv.len() {
        if let Some(name) = argv[i].strip_prefix("--") {
            let val = argv.get(i + 1).cloned().unwrap_or_default();
            m.insert(name.to_string(), val);
            i += 2;
        } else {
            i += 1;
        }
    }
    let env = |k: &str, d: &str| std::env::var(k).unwrap_or_else(|_| d.to_string());
    Args {
        http: m.get("http").cloned().unwrap_or_else(|| env("REACTOR_SINK_HTTP_ADDR", "127.0.0.1:9931")),
        dns: m.get("dns").cloned().unwrap_or_else(|| env("REACTOR_SINK_DNS_ADDR", "")),
        log_dir: m.get("log-dir").cloned().unwrap_or_else(|| env("REACTOR_LOG_DIR", ".")),
        canaries: m.get("canaries").cloned().unwrap_or_else(|| env("REACTOR_CANARY_FILE", "")),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn canary(token: &str, kind: &str, label: &str) -> Canary {
        Canary { token: token.into(), kind: kind.into(), label: label.into() }
    }

    fn table() -> Vec<Canary> {
        vec![
            canary("REACTOR-ctx01", "context", "system_prompt"),
            canary("REACTOR-env01", "file", "dotenv"),
            canary("REACTOR-bare", "file", ""),
        ]
    }

    /// "REACTOR-a1b2 hit the sink" is the detection event. The match has to be
    /// exact — a near miss reported as a hit is a fabricated exfiltration.
    #[test]
    fn matches_canaries_exactly_and_labels_them() {
        let (toks, kinds) = match_canaries(&table(), r#"{"note":"REACTOR-env01"}"#);
        assert_eq!(toks, vec!["REACTOR-env01"]);
        assert_eq!(kinds, vec!["file:dotenv"]);

        // Multiple canaries in one body stay in parallel with their kinds.
        let (toks, kinds) = match_canaries(&table(), "REACTOR-ctx01 and REACTOR-env01");
        assert_eq!(toks, vec!["REACTOR-ctx01", "REACTOR-env01"]);
        assert_eq!(kinds, vec!["context:system_prompt", "file:dotenv"]);

        // No label => bare kind, no dangling colon.
        let (_, kinds) = match_canaries(&table(), "REACTOR-bare");
        assert_eq!(kinds, vec!["file"]);

        for miss in ["REACTOR-ctx0", "reactor-ctx01", "REACTOR-ctx02", ""] {
            let (toks, _) = match_canaries(&table(), miss);
            assert!(toks.is_empty(), "{miss} should not match");
        }
        // Base64/URL-encoded bodies are a real evasion; the sink only claims
        // literal matches, so this is a documented miss, not a silent one.
        let (toks, _) = match_canaries(&table(), "UkVBQ1RPUi1jdHgwMQ==");
        assert!(toks.is_empty());
    }

    /// The Host header carries a port; the behavioral event's `host` must not.
    #[test]
    fn host_only_strips_the_port_but_keeps_ipv6_literals() {
        assert_eq!(host_only("c2.attacker.net:8443"), "c2.attacker.net");
        assert_eq!(host_only("c2.attacker.net"), "c2.attacker.net");
        assert_eq!(host_only("127.0.0.1:9931"), "127.0.0.1");
        assert_eq!(host_only("[::1]"), "[::1]");
        assert_eq!(host_only(""), "");
    }

    /// preview is the only raw-ish field the sink emits, and it is stripped at
    /// the analyst boundary. It must be bounded and never split a UTF-8
    /// character — an artifact controls these bytes.
    #[test]
    fn preview_is_bounded_and_utf8_safe() {
        assert_eq!(preview("a\nb\nc", 240), "a b c");
        let long = "é".repeat(500);
        let p = preview(&long, 240);
        assert_eq!(p.chars().count(), 241); // 240 chars + the ellipsis
        assert!(p.ends_with('…'));
        assert_eq!(preview("short", 240), "short");
        assert_eq!(preview("", 10), "");
    }

    #[test]
    fn hashes_bodies_stably() {
        assert_eq!(
            sha256_hex(b""),
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        );
        assert_ne!(sha256_hex(b"a"), sha256_hex(b"b"));
    }

    // ---- DNS: a beacon's intended hostname is evidence even though it never
    // resolves off-box. The packet comes from untrusted code, so truncated and
    // malformed queries must be handled, not trusted.

    fn query(name: &str) -> Vec<u8> {
        let mut p = vec![0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0];
        for label in name.split('.') {
            p.push(label.len() as u8);
            p.extend_from_slice(label.as_bytes());
        }
        p.push(0);
        p.extend_from_slice(&[0x00, 0x01, 0x00, 0x01]); // A, IN
        p
    }

    #[test]
    fn parses_the_queried_name() {
        assert_eq!(parse_qname(&query("c2.attacker.net")), "c2.attacker.net");
        assert_eq!(parse_qname(&query("a")), "a");
    }

    #[test]
    fn survives_truncated_and_malformed_dns_packets() {
        assert_eq!(parse_qname(&[]), "");
        assert_eq!(parse_qname(&[0u8; 12]), "");
        // A length byte that runs past the end of the packet.
        let mut lying = vec![0u8; 12];
        lying.push(200);
        lying.extend_from_slice(b"short");
        assert_eq!(parse_qname(&lying), "");

        assert!(dns_answer(&[]).is_none());
        assert!(dns_answer(&[0u8; 5]).is_none());
    }

    /// Every A query resolves to the sink, so the beacon "succeeds" and the
    /// artifact keeps talking — while nothing leaves the chamber.
    #[test]
    fn answers_every_query_with_the_sink_address() {
        let q = query("c2.attacker.net");
        let a = dns_answer(&q).expect("answer");
        assert_eq!(a[2] & 0x80, 0x80, "QR bit must mark it a response");
        assert_eq!(u16::from_be_bytes([a[6], a[7]]), 1, "exactly one answer");
        assert_eq!(&a[..2], &q[..2], "transaction id must be echoed");
        assert_eq!(&a[a.len() - 4..], &[127, 0, 0, 1], "resolves to the sink");
    }
}
