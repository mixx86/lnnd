# lnnd architecture

lnnd is the background daemon of lnn. It owns all network I/O, peer sessions, the durable storage of incoming content and link state. Users never talk to it directly: the [`lnn`](https://github.com/mixx86/lnn) CLI sends it requests over a local Unix socket.

Related documents: [protocol.md](protocol.md) (LNNP wire protocol), [ipc.md](ipc.md) (CLI ↔ daemon contract), [trust-model.md](trust-model.md) and [filesystem.md](filesystem.md) (planned features).

```
 User
  │
  ▼
┌────────┐   Unix socket (JSON)   ┌─────────────────────────────────────────────┐
│  lnn   │ ─────────────────────► │                    lnnd                     │
│ (CLI)  │                        │                                             │
└────────┘                        │  ┌──────────┐        ┌───────────────────┐  │
                                  │  │   IPC    │        │  Network listener │  │
                                  │  │ listener │        │  (TCP, LNNP)      │  │
                                  │  └────┬─────┘        └─────────┬─────────┘  │
                                  │       │ outbound               │ inbound    │
                                  │       ▼                        ▼            │
                                  │  ┌─────────────────────────────────────┐    │
                                  │  │ Sessions · Scheduler · Queues       │    │
                                  │  └──────────────────┬──────────────────┘    │
                                  │                     ▼                       │
                                  │  ┌─────────────────────────────────────┐    │
                                  │  │ Storage: WAL · outbox · recovery    │    │
                                  │  └─────────────────────────────────────┘    │
                                  └─────────────────────────┬───────────────────┘
                                                            │ TCP
                                                            ▼
                                                      Remote peers
```

lnnd is layered. The listeners accept work from the CLI and from peers; peer sessions, the write scheduler and its queues decide when that work reaches the disk; storage keeps what must survive a crash. `daemon` builds the layers and wires them together.

---

## Internals

### Package map

```
cmd/lnnd/           Entry point
configs/            Default lnnd.conf, embedded and written to ~/.lnn/ if missing
internal/
  services.go       Services struct (package app) — the daemon's shared services
  config/           Env file loader, network config, data dir, memory limits, disk device
  daemon/           Orchestrator — builds all services and starts them
  fsutil/           Atomic file replacement (temp file + fsync + rename)
  identity/         Node identity: UUID + Ed25519 keypair (~/.lnn/)
  ipc/              Unix socket listener, JSON request parsing, validation, routing
  logger/           Colored loggers (lnnd / lnnp prefixes)
  network/          Transport abstraction (TCP), listener, framing, handshake, message dispatch
  outbox/           Sent messages kept per peer until answered; per-peer tag counter
  peer/             Session registry, session lifecycle states, pending-message tracking
  protocol/         Wire format: encoding, parsing, constants
  queues/           Bounded in-memory queues for incoming work waiting to be persisted
  recovery/         What peers still owe after a crash damaged the log
  sched/            Write scheduler: disk-load monitor, write turns, backpressure
  store/            Durable storage of incoming content: write-ahead log, tag cache, startup repair
```

### Services and import cycles

`internal/services.go` (package `app`) defines the `Services` struct that bundles everything the daemon shares between its components:

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

`daemon.Daemon` embeds it. `recovery` is its own package so that `network` (which resolves entries) and `store` (which adds them) do not import each other. `ipc` consumes `app.Services`, plus the `network.Server` it opens outbound sessions with, without importing `daemon` — `daemon` already imports `ipc` for `Start()`, so importing it back would create a cycle.

### Startup sequence

```
main()
 └─ config.EnsureConfig(path, configs.Default)  create ~/.lnn/lnnd.conf if missing
 └─ config.LoadEnv("~/.lnn/lnnd.conf")          load env vars
 └─ daemon.New()
     ├─ identity.NewService()                   load/generate UUID + Ed25519 keys
     ├─ peer.NewSessionManager()                empty session registry
     ├─ config.DiskDevice()                     pick the disk to monitor
     ├─ sched.NewService(device)                scheduler, queues, writer config
     ├─ outbox.Open()                           replay + compact ~/.lnn/outbox
     ├─ recovery.Open()                         load pending recoveries
     └─ store.NewRequestsWriter()               open the write-ahead log and its cache
 └─ daemon.Start()
     ├─ go Sched.Run()                          disk-load sampler + write-turn dispatch loop
     ├─ go Writers.Run(Sched)                   startup recovery, then batch-persist loop
     ├─ go server.Listen()                      TCP listener (network.Server)
     └─ ipc.Listen()                            Unix socket server (blocks main)
```

### On-disk state

Everything lnnd persists lives in `~/.lnn/` (directory mode `0700`, since it holds private key material):

| File                | Owner      | Content |
|---------------------|------------|---------|
| `lnnd.conf`         | `config`   | Daemon configuration ([Configuration](#configuration)) |
| `uuid`              | `identity` | Node UUID (plain text, mode 0600) |
| `id_ed25519`        | `identity` | Ed25519 private key (raw bytes, mode 0600) |
| `id_ed25519.pub`    | `identity` | Ed25519 public key (base64, mode 0644) |
| `requests`          | `store`    | Write-ahead log of incoming requests (mode 0600) |
| `requests.cache`    | `store`    | Local request id → wire tag map |
| `requests.recovery` | `recovery` | What peers still owe after a crash |
| `outbox`            | `outbox`   | Messages sent and not yet answered, per peer, and the per-peer tag counters |

### Inbound request lifecycle

When an authenticated peer sends a request (`SYMLINK_REQUEST`, for example):

```
 peer ── request (tag) ──────────► network.HandleConn
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

### Scheduler

`internal/sched` arbitrates disk writes and protects the daemon under load. Every kind of work written to disk has a write source with its own weight and expected write size, and may have an in-memory queue that the scheduler creates and holds (see [Queues](#queues)).

**Write turns.** A writer calls `WaitWriteTurn(source)` before touching the disk and `ReleaseWriteTurn` afterwards. The scheduler grants turns up to a limit that depends on current disk load:

| Disk load state | Entered when                                  | Max concurrent writers | Selection order        |
|-----------------|-----------------------------------------------|------------------------|------------------------|
| `Normal`        | smoothed utilization < 65 % for ≥ 2 s         | 8                      | FIFO                   |
| `Elevated`      | between the thresholds (hysteresis band)      | 4                      | highest weight first   |
| `Saturated`     | smoothed utilization > 90 % for ≥ 2 s         | 2                      | highest weight first   |

**Disk monitor.** Utilization is sampled about once per second from the device's I/O busy time, smoothed with an exponential moving average (α = 0.2). The monitored device comes from `LNND_DISK_DEVICE`; if unset, lnnd uses the block device holding its own executable, falling back to the one holding `~/.lnn` (e.g. when started with `go run`, which builds into a tmpfs). The device is resolved through `/proc/self/mountinfo`, so btrfs subvolumes and LUKS/LVM volumes map to their kernel name (`dm-0`). If no device can be found, disk monitoring is disabled and the disk load stays `Normal`. Every state change wakes the scheduler so limits are re-applied immediately.

**Sources and weights.** Each source has a weight; heavier sources are served first when the disk is under pressure.

| Source            | Weight | Expected write size | Used by |
|-------------------|--------|---------------------|---------|
| `FilesID`         | 4.0    | 1 MiB               | Reserved for file transfers (planned) |
| `RequestsID`      | 2.0    | 16 KiB              | Write-ahead log writer |
| `RequestsCacheID` | 1.0    | 4 KiB               | Reserved: the tag cache is written without a turn today |

**Write-time baselines.** `ReleaseWriteTurn(w, writtenBytes)` records how long the turn took, normalized to the source's expected write size: a recent write time (EMA, α = 0.1) and a typical write time (EMA, α = 0.01) that is only updated while the disk load is `Normal`. The expected write size itself follows the sizes actually written (EMA, α = 0.0025). After each sample, a source's weight becomes its base weight times its overrun (recent / typical write time), never below the base weight and at most twice it, so under `Elevated` or `Saturated` load the source falling behind its usual pace is served first. Per-source budgets and tickets are planned.

**Backpressure.** Each queue can have a pressure flag, raised when its queued bytes reach its `MaxQueuedBytes` (owned by `queues.QueueConfig`) and cleared when they drop below it. `UpdateQueuePressure` takes signed deltas, so enqueue and dequeue paths report their own sizes, and `IsBackPressured(source)` looks the flag up through the source's queue. Only the requests queue is tracked today; its limit is `LNND_REQ_QUEUE_MAX_BYTES` (default ¼ of `LNND_MAX_MEM`).

### Queues

`internal/queues` holds the in-memory hand-off between the components that receive work and the writers that persist it.

- **`RequestsQueue`** — bounded channel of `PendingRequest`, for the requests to log: those peers send and those lnnd sends. Capacity comes from `LNND_REQ_QUEUE_SIZE` (default 4096). Supports non-blocking and timed enqueue, and blocking / non-blocking dequeue.
- **`PendingRequest`** — `{Peer, Req, Written, EnqueuedAt, Tag, RMTReqID, Sent}`. It is transient: it exists only until the writer reports the persistence result, with the `req_id` it assigned, on `Written`. The on-disk file is the source of truth afterwards.
- **`FilesQueue`** — placeholder for file transfers.

The scheduler puts work in and takes it out: `SubmitRequest` waits up to 2 s for the queue's backpressure to clear (and otherwise fails with `ErrOverloaded`), enqueues the request, adds its size to the queue's queued bytes and waits for the writer's result; `SubmitSentRequest` does the same for a request of ours and returns its `req_id`; `NextRequest` / `TryNextRequest` hand queued requests to the writer and subtract their size.

**Timeouts** (`sched/config.go`): enqueue 100 ms, write confirmation 10 s, pressure wait 2 s.

### Storage

`internal/store` persists the content that must survive a crash, before it is acknowledged or sent. Its writers are grouped in `store.Writers`, one per log; the only log today holds requests, both those peers send and those lnnd sends. The outbox and the list of pending recoveries are persisted by their own packages.

#### Requests WAL (`requests`)

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
| 0  | `REQUEST`     | `[req_type:2][peer_uuid:16][req_id:4][rmt_req_id:4][unix_ts:8][payload]` — a request a peer sent; `rmt_req_id` is the peer's id for it; `payload` is the request's wire payload as is, decoded according to `req_type` (for example, the [`SYMLINK_REQUEST` payload](protocol.md#message-types)) |
| 1  | `ACCEPTED`    | `[req_id:4]`                                                                    |
| 2  | `REJECTED`    | `[req_id:4]`                                                                    |
| 3  | `CHECKPOINT`  | `[checksum:4]` — byte-sum of all entries since the previous checkpoint          |
| 4  | `SENT_REQUEST`| Same as `REQUEST`, for a request lnnd sends: `peer_uuid` is the peer it goes to and `rmt_req_id` is `0`. It is logged before it is sent |

The direction is in the op rather than in `rmt_req_id`, so recovery can tell it from the entry header alone, even for an entry cut short. In `REQUEST` entries `rmt_req_id` is written as `0` for now: the wire does not carry the peer's id yet.

**Write path** (`persistRequests`):

1. Assign a local `req_id` to every request in the batch.
2. Record `req_id → tag` in the requests cache for the requests peers sent (in the background); a sent request's tag belongs to the outbox.
3. Serialize the batch into a buffer.
4. Wait for a write turn from the scheduler.
5. Flush + `fsync` the entries.
6. Append a checkpoint covering the batch; flush + `fsync`.
7. Update `lco` in the header; `fsync`.
8. Report the result, with the assigned `req_id`, on each request's `Written` channel.

#### Requests cache (`requests.cache`)

Fixed 8-byte slots: `[req_id:4][tag:4]`. It maps each local request id to the tag of the wire message that carried it.

The tag is what lets the daemon re-establish a conversation after a crash: if a request entry survives but the exchange with the peer was interrupted, the daemon sends a `RECOVER_REQUEST` carrying that tag so the peer knows which request is meant. The peer's own request id (`rmt_req_id`) is deliberately not used for recovery — after a crash there is no reliable way to know whether it is consistent, up to date, or even exists. `rmt_req_id` is only used to tell the peer whether a request was accepted or rejected.

#### Startup recovery

1. **Header repair** (`OpenRequestsFile`): a wrong magic is rewritten; an invalid `lco` is recomputed by scanning for the last valid checkpoint; if neither fixes the header, the file is reset.
2. **Validation** (`Run` → `checkEntries`): the daemon validates either the tail written after the last checkpoint (entry framing) or, if the file ends at a checkpoint, the region that checkpoint closes (framing + checksum).
3. **Outcome:**
   - *Valid* — nothing to do.
   - *Damaged but recoverable* (a `REQUEST` entry, possibly cut short at the end of the file, still has its peer UUID and request id; a `SENT_REQUEST` never is, since there is nothing to ask the peer for) — each request's tag is looked up in the cache and the `(peer, req_id, tag)` triple is added to the [pending recoveries](#pending-recoveries). Then the damaged region is truncated as below. See [protocol.md](protocol.md#recovery) for how entries are resolved.
   - *Damaged* — `removeTail` truncates everything after the last valid checkpoint; if that checkpoint is itself corrupt, the file is rolled back to the previous one.

   A file that only holds the initial checkpoint is valid.

#### Outbox

Package `outbox`, file `~/.lnn/outbox`. Every message put in the outbox is kept per peer, keyed by its tag, until the peer answers it with `OK` or `ERROR`, and the outbox hands out the per-peer tags. The network read loop removes a message when its answer arrives and sends it again when the peer asks for it with `RECOVER_REQUEST`. The window holds 256 messages per peer.

On disk the outbox is an append-only log of CRC-checked records. It is replayed and compacted at startup and compacted again whenever it grows past 1 MiB. On every start the tag counters jump ahead, so a tag that reached a peer but not the disk before a crash is never handed out again.

IPC puts its requests in the outbox and sends them over the peer's session, opening one with `network.Server.Connect` when there is none. Outbound sessions run the same read loop as inbound ones, so their answers reach the outbox too. Protocol-level behaviour: [protocol.md](protocol.md#recovery).

#### Pending recoveries

Package `recovery`, file `~/.lnn/requests.recovery`. It lists what peers still owe after a crash: log entries that were damaged but still name their peer and request id, each with the tag found in the requests cache during [startup recovery](#startup-recovery). It is its own package so that `store`, which adds entries, and `network`, which settles them, do not import each other.

After each handshake, inbound or outbound, the network layer sends one `RECOVER_REQUEST` per entry of that peer. An entry is settled once the peer's resend is durable again, or when the peer answers `ErrUnknownTag`. The file is rewritten atomically on every change, so a second crash cannot forget what peers still owe.

Protocol-level behaviour: [protocol.md](protocol.md#recovery).

---

## Identity storage

The node identity — `uuid`, `id_ed25519` and `id_ed25519.pub`, listed under [On-disk state](#on-disk-state) — is generated automatically on first run if missing. `~/.lnn/` is created with mode `0700` because it holds the private key.

---

## Configuration

`~/.lnn/lnnd.conf` is loaded at startup. Each `KEY=VALUE` line is exported as an environment variable; lines without `=` are ignored.

| Variable              | Purpose                                                              | Default |
|-----------------------|----------------------------------------------------------------------|---------|
| `TCP_PORT`            | Port for the TCP peer listener                                       | —       |
| `LNND_IPC_SOCKET`     | Path to the Unix IPC socket                                          | —       |
| `LNND_REQ_QUEUE_SIZE` | Capacity of the inbound requests queue (1–65534)                     | `4096`  |
| `LNND_MAX_MEM`        | Memory limit; plain bytes or with `KB` / `MB` / `GB` suffix          | `64MB`  |
| `LNND_REQ_QUEUE_MAX_BYTES` | Requests queue byte limit before backpressure; same format as `LNND_MAX_MEM` | ¼ of `LNND_MAX_MEM` |
| `LNND_DISK_DEVICE`    | Block device monitored by the scheduler                              | device holding the executable, else `~/.lnn` |
| `QUIC_PORT`           | Reserved for the QUIC transport (not used yet)                       | —       |
