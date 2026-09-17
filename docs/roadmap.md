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
- [x] Protocol redesign — `Message` struct (Header + Payload), `Payload` interface (`Encode`, `Type`, `Size`), `ReceiveTCP` returns `protocol.Message`
- [x] Tag/correlation ID system — `RegisterPending` / `Resolve` / `CancelPending` for async request-response matching
- [x] `SymLinkRequest` encoder/parser — variable-length payload, absolute path validation
- [x] Inbound request queue (`iocore`) — non-blocking enqueue with backpressure from io_uring pressure signal
- [ ] Split `iocore` into `reqqueue` (request queue, backpressure) and `ioring` (io_uring pressure signal; future scheduler)
- [x] Write-ahead log (`wal`) — append-only log with checkpoint and startup replay; tracks accept/reject resolutions

---

## Phase 2 — Remote links
*Core feature: creating links across machines.*

- [x] Concurrent write protection on `Peer` — `sendMu sync.Mutex` + `sendRaw` for handshake (which doesn't have a peer yet), `SendTCP(*Peer)` post-handshake
- [ ] Link message handling in the daemon (`LINK_SYM_REQUEST`, `LINK_HARD_REQUEST`)
- [ ] IPC → network bridge (forward CLI requests to the correct peer)
- [ ] `lnn <ip>://path ./link` — remote hard link, local copy (default)
- [ ] `lnn -s <ip>://path ./link` — remote symlink, virtual via FUSE (default for `-s`)
- [ ] `lnn -s --local <ip>://path ./link` — remote symlink, local copy
- [ ] `lnn --virtual <ip>://path ./link` — remote hard link, virtual via FUSE (inode-tracked)
- [ ] FUSE mount — one mount point per peer at `~/.lnn/<ip>:/`; virtual link entries point into these directories (e.g. `./link -> ~/.lnn/192.168.1.5:/path/to/file`)
- [ ] IP change handling — on reconnect with known UUID but new IP, atomically rename `~/.lnn/<old-ip>:/` to `~/.lnn/<new-ip>:/`; existing link entries remain valid
- [ ] `OK` / `ERROR` response propagation back to CLI
- [ ] Graceful connection teardown (`StateClosing`)

---

## Phase 3 — Peer management
*Trust model, permission templates, anti-flooding, and alerts.*

**Peer commands**
- [ ] `lnn --add-peer <ip>` — add peer using the default `trusted` template
- [ ] `lnn --name <alias> <ip>` — assign a friendly name to a peer; renames `~/.lnn/<ip>:/` to `~/.lnn/<alias>/`; all existing link entries remain valid
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

## Phase 7 — Advanced features
*Power-user and observability features.*

- [ ] `--sync` mode — keep remote link up to date with source changes
- [ ] Smart links — rule-based linking (e.g., link all `.log` files matching a pattern)
- [ ] `--analyze` — show inode info, hard link count, link targets
- [ ] `--graph` — visualize link relationships between peers
- [ ] High-throughput mode — thousands of concurrent links; io_uring for transfers >64 KB (submission/completion queues, zero syscall overhead in the hot path)
- [ ] io_uring priority scheduler — weighted priority function combining deadline urgency and file size: `priority = w1 * (1/time_remaining) + w2 * (1/file_size)`; aging via dynamic recalculation at extraction time prevents starvation of large files; implemented as `container/heap` with priorities recalculated on pop, not on insert
- [ ] FUSE data-path optimization — splice for zero-copy serving (socket → pipe → `/dev/fuse`, no userspace buffer); POLL_ADD on `/dev/fuse` fd for non-blocking operation; FUSE_URING (kernel 6.14+) for direct kernel↔daemon ring without extra context switches per FUSE op
- [ ] Optional UX kernel module — presents link entries as `192.168.1.5://file` directly in `ls` and standard tools instead of `~/.lnn/<ip>:/file`; surfaces link metadata (type, peer name, sync status) natively via the VFS; requires kernel module installation
- [ ] Optional performance kernel module — replaces FUSE with a native VFS driver; enables true zero-copy (network socket → page cache without passing through lnnd) and native page cache integration (kernel-managed read-ahead and cache coherency); recommended for corporate environments with large file transfers at wire speed or high-concurrency small file workloads
- [ ] QUIC transport — replace TCP with QUIC (TLS 1.3 built-in, certificate pinning via Ed25519 public key, independent streams per connection, no head-of-line blocking; audit handshake for replay and MITM vectors during migration)
- [ ] Configurable encryption method per peer

---

## Ideas / Backlog
*Not scheduled, subject to change.*

- Custom UDP multicast peer discovery — signed Ed25519 announcements (UUID + port + timestamp + pubkey + signature); `lnn --peers local` with terminal UI listing nearby lnnd nodes for easy `--add-peer`
- Multi-server / client topology — optional model where multiple lnnd nodes serve shared resources and clients consume from all of them; requires consistency protocol, write coordination, and resource discovery; depends on P2P lnn being mature and real deployment feedback
- `lnn` man page
