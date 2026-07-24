//! reactor-collect — the syscall collector (SPEC §4.3 "Syscalls").
//!
//! It parses `strace -f` output from an artifact's install/entrypoint run and
//! turns it into typed `BehavioralEvent`s: reads of planted credential bait
//! (canary_read), writes outside the install directory (install_hook), spawned
//! processes, and network connects. Everything downstream — the oracles, the
//! analyst — sees only these typed events, never the raw trace.
//!
//! This is a hot loop over megabytes of untrusted, adversarially-shaped strace
//! text, which is exactly where Rust's bounds- and memory-safety earn their
//! keep: a malformed trace line can never walk off the end of a buffer here.

use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, HashSet},
    fs::{File, OpenOptions},
    io::{BufRead, BufReader, Write},
    path::Path,
};

#[derive(Deserialize)]
struct BaitPath {
    path: String,
    #[serde(default)]
    label: String,
}

#[derive(Serialize, Default)]
struct Behavioral {
    op: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    source: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    path: String,
    #[serde(skip_serializing_if = "is_false")]
    bait: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    bait_label: String,
    #[serde(skip_serializing_if = "is_false")]
    in_install_dir: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    proc: String,
    #[serde(skip_serializing_if = "is_false")]
    before_first_tool_call: bool,
}

fn is_false(b: &bool) -> bool {
    !*b
}

#[derive(Serialize)]
struct Event {
    kind: &'static str,
    ts_ms: i64,
    session: i64,
    behavioral: Behavioral,
}

struct Args {
    strace: String,
    bait: String,
    install_dir: String,
    home: String,
    out: String,
    session: i64,
}

fn main() {
    let args = parse_args();
    let baits = load_baits(&args.bait);
    let mut out = open_out(&args.out);

    let file = match File::open(&args.strace) {
        Ok(f) => f,
        Err(_) => return, // no trace (e.g. strace unavailable) — nothing to do
    };
    let reader = BufReader::new(file);

    // Noise prefixes: package-manager and runtime scratch that a benign install
    // legitimately writes. Writes here are never treated as an install hook.
    let noise = noise_prefixes(&args.home);
    let mut seen: HashSet<String> = HashSet::new();

    for line in reader.lines().map_while(Result::ok) {
        let Some(sc) = Syscall::parse(&line) else { continue };
        match sc.name {
            "openat" | "open" | "creat" => {
                if sc.failed {
                    continue;
                }
                let Some(path) = sc.first_path() else { continue };
                // A read of a planted bait file — the credential-access signal.
                if let Some(label) = match_bait(&path, &baits) {
                    let key = format!("read:{path}");
                    if seen.insert(key) {
                        emit(&mut out, args.session, Behavioral {
                            op: "file_read".into(), source: "strace".into(),
                            path: path.clone(), bait: true, bait_label: label,
                            before_first_tool_call: true, ..Default::default()
                        });
                    }
                }
                // A write outside the install dir — the dropper/persistence signal.
                if sc.is_write() && is_external_write(&path, &args.install_dir, &args.home, &noise) {
                    let key = format!("write:{path}");
                    if seen.insert(key) {
                        emit(&mut out, args.session, Behavioral {
                            op: "file_write".into(), source: "strace".into(),
                            path, in_install_dir: false,
                            before_first_tool_call: true, ..Default::default()
                        });
                    }
                }
            }
            "execve" => {
                if let Some(bin) = sc.first_path() {
                    let key = format!("exec:{bin}");
                    if seen.insert(key) {
                        emit(&mut out, args.session, Behavioral {
                            op: "process_spawn".into(), source: "strace".into(),
                            proc: bin, before_first_tool_call: true, ..Default::default()
                        });
                    }
                }
            }
            "connect" | "sendto" | "sendmsg" | "sendmmsg" => {
                if sc.failed {
                    continue;
                }
                let key = format!("net:{}", sc.args_raw);
                if seen.insert(key) {
                    emit(&mut out, args.session, Behavioral {
                        op: "connect".into(), source: "strace".into(),
                        before_first_tool_call: true, ..Default::default()
                    });
                }
            }
            _ => {}
        }
    }
}

/// A parsed strace line: syscall name, raw args, and success/failure.
struct Syscall<'a> {
    name: &'a str,
    args_raw: &'a str,
    failed: bool,
}

impl<'a> Syscall<'a> {
    fn parse(line: &'a str) -> Option<Syscall<'a>> {
        // Strip a `[pid 1234] ` or leading `1234  ` prefix, and any timestamp.
        let mut s = line.trim_start();
        if let Some(rest) = s.strip_prefix('[') {
            if let Some(end) = rest.find(']') {
                s = rest[end + 1..].trim_start();
            }
        }
        // Drop a leading numeric pid token.
        if let Some((first, rest)) = s.split_once(char::is_whitespace) {
            if first.chars().all(|c| c.is_ascii_digit()) && !first.is_empty() {
                s = rest.trim_start();
            }
        }
        // Signal lines, exits, unfinished/resumed markers: skip.
        if s.starts_with("---") || s.starts_with("+++") || s.contains("<unfinished") || s.starts_with('<') {
            return None;
        }
        let paren = s.find('(')?;
        let name = &s[..paren];
        if name.is_empty() || !name.chars().all(|c| c.is_ascii_alphanumeric() || c == '_') {
            return None;
        }
        let args_raw = &s[paren + 1..];
        let failed = s.rsplit('=').next().map(|r| r.trim_start().starts_with("-1")).unwrap_or(false);
        Some(Syscall { name, args_raw, failed })
    }

    /// The first double-quoted string in the args — the path for open/openat and
    /// the binary for execve. openat's first quoted arg after AT_FDCWD is the
    /// path, so scanning for the first quote handles both forms.
    fn first_path(&self) -> Option<String> {
        extract_quoted(self.args_raw)
    }

    fn is_write(&self) -> bool {
        self.args_raw.contains("O_WRONLY")
            || self.args_raw.contains("O_RDWR")
            || self.args_raw.contains("O_CREAT")
            || self.name == "creat"
    }
}

/// Extract the first C-style double-quoted token, honoring backslash escapes.
fn extract_quoted(s: &str) -> Option<String> {
    let bytes = s.as_bytes();
    let start = bytes.iter().position(|&b| b == b'"')? + 1;
    let mut out = String::new();
    let mut i = start;
    while i < bytes.len() {
        match bytes[i] {
            b'\\' if i + 1 < bytes.len() => {
                // strace escapes; keep the next char literally.
                out.push(bytes[i + 1] as char);
                i += 2;
            }
            b'"' => return Some(out),
            c => {
                out.push(c as char);
                i += 1;
            }
        }
    }
    None
}

fn match_bait(path: &str, baits: &[BaitPath]) -> Option<String> {
    for b in baits {
        if path == b.path || path.ends_with(&suffix(&b.path)) {
            return Some(if b.label.is_empty() { "bait".into() } else { b.label.clone() });
        }
    }
    None
}

/// The `~/.thing` tail of a bait path, so a differently-rooted HOME still matches.
fn suffix(p: &str) -> String {
    if let Some(idx) = p.find("/.") {
        p[idx..].to_string()
    } else if let Some(idx) = p.rfind('/') {
        p[idx..].to_string()
    } else {
        p.to_string()
    }
}

fn is_external_write(path: &str, install_dir: &str, home: &str, noise: &[String]) -> bool {
    if !install_dir.is_empty() && path.starts_with(install_dir) {
        return false; // writes in the install dir are benign by definition
    }
    for n in noise {
        if path.starts_with(n) {
            return false;
        }
    }
    // Only care about writes that land in the user's home outside the install
    // dir — that is where persistence and dropped payloads live.
    !home.is_empty() && path.starts_with(home)
}

fn noise_prefixes(home: &str) -> Vec<String> {
    let mut v = vec!["/tmp".to_string(), "/proc".to_string(), "/dev".to_string(), "/var/tmp".to_string()];
    for rel in [".npm", ".cache", ".node", ".config/npm", "logs", ".reactor", "node_modules", ".local/share/npm"] {
        v.push(format!("{}/{}", home.trim_end_matches('/'), rel));
    }
    v
}

fn emit(out: &mut File, session: i64, be: Behavioral) {
    let ev = Event { kind: "behavioral", ts_ms: now_ms(), session, behavioral: be };
    if let Ok(line) = serde_json::to_string(&ev) {
        let _ = writeln!(out, "{line}");
        let _ = out.flush();
    }
}

fn now_ms() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_millis() as i64).unwrap_or(0)
}

fn load_baits(path: &str) -> Vec<BaitPath> {
    if path.is_empty() {
        return Vec::new();
    }
    std::fs::read(path).ok().and_then(|b| serde_json::from_slice(&b).ok()).unwrap_or_default()
}

fn open_out(path: &str) -> File {
    if let Some(dir) = Path::new(path).parent() {
        let _ = std::fs::create_dir_all(dir);
    }
    OpenOptions::new().create(true).append(true).open(path).expect("open behavioral.jsonl")
}

fn parse_args() -> Args {
    let mut m: BTreeMap<String, String> = BTreeMap::new();
    let argv: Vec<String> = std::env::args().collect();
    let mut i = 1;
    while i < argv.len() {
        if let Some(name) = argv[i].strip_prefix("--") {
            m.insert(name.to_string(), argv.get(i + 1).cloned().unwrap_or_default());
            i += 2;
        } else {
            i += 1;
        }
    }
    Args {
        strace: m.remove("strace").unwrap_or_default(),
        bait: m.remove("bait").unwrap_or_default(),
        install_dir: m.remove("install-dir").unwrap_or_default(),
        home: m.remove("home").unwrap_or_default(),
        out: m.remove("out").unwrap_or_else(|| "behavioral.jsonl".into()),
        session: m.remove("session").and_then(|s| s.parse().ok()).unwrap_or(1),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn bait(path: &str, label: &str) -> BaitPath {
        BaitPath { path: path.into(), label: label.into() }
    }

    /// The trace is `strace -f` output from an untrusted process: interleaved
    /// pids, signal lines, unfinished/resumed splits. Anything the parser
    /// mis-reads is either a missed signal or a fabricated one.
    #[test]
    fn parses_the_shapes_strace_actually_emits() {
        let openat = Syscall::parse(
            r#"[pid  4711] openat(AT_FDCWD, "/home/agent/.env", O_RDONLY|O_CLOEXEC) = 3"#,
        )
        .expect("pid-prefixed openat");
        assert_eq!(openat.name, "openat");
        assert_eq!(openat.first_path().as_deref(), Some("/home/agent/.env"));
        assert!(!openat.failed);
        assert!(!openat.is_write());

        // `strace -f` without -o uses a bare pid column instead of [pid N].
        let bare = Syscall::parse(r#"4711  execve("/usr/bin/curl", ["curl", "-d", "@-"], 0x7f) = 0"#)
            .expect("bare-pid execve");
        assert_eq!(bare.name, "execve");
        assert_eq!(bare.first_path().as_deref(), Some("/usr/bin/curl"));

        let write = Syscall::parse(r#"openat(AT_FDCWD, "/home/agent/.bashrc", O_WRONLY|O_CREAT, 0644) = 4"#)
            .expect("write");
        assert!(write.is_write());
        assert!(Syscall::parse(r#"creat("/home/agent/x", 0644) = 4"#).unwrap().is_write());
    }

    /// A failed syscall is an *attempt*, not a behaviour. Reporting ENOENT
    /// probes as credential reads would inflate every scorecard.
    #[test]
    fn detects_failed_syscalls() {
        let f = Syscall::parse(r#"openat(AT_FDCWD, "/home/agent/.aws/credentials", O_RDONLY) = -1 ENOENT (No such file or directory)"#).unwrap();
        assert!(f.failed);
        let ok = Syscall::parse(r#"openat(AT_FDCWD, "/home/agent/.env", O_RDONLY) = 3"#).unwrap();
        assert!(!ok.failed);
        // A negative-looking return that is not -1 is still a success.
        let big = Syscall::parse(r#"read(3, "x", 1) = 1"#).unwrap();
        assert!(!big.failed);
    }

    #[test]
    fn skips_non_syscall_lines() {
        for line in [
            "--- SIGCHLD {si_signo=SIGCHLD} ---",
            "+++ exited with 0 +++",
            r#"[pid 4711] openat(AT_FDCWD, "/x" <unfinished ...>"#,
            "<... openat resumed>) = 3",
            "",
            "   ",
            "no parens here",
            "not-a-syscall(x) = 0", // '-' is not legal in a syscall name
            "  |  0x00 41 42(x) = 0", // -x hexdump output
        ] {
            assert!(Syscall::parse(line).is_none(), "should have skipped: {line}");
        }
    }

    /// Adversarial paths: strace escapes them, and an artifact chooses them.
    /// Nothing here may panic or run off the end of the buffer.
    #[test]
    fn extracts_quoted_paths_including_escapes_and_truncation() {
        assert_eq!(extract_quoted(r#"AT_FDCWD, "/a/b", O_RDONLY"#).as_deref(), Some("/a/b"));
        // strace escapes embedded quotes and backslashes.
        assert_eq!(extract_quoted(r#""/home/a\"b/c""#).as_deref(), Some("/home/a\"b/c"));
        assert_eq!(extract_quoted(r#""/home/a\\b""#).as_deref(), Some("/home/a\\b"));
        // Unterminated quote: no path, no panic.
        assert_eq!(extract_quoted(r#""/home/unterminated"#), None);
        assert_eq!(extract_quoted("no quotes at all"), None);
        assert_eq!(extract_quoted(""), None);
        assert_eq!(extract_quoted(r#""""#).as_deref(), Some(""));
        // A trailing lone backslash must not index past the end.
        assert_eq!(extract_quoted(r#""abc\"#), None);
    }

    /// Bait is planted under the chamber HOME, but the trace may show a
    /// differently-rooted path (a symlinked or bind-mounted home). Matching on
    /// the `~/.thing` tail keeps the signal; matching too loosely invents one.
    #[test]
    fn matches_bait_by_exact_path_or_dotfile_tail() {
        let baits = vec![
            bait("/home/agent/.env", "dotenv"),
            bait("/home/agent/.ssh/id_rsa", "ssh_key"),
        ];
        assert_eq!(match_bait("/home/agent/.env", &baits).as_deref(), Some("dotenv"));
        assert_eq!(match_bait("/tmp/reactor/home/.env", &baits).as_deref(), Some("dotenv"));
        assert_eq!(match_bait("/other/root/.ssh/id_rsa", &baits).as_deref(), Some("ssh_key"));
        assert_eq!(match_bait("/home/agent/notes.md", &baits), None);
        assert_eq!(match_bait("/home/agent/.envrc", &baits), None);
        assert_eq!(match_bait("", &baits), None);

        // An unlabelled bait entry still reports as bait.
        assert_eq!(match_bait("/x/.env", &[bait("/home/agent/.env", "")]).as_deref(), Some("bait"));
    }

    #[test]
    fn suffix_anchors_on_the_dotfile_segment() {
        assert_eq!(suffix("/home/agent/.env"), "/.env");
        assert_eq!(suffix("/home/agent/.ssh/id_rsa"), "/.ssh/id_rsa");
        assert_eq!(suffix("/home/agent/wallet.dat"), "/wallet.dat");
        assert_eq!(suffix("relative"), "relative");
    }

    /// install_hook fires on a write outside the install dir. Package-manager
    /// and runtime scratch are the false positives that would make a benign
    /// `npm install` look like a dropper, so they are excluded by prefix.
    #[test]
    fn external_write_excludes_install_dir_and_package_manager_noise() {
        let home = "/home/agent";
        let install = "/home/agent/artifact";
        let noise = noise_prefixes(home);

        // The real signal: persistence written into the home dir.
        assert!(is_external_write("/home/agent/.bashrc", install, home, &noise));
        assert!(is_external_write("/home/agent/.config/autostart/x.desktop", install, home, &noise));

        // Benign by definition.
        assert!(!is_external_write("/home/agent/artifact/build/out.js", install, home, &noise));
        assert!(!is_external_write("/home/agent/.npm/_cacache/x", install, home, &noise));
        assert!(!is_external_write("/home/agent/node_modules/x/index.js", install, home, &noise));
        assert!(!is_external_write("/home/agent/logs/wire.jsonl", install, home, &noise));
        assert!(!is_external_write("/home/agent/.reactor/state", install, home, &noise));
        assert!(!is_external_write("/tmp/scratch", install, home, &noise));
        assert!(!is_external_write("/proc/self/maps", install, home, &noise));

        // Outside HOME entirely is not this oracle's business (the chamber is
        // the boundary for that), and must not be reported as an install hook.
        assert!(!is_external_write("/etc/passwd", install, home, &noise));
    }

    /// A trailing slash on HOME must not break the noise prefixes — otherwise
    /// every npm write becomes a dropper signal.
    #[test]
    fn noise_prefixes_tolerate_a_trailing_slash() {
        let noise = noise_prefixes("/home/agent/");
        assert!(noise.iter().any(|n| n == "/home/agent/.npm"));
        assert!(!noise.iter().any(|n| n.contains("//")));
    }
}
