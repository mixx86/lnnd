# Roadmap

Where lnnd is going, in the order the work is expected to land. Each phase
builds on the previous one; items inside a phase can be done in any order.

Status per feature is also summarised in the [README](../README.md#status).
Design details live in the linked documents.

---

## Phase 0 — Foundations and hygiene

Make the codebase consistent before it grows further.

- [x] Naming consistency pass (`message.go`, `Credentials`, `OK`, `Symlink`,
      `Checksum`, `MsgType`, `ipc.HandleConn`).
- [x] Apply the rest of the coding style: 80-column limit (`make lll` still
      reports long comment lines) and *why*-only comments.
- [x] `fmt`, `golines` and `lll` Makefile targets.
- [x] Run `lll` in CI once the codebase passes it.
- [x] Align the Go version: README and `go.mod` both require Go 1.26.
- [x] Configurable monitored disk (`LNND_DISK_DEVICE`), auto-detected when
      unset.
- [x] Load the configuration from `~/.lnn/lnnd.conf`, independently of the
      working directory.
- [x] Rename the `SCREAMING_CASE` constants in `internal/store` to Go style
      (`F_HEADER_MAGIC_LEN` → `fileHeaderMagicLen`, `OP_KIND` → `opKind`).
- [ ] Test coverage for `protocol` (encode/parse round trips), `network`
      (handshake) and `sched` (turns and backpressure).

## Phase 1 — Durable inbound pipeline

Finish the path from "peer sends a request" to "request is safely on disk".
See [architecture.md](architecture.md#inbound-request-lifecycle).

- [ ] Inbound request queue + write scheduler: finish adaptive tuning from
      `RecordWriteResult` baselines.
- [x] Requests WAL + crash recovery: outbox on the sender, pending
      recoveries on the receiver, `RECOVER_REQUEST` / `ErrUnknownTag`
      handling, 32-bit per-peer tags.
- [x] Send `RECOVER_REQUEST` when a peer with pending recoveries completes
      the inbound handshake.
- [ ] IPC sends requests through the outbox (tags + pending message), and
      outbound sessions also send pending `RECOVER_REQUEST`s.
- [ ] Session state machine: explicit states and transitions, reject
      messages that are invalid for the current state with
      `ErrInvalidState`.

## Phase 2 — Link operations

The first user-visible feature: creating links between machines.
See [protocol.md](protocol.md) and [ipc.md](ipc.md).

- [ ] Request resolution: `--accept` / `--reject` over IPC, `ACCEPTED` /
      `REJECTED` WAL entries, `RESOLUTION` message to the peer.
- [ ] Choosing `dst` on `--accept` when the sender omitted it.
- [ ] IPC operations to list pending requests.
- [ ] Remote symlinks end to end (`SYMLINK_REQUEST` → resolution → link).
- [ ] Remote hard links (`HARDLINK_REQUEST`, `hard` IPC type).
- [ ] Persistent link database: which links exist, to which peer and path.

## Phase 3 — Peers and trust

Let users decide what each peer is allowed to do.
See [trust-model.md](trust-model.md).

- [ ] Peer management: `--add-peer`, `--remove-peer`, peer list.
- [ ] Permission templates (`unknown`, `trusted`, `max`, custom) in
      `~/.lnn/templates/`.
- [ ] Anti-flood rules per peer.
- [ ] Direct link messages (`LINK_SYM`, `LINK_HARD`) for peers with path
      permissions.
- [ ] Alerts.

## Phase 4 — Transport and security

- [ ] TLS encryption on top of the Ed25519 handshake.
- [ ] QUIC transport (`QUIC_PORT`), behind the existing `Transport`
      abstraction.

## Phase 5 — Filesystem presentation

Expose remote links as regular filesystem entries.
See [filesystem.md](filesystem.md).

- [ ] FUSE mount at `~/.lnn/<ip>:/` (io_uring + FUSE).
- [ ] IP change handling via mount directory rename.
- [ ] Peer naming (`lnn --name raspi 192.168.1.5`).

## Phase 6 — Beyond links

- [ ] Sync mode.
- [ ] Smart links.
- [ ] Optional kernel drivers: UX driver (`ip://path` presentation) and
      performance driver (zero-copy, native page cache).
