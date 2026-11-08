# lnnd architecture

lnnd is the background daemon of lnn. It is made of **functional subsystems**, each responsible for one domain, built on **shared infrastructure** that provides the services they have in common. Users never talk to it directly: the [`lnn`](https://github.com/mixx86/lnn) CLI drives it over a local Unix socket.

- **Shared infrastructure** — the orchestrator that wires everything together, configuration, logging, the node identity, the wire protocol, the network transport with authenticated peer sessions, the local IPC interface, and the write scheduler that arbitrates disk writes and applies backpressure.
- **Functional subsystems** — each one owns its domain's messages, on-disk state, recovery rules and lifecycle, plus a queue and a write source if it needs them. **Requests** (link requests exchanged between peers) is the first and most developed subsystem; the **outbox** and **recovery** subsystems support it today; file transfers, link management and peer trust are planned.

There is no generic subsystem interface, registry or plugin mechanism: `daemon.New` builds every component explicitly and hands each one the shared services it needs, and the network read loop calls the subsystem that owns each message directly. Subsystems are free to differ — not every one has a queue, takes write turns or follows the request lifecycle described below.

Related documents: [protocol.md](protocol.md) (LNNP wire protocol), [ipc.md](ipc.md) (CLI ↔ daemon contract), [trust-model.md](trust-model.md) and [filesystem.md](filesystem.md) (planned features).

```
  lnn CLI                                              Remote peers
     │ Unix socket (JSON)                         TCP (LNNP) │
┌────┼───────────────────────────────────────────────────────┼─────┐
│    ▼                         lnnd                          ▼     │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ Shared infrastructure                                        │ │
│ │  IPC listener · network transport, handshake, peer sessions, │ │
│ │  message dispatch · write scheduler (write turns, disk-load  │ │
│ │  monitor, backpressure) · identity · configuration · logging │ │
│ └──────────────────────────────┬───────────────────────────────┘ │
│                                │ shared services; each peer      │
│                                │ message goes to its subsystem   │
│ ┌──────────────────────────────┴───────────────────────────────┐ │
│ │ Functional subsystems                                        │ │
│ │ ┌────────────┐ ┌─────────────┐ ┌─────────────┐ ┌───────────┐ │ │
│ │ │ Requests   │ │ Outbox      │ │ Recovery    │ │ File      │ │ │
│ │ │ queue, WAL,│ │ sent frames │ │ requests    │ │ transfers │ │ │
│ │ │ cache      │ │ per peer    │ │ owed after  │ │ (planned) │ │ │
│ │ │            │ │ until acked │ │ a crash     │ │           │ │ │
│ │ └────────────┘ └─────────────┘ └─────────────┘ └───────────┘ │ │
│ └──────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

---

## Shared infrastructure

| Component                 | Packages                                  | Provides |
|---------------------------|-------------------------------------------|----------|
| Orchestrator              | `daemon`, `app` (`internal/services.go`)  | Builds the shared services and the subsystems and starts them; `app.Services` bundles what they share |
| Configuration             | `config`, `configs/`                      | `~/.lnn/lnnd.conf`, network settings, data directory, memory limits, monitored disk ([Configuration](#configuration)) |
| Logging                   | `logger`                                  | The `lnnd` and `lnnp` loggers |
| Identity                  | `identity`                                | Node UUID and Ed25519 keypair, used to authenticate every connection ([Identity storage](#identity-storage)) |
| Wire protocol             | `protocol`                                | LNNP frames, message types and error codes for the messages of every subsystem ([protocol.md](protocol.md)) |
| Network and peer sessions | `network`, `peer`                         | Transport abstraction (TCP), listener, framing, handshake, session registry, pending-message tracking, and the read loop that dispatches each peer message to the subsystem that owns it |
| IPC                       | `ipc`                                     | Unix-socket interface used by the CLI ([ipc.md](ipc.md)) |
| Write scheduler           | `sched`, `queues`                         | Write turns per workload source, disk-load monitor, queue backpressure, and the in-memory queues of the sources that have one ([Write scheduler](#write-scheduler)) |
| Filesystem utilities      | `fsutil`                                  | Atomic file replacement |

## Functional subsystems

| Subsystem                             | Status | Owns | On disk (`~/.lnn/`) |
|---------------------------------------|--------|------|---------------------|
| [Requests](#requests-subsystem)       | Most developed. Receiving works up to the durable acknowledgment; sending goes straight from IPC; resolution (accept / reject) is in progress and not wired yet | `SYMLINK_REQUEST` handling, the requests queue, write source `RequestsID`, the requests WAL and cache | `requests`, `requests.cache` |
| [Outbox](#outbox-subsystem)           | Implemented and wired to incoming `OK`, `ERROR` and `RECOVER_REQUEST`; IPC does not send through it yet | Frames sent to each peer until the peer answers them, and the per-peer tag counters | `outbox` |
| [Recovery](#recovery-subsystem)       | Implemented, for requests only | Requests owed by peers after a crash damaged the requests WAL | `requests.recovery` |
| File transfers                        | Planned: only a placeholder queue (`FilesQueue`) and write source (`FilesID`) exist | — | — |

Further domains are planned but have no code yet: link creation and the persistent link database, peer management and trust, filesystem presentation. See [roadmap.md](roadmap.md).

---

## Internals

### Package map

```
cmd/lnnd/           Entry point
configs/            Default lnnd.conf, embedded and written to ~/.lnn/ if missing
internal/
  services.go       Services struct (package app): what the daemon shares between subsystems

  Shared infrastructure
  daemon/           Orchestrator: builds the shared services and the subsystems, starts them
  config/           Env file loader, network config, data dir, memory limit parsing, disk device
  logger/           Colored loggers (lnnd / lnnp prefixes)
  identity/         Node identity: UUID + Ed25519 keypair (~/.lnn/)
  protocol/         Wire format: encoding, parsing, constants
  network/          Transport abstraction (TCP), listener, framing, handshake, message dispatch
  peer/             Session registry, session lifecycle states, pending-message tracking
  ipc/              Unix socket listener, JSON request parsing, validation, routing
  sched/            Write scheduler: disk-load monitor, write turns, backpressure; request operations
  queues/           Queues held by the scheduler, one type per subsystem (requests; files: placeholder)
  fsutil/           Atomic file replacement (temp file + fsync + rename)

  Functional subsystems
  store/            Persistence domains and their writers; only requests today (WAL, cache, repair)
  outbox/           Outbox: sent frames kept per peer until answered; per-peer tag counter
  recovery/         Recovery: requests owed by peers after a crash
```

`sched`, `queues`, `network` and `ipc` are shared, but each holds the parts of a subsystem that plug into it: the request operations in `sched`, `RequestsQueue` in `queues`, the per-message handlers in `network`, the `symreq` operation in `ipc`.

### Services and import cycles

`internal/services.go` (package `app`) defines the `Services` struct that bundles everything the daemon shares between subsystems: the shared services (`Identity`, `Session`, `Sched`) and the subsystems other components call into (`Writers` — the persistence writers, only the requests writer today — `Outbox` and `Recovery`):

```go
type Services struct {
    Identity *identity.Service
    Session  *peer.SessionManager
    Sched    *sched.Service
    Writers  *store.Writers
    Outbox   *outbox.Outbox
    Recovery *recovery.Pending
}
```

`daemon.Daemon` embeds it. `recovery` is its own package so that `network` (which resolves entries) and `store` (which adds them) do not import each other. `ipc` consumes `app.Services` without importing `daemon` — `daemon` already imports `ipc` for `Start()`, so importing it back would create a cycle.

### Startup sequence

`daemon.New` builds the shared services first, then the subsystems that depend on them; `daemon.Start` launches the long-running loops.

```
main()
 └─ config.EnsureConfig(path, configs.Default)  create ~/.lnn/lnnd.conf if missing
 └─ config.LoadEnv("~/.lnn/lnnd.conf")          load env vars
 └─ daemon.New()
     ├─ identity.NewService()                   load/generate UUID + Ed25519 keys
     ├─ peer.NewSessionManager()                empty session registry
     ├─ config.DiskDevice()                     pick the disk to monitor
     ├─ sched.NewService(device)                scheduler, its queues, writer config per source
     ├─ outbox.Open()                           outbox: replay + compact ~/.lnn/outbox
     ├─ recovery.Open()                         recovery: load pending recoveries
     └─ store.NewRequestsWriter()               requests: open requests WAL + requests cache
 └─ daemon.Start()
     ├─ go Sched.Run()                          disk-load sampler + write-turn dispatch loop
     ├─ go Writers.Run(Sched)                   requests writer: startup repair, then batch-persist loop
     ├─ go server.Listen()                      TCP listener (network.Server)
     └─ ipc.ListenIPC()                         Unix socket server (blocks main)
```

### On-disk state

Everything lnnd persists lives in `~/.lnn/` (directory mode `0700`, since it holds private key material). Each file belongs to one component:

| File                | Owner         | Content |
|---------------------|---------------|---------|
| `lnnd.conf`         | configuration | Daemon configuration ([Configuration](#configuration)) |
| `uuid`              | identity      | Node UUID (plain text, mode 0600) |
| `id_ed25519`        | identity      | Ed25519 private key (raw bytes, mode 0600) |
| `id_ed25519.pub`    | identity      | Ed25519 public key (base64, mode 0644) |
| `requests`          | requests      | Write-ahead log of inbound requests (mode 0600) |
| `requests.cache`    | requests      | Local request id → wire tag map |
| `requests.recovery` | recovery      | Requests owed by peers after a crash |
| `outbox`            | outbox        | Frames sent and not yet answered, per peer, and the per-peer tag counters |

---

## Write scheduler

Shared infrastructure. `internal/sched` arbitrates the disk writes of the subsystems and protects the daemon under load. Each workload source has a `SourceID`, a weight and an expected write size, and may have an in-memory queue that the scheduler creates and holds. The operations that put work into a queue and take it out belong to the subsystem that owns the queue; for requests, see [Requests queue](#requests-queue).

**Write turns.** A writer calls `WaitWriteTurn(source)` before touching the disk and `ReleaseWriteTurn` afterwards. The scheduler grants turns up to a limit that depends on current disk load:

| Disk load state | Entered when                                  | Max concurrent writers | Selection order        |
|-----------------|-----------------------------------------------|------------------------|------------------------|
| `Normal`        | smoothed utilization < 65 % for ≥ 2 s         | 8                      | FIFO                   |
| `Elevated`      | between the thresholds (hysteresis band)      | 4                      | highest weight first   |
| `Saturated`     | smoothed utilization > 90 % for ≥ 2 s         | 2                      | highest weight first   |

**Disk monitor.** Utilization is sampled about once per second from the device's I/O busy time, smoothed with an exponential moving average (α = 0.2). The monitored device comes from `LNND_DISK_DEVICE`; if unset, lnnd uses the block device holding its own executable, falling back to the one holding `~/.lnn` (e.g. when started with `go run`, which builds into a tmpfs). The device is resolved through `/proc/self/mountinfo`, so btrfs subvolumes and LUKS/LVM volumes map to their kernel name (`dm-0`). If no device can be found, disk monitoring is disabled and the disk load stays `Normal`. Every state change wakes the scheduler so limits are re-applied immediately.

**Sources and weights.** Each write source has a weight; heavier sources are served first when the disk is under pressure.

| Source            | Weight | Expected write size | Used by |
|-------------------|--------|---------------------|---------|
| `FilesID`         | 4.0    | 1 MiB               | Reserved for file transfers (planned) |
| `RequestsID`      | 2.0    | 16 KiB              | Requests WAL writer |
| `RequestsCacheID` | 1.0    | 4 KiB               | Reserved: the requests cache writes without a turn today |

**Write-time baselines.** `ReleaseWriteTurn(w, writtenBytes)` records how long the turn took, normalized to the source's expected write size: a recent write time (EMA, α = 0.1) and a typical write time (EMA, α = 0.01) that is only updated while the disk load is `Normal`. The expected write size itself follows the sizes actually written (EMA, α = 0.0025). The baselines are collected for adaptive tuning, which is not implemented yet.

**Backpressure.** Backpressure is tracked per queue: a pressure flag is raised when the queue's queued bytes reach its `MaxQueuedBytes` (owned by `queues.QueueConfig`) and cleared when they drop below it. `UpdateQueuePressure` takes signed deltas, so enqueue and dequeue paths report their own sizes, and `IsBackPressured(source)` looks the flag up through the source's queue. Only the requests queue is tracked today; its limit is `LNND_REQ_QUEUE_MAX_BYTES` (default ¼ of `LNND_MAX_MEM`).

**Queues.** `internal/queues` holds the in-memory hand-off between the components that produce work and the writers that persist it. `sched.NewService` creates one queue per source that needs one, and each queue type belongs to one subsystem:

- **`RequestsQueue`** — the requests subsystem, see [Requests queue](#requests-queue).
- **`FilesQueue`** — placeholder for file transfers.

---

## Requests subsystem

Functional subsystem. Requests are link requests exchanged between peers: `SYMLINK_REQUEST` today, `HARDLINK_REQUEST` reserved. Receiving them is the most developed part of lnnd: an inbound request is queued, persisted to the requests WAL and acknowledged once it is durable. Sending is a direct `SYMLINK_REQUEST` from IPC ([ipc.md](ipc.md)); resolution (accept / reject, `RESOLUTION`) is in progress.

The subsystem spans the layers it uses:

- `network` — the `SYMLINK_REQUEST` handler on the read loop;
- `sched` — the request operations `SubmitRequest`, `NextRequest` and `TryNextRequest`, and write source `RequestsID`;
- `queues` — `RequestsQueue` and `PendingRequest`;
- `store` — the requests writer, the requests WAL and cache, and their startup repair;
- `ipc` — the `symreq` operation that sends a request to a peer.

Requests lost in a crash are brought back by the [recovery subsystem](#recovery-subsystem).

### Inbound request lifecycle

When an authenticated peer sends a `SYMLINK_REQUEST`:

```
 peer ── SYMLINK_REQUEST (tag) ──► network.HandleConn
                                      │  ReadFrame → HandleTCPMessage
                                      ▼
                              sched.SubmitRequest
                                1. wait for backpressure release   (≤ 2 s)
                                2. enqueue in RequestsQueue        (≤ 100 ms)
                                3. wait for durable write          (≤ 10 s)
                                      │
                                      ▼
                              store.RequestsWriter  (batches of ≤ 64)
                                 WAL append → fsync → checkpoint → fsync
                                      │
 peer ◄──────────── OK (tag) ─────────┘
```

- The reply is sent only after the request is **durably persisted**. `OK` means "stored and pending", not "accepted"; the accept/reject decision is meant to arrive later as a `RESOLUTION`, which is not wired yet.
- If the requests queue is full, or stays back-pressured for 2 s, the peer receives `ERROR(ErrOverloaded)`; any other failure produces `ERROR(ErrInternal)`. The reply carries the same tag as the request.
- Requests are handled in a goroutine per request, so a slow disk never blocks the connection's read loop.

### Requests queue

- **`RequestsQueue`** — bounded channel of `PendingRequest`. Capacity comes from `LNND_REQ_QUEUE_SIZE` (default 4096). Supports non-blocking and timed enqueue, and blocking / non-blocking dequeue.
- **`PendingRequest`** — `{From, Req, Written, EnqueuedAt, Tag}`. It is transient: it exists only until the writer reports the persistence result on `Written`. The on-disk file is the source of truth afterwards.
- **Scheduler operations.** `SubmitRequest` waits up to 2 s for the requests queue's backpressure to clear (and otherwise fails with `ErrOverloaded`), enqueues the request, adds its size to the queue's queued bytes and waits for the writer's result; `NextRequest` / `TryNextRequest` hand queued requests to the writer and subtract their size.
- **Timeouts** (`sched/config.go`): enqueue 100 ms, write confirmation 10 s, pressure wait 2 s.

### Requests WAL (`requests`)

```
┌──────────────── file header (16 B) ────────────────┐
│ magic "LNNR" (4) │ lco (8) │ current_req_id (4)    │
├────────────────────────────────────────────────────┤
│ initial checkpoint                                 │
│ entry … entry │ checkpoint │ entry … │ checkpoint  │
└────────────────────────────────────────────────────┘
```

- `lco` — *last checkpoint offset*, the offset of the most recent checkpoint entry.
- `current_req_id` — persisted counter used to assign local request ids.

Every entry starts with `[entry_length:4][op:1]`:

| Op | Name          | Payload                                                                         |
|----|---------------|---------------------------------------------------------------------------------|
| 0  | `REQUEST`     | `[req_type:2][peer_uuid:16][req_id:4][unix_ts:8][src_len:2][src][dst_len:2][dst]` (`dst` part only if set) |
| 1  | `ACCEPTED`    | `[req_id:4]`                                                                    |
| 2  | `REJECTED`    | `[req_id:4]`                                                                    |
| 3  | `CHECKPOINT`  | `[checksum:4]` — byte-sum of all entries since the previous checkpoint          |

**Write path** (`persistRequests`):

1. Assign a local `req_id` to every request in the batch.
2. Record `req_id → tag` in the requests cache (in the background).
3. Serialize the batch into a buffer.
4. Wait for a write turn from the scheduler.
5. Flush + `fsync` the entries.
6. Append a checkpoint covering the batch; flush + `fsync`.
7. Update `lco` in the header; `fsync`.
8. Report the result on each request's `Written` channel.

### Requests cache (`requests.cache`)

Fixed 8-byte slots: `[req_id:4][tag:4]`. It maps each local request id to the tag of the wire message that carried it.

The tag is what lets the daemon re-establish a conversation after a crash: if a request entry survives but the exchange with the peer was interrupted, the daemon sends a `RECOVER_REQUEST` carrying that tag so the peer knows which request is meant. The peer's own request id (`rmt_req_id`) is deliberately not used for recovery — after a crash there is no reliable way to know whether it is consistent, up to date, or even exists. `rmt_req_id` is only used to tell the peer whether a request was accepted or rejected.

### Startup repair

Before the requests writer starts its loop, it validates and repairs the requests WAL:

1. **Header repair** (`OpenRequestsFile`): a wrong magic is rewritten; an invalid `lco` is recomputed by scanning for the last valid checkpoint; if neither fixes the header, the file is reset.
2. **Validation** (`Run` → `checkEntries`): the daemon validates either the tail written after the last checkpoint (entry framing) or, if the file ends at a checkpoint, the region that checkpoint closes (framing + checksum).
3. **Outcome:**
   - *Valid* — nothing to do.
   - *Damaged but recoverable* (a request entry, possibly cut short at the end of the file, still has its peer UUID and request id) — each request's tag is looked up in the cache and the `(peer, req_id, tag)` triple is handed to the [recovery subsystem](#recovery-subsystem), which persists it in `requests.recovery`. Then the damaged region is truncated as below. See [protocol.md](protocol.md#recovery) for how entries are resolved.
   - *Damaged* — `removeTail` truncates everything after the last valid checkpoint; if that checkpoint is itself corrupt, the file is rolled back to the previous one.

   A file that only holds the initial checkpoint is valid.

---

## Outbox subsystem

Functional subsystem, package `outbox`, file `~/.lnn/outbox`. It is the sender side of crash recovery: every frame put in the outbox is kept per peer, keyed by its tag, until the peer answers it with `OK` or `ERROR`, and the outbox hands out the per-peer tags. The network read loop removes a frame when its answer arrives and resends it when the peer asks for it with `RECOVER_REQUEST`. The window holds 256 frames per peer.

On disk the outbox is an append-only log of CRC-checked records. It is replayed and compacted at startup and compacted again whenever it grows past 1 MiB. On every start the tag counters jump ahead, so a tag that reached a peer but not the disk before a crash is never handed out again.

Not wired yet: IPC sends its requests directly instead of through the outbox. Protocol-level behaviour: [protocol.md](protocol.md#recovery).

## Recovery subsystem

Functional subsystem, package `recovery`, file `~/.lnn/requests.recovery`. It is the receiver side of crash recovery and currently serves only the requests subsystem: each entry is a request whose WAL entry was damaged by a crash but still carries its peer UUID and request id, stored with the tag found in the requests cache during [startup repair](#startup-repair). It is its own package so that `store`, which adds entries, and `network`, which settles them, do not import each other.

After each inbound handshake the network layer sends one `RECOVER_REQUEST` per entry of that peer. An entry is settled once the peer's resend is durable again, or when the peer answers `ErrUnknownTag`. The file is rewritten atomically on every change, so a second crash cannot forget what peers still owe.

Not wired yet: outbound sessions (opened from IPC) do not send pending `RECOVER_REQUEST`s. Protocol-level behaviour: [protocol.md](protocol.md#recovery).

---

## Identity storage

Shared infrastructure. The node identity — `uuid`, `id_ed25519` and `id_ed25519.pub`, listed under [On-disk state](#on-disk-state) — is generated automatically on first run if missing. `~/.lnn/` is created with mode `0700` because it holds the private key.

---

## Configuration

Shared infrastructure. `~/.lnn/lnnd.conf` is loaded at startup. Each `KEY=VALUE` line is exported as an environment variable; lines without `=` are ignored. Some settings belong to a single subsystem:

| Variable              | Purpose                                                              | Default | Used by |
|-----------------------|----------------------------------------------------------------------|---------|---------|
| `TCP_PORT`            | Port for the TCP peer listener                                       | —       | network |
| `LNND_IPC_SOCKET`     | Path to the Unix IPC socket                                          | —       | IPC |
| `LNND_REQ_QUEUE_SIZE` | Capacity of the inbound requests queue (1–65534)                     | `4096`  | requests |
| `LNND_MAX_MEM`        | Memory limit; plain bytes or with `KB` / `MB` / `GB` suffix          | `64MB`  | today only the default of `LNND_REQ_QUEUE_MAX_BYTES` |
| `LNND_REQ_QUEUE_MAX_BYTES` | Requests queue byte limit before backpressure; same format as `LNND_MAX_MEM` | ¼ of `LNND_MAX_MEM` | requests |
| `LNND_DISK_DEVICE`    | Block device monitored by the scheduler                              | device holding the executable, else `~/.lnn` | write scheduler |
| `QUIC_PORT`           | Reserved for the QUIC transport (not used yet)                       | —       | network |
