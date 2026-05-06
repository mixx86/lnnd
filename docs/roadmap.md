# Roadmap

Development is tracked in phases. Each phase builds on the previous one and produces a usable increment.

---

## Phase 1 — Core infrastructure
*Foundation: daemon, identity, transport, and authentication.*

- [x] Daemon skeleton (`lnnd start/stop/status`)
- [x] Node identity — persistent UUID + Ed25519 keypair (`~/.lnn/`)
- [x] TCP transport layer
- [x] Binary wire protocol (header + typed messages)
- [x] Mutual Ed25519 handshake (challenge-response)
- [x] Peer state machine (`New → Handshake → Verified → Closing`)
- [x] Thread-safe peer registry (`PeerManager`)
- [x] IPC — Unix socket + JSON request parsing
- [x] Application message loop (`HandleTCPMessage`)

---

## Phase 2 — Remote links
*Core feature: creating links across machines.*

- [x] Concurrent write protection on `Peer` — `sendMu sync.Mutex` + `sendRaw` for handshake (which doesn't have a peer yet), `SendTCP(*Peer)` post-handshake
- [ ] Link message handling in the daemon (`LINK_SYM_REQUEST`, `LINK_HARD_REQUEST`)
- [ ] IPC → network bridge (forward CLI requests to the correct peer)
- [ ] `lnn <ip>://path ./link` — remote symlink via CLI
- [ ] `lnn <ip>://path ./link` — remote hard link via CLI
- [ ] `OK` / `ERROR` response propagation back to CLI
- [ ] Graceful connection teardown (`StateClosing`)

---

## Phase 3 — Peer management
*Trust model, permission templates, anti-flooding, and alerts.*

**Peer commands**
- [ ] `lnn --add-peer <ip>` — add peer using the default `trusted` template
- [ ] `lnn --add-peer <ip> --max` — add peer with max trust (`/*`)
- [ ] `lnn --add-peer <ip> --template <name>` — add peer using a custom template
- [ ] `lnn --peer <ip> --allow <path>` — grant an extra path to an existing peer (not written to template)
- [ ] `lnn --peer <ip> --anti-flooding <rule>` — update rate limit for a peer (e.g. `4hours;3requests`)
- [ ] `lnn --block <ip>` — block a peer
- [ ] `lnn --peers` — list known peers with template, paths, and rate limit
- [ ] `lnn --peers <ip>` — show full peer info (UUID, public key, paths, template, rate limit)

**Removing permissions**
- [ ] `lnn --remove-peer <ip>` — strip all paths, apply `unknown` template (anti-flood only)
- [ ] `lnn --remove-peer <ip> --template <name>` — remove only template paths, keep extras and rate limit
- [ ] `lnn --remove-peer <ip> /*` — remove all path permissions
- [ ] `lnn --remove-peer <ip> /path/to/specific` — remove a single path

**Templates**
- [ ] Default templates on install: `unknown` (anti-flood only), `trusted`, `max`
- [ ] Template format: first line = anti-flood rule, remaining lines = allowed paths
- [ ] Custom template management (`~/.lnn/templates/<name>`)
- [ ] Template applied as snapshot at `--add-peer` time (no live link)

**Anti-flooding**
- [ ] Anti-flood rule parser (`4hours;3requests`, `1week;9requests`, `never;900requests`)
- [ ] Per-peer request counter with time-window tracking
- [ ] `never;<n>requests` — permanent block after n requests total
- [ ] Rate limit applied to all peers, including `unknown`

**Enforcement & alerts**
- [ ] Path matching engine (`/foo/bar`, `/foo/*`, `/*`)
- [ ] Inbound `LINK_SYM`/`LINK_HARD` path validation against peer's allowed set
- [ ] Inbound connection filtering (reject blocked peers)
- [ ] Alert on rate limit exceeded
- [ ] Alert on direct link attempt to unauthorized path
- [ ] Alert on malformed or suspicious path in link message
- [ ] `lnn --alert` — show unread alerts
- [ ] `lnn --alert all` — show full alert history
- [ ] Alert storage (`~/.lnn/alerts`)

---

## Phase 4 — Request workflow
*Async link requests between peers.*

- [ ] `lnn <ip>://path/to/file [./local_link]` — send a link request; dst is optional, receiver decides if omitted
- [ ] `lnn --requests` — list pending incoming requests
- [ ] `lnn --requests all` — list all requests (pending + history)
- [ ] `lnn --accept <ip>:<id> [/path]` — accept a request; /path required only if sender did not specify dst — if sender did specify dst, it cannot be changed
- [ ] `lnn --reject <ip>:<id>` — reject a request

---

## Phase 5 — CLI (`lnn`)
*Complete the C CLI as a drop-in replacement for `ln`.*

- [ ] All original `ln` flags (`-s`, `-f`, `-n`, `-v`, `-b`, etc.)
- [ ] Local hard links and symlinks (delegate to system `ln` or reimplement)
- [ ] Remote syntax parsing (`<ip>://path`)
- [ ] IPC client — send JSON to `lnnd` and print response
- [ ] Error messages matching `ln` conventions

---

## Phase 6 — Reliability
*Make remote links survive network interruptions.*

- [ ] Persistent link database (track all active remote links)
- [ ] Automatic reconnection with exponential backoff
- [ ] Link health monitoring
- [ ] Restore links on daemon restart

---

## Phase 7 — Security hardening
*Encrypt the transport layer.*

- [ ] TLS 1.3 over TCP (replace plaintext transport)
- [ ] Certificate pinning based on Ed25519 public key
- [ ] Configurable cipher suites
- [ ] Audit existing handshake for replay and MITM vectors

---

## Phase 8 — Advanced features
*Power-user and observability features.*

- [ ] `--sync` mode — keep remote link up to date with source changes
- [ ] Smart links — rule-based linking (e.g., link all `.log` files matching a pattern)
- [ ] `--analyze` — show inode info, hard link count, link targets
- [ ] `--graph` — visualize link relationships between peers
- [ ] High-throughput mode — thousands of concurrent links
- [ ] Configurable encryption method per peer

---

## Ideas / Backlog
*Not scheduled, subject to change.*

- mDNS peer discovery on the local network
- `lnn` man page
