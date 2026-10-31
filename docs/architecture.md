# lnnd architecture

lnnd is the background daemon of lnn. It owns all network I/O, peer sessions, request persistence and link state. Users never talk to it directly: the [`lnn`](https://github.com/mixx86/lnn) CLI sends it requests over a local Unix socket.

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
                                  │  │ Store: requests WAL + cache         │    │
                                  │  └─────────────────────────────────────┘    │
                                  └─────────────────────────┬───────────────────┘
                                                            │ TCP
                                                            ▼
                                                      Remote peers
```

---

## Internals

### Package map

```
cmd/lnnd/           Entry point
configs/lnnd.conf   Example configuration (copy to ~/.lnn/lnnd.conf)
internal/
  services.go       Services struct (package app) — the daemon's shared services
  config/           Env file loader, network config, data dir, memory limit parsing
  daemon/           Orchestrator — builds all services and starts them
  identity/         Node identity: UUID + Ed25519 keypair (~/.lnn/)
  ipc/              Unix socket listener, JSON request parsing, validation, routing
  logger/           Colored loggers (lnnd / lnnp prefixes)
  network/          Transport abstraction (TCP), listener, framing, handshake, message dispatch
  peer/             Session registry, session lifecycle states, pending-message tracking
  protocol/         Wire format: encoding, parsing, constants
  queues/           Bounded in-memory queues for inbound work (requests; files is a placeholder)
  sched/            Write scheduler: disk-load monitor, writer turns, backpressure
  store/            Durable storage: requests WAL, requests cache, crash recovery
```

### Services and import cycles

`internal/services.go` (package `app`) defines the `Services` struct that bundles everything the daemon shares between subsystems:

```go
type Services struct {
    Identity *identity.Service
    Session  *peer.SessionManager
    Sched    *sched.Service
    Writers  *store.Writers
}
```

`daemon.Daemon` embeds it. `ipc` consumes `app.Services` without importing `daemon` — `daemon` already imports `ipc` for `Start()`, so importing it back would create a cycle.

### Startup sequence

```
main()
 └─ config.LoadEnv("~/.lnn/lnnd.conf")      load env vars
 └─ daemon.New()
     ├─ identity.NewService()               load/generate UUID + Ed25519 keys
     ├─ peer.NewSessionManager()            empty session registry
     ├─ config.DiskDevice()                 pick the disk to monitor
     ├─ sched.NewService(device)            scheduler, queues, writer config
     └─ store.NewRequestsWriter()           open requests WAL + requests cache
 └─ daemon.Start()
     ├─ go Sched.Run()                      disk-load sampler + writer dispatch loop
     ├─ go Writers.Run(Sched)               startup recovery, then batch-persist loop
     ├─ go network.ListenTCP()              TCP server (goroutine)
     └─ ipc.ListenIPC()                     Unix socket server (blocks main)
```

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

- The reply is sent only after the request is **durably persisted**. `OK` means "stored and pending", not "accepted"; the accept/reject decision arrives later as a `RESOLUTION`.
- If the queue is full the peer receives `ERROR(ErrOverloaded)`; any other failure produces `ERROR(ErrInternal)`. The reply carries the same tag as the request.
- Requests are handled in a goroutine per request, so a slow disk never blocks the connection's read loop.

### Scheduler

`internal/sched` arbitrates disk writes and protects the daemon under load.

**Write turns.** A writer calls `WaitWriteTurn(source)` before touching the disk and `ReleaseWriteTurn` afterwards. The scheduler grants turns up to a limit that depends on current disk load:

| Disk load state | Entered when                                  | Max concurrent writers | Selection order        |
|-----------------|-----------------------------------------------|------------------------|------------------------|
| `Normal`        | smoothed utilization < 65 % for ≥ 2 s         | 8                      | FIFO                   |
| `Elevated`      | between the thresholds (hysteresis band)      | 4                      | highest weight first   |
| `Saturated`     | smoothed utilization > 90 % for ≥ 2 s         | 2                      | highest weight first   |

**Disk monitor.** Utilization is sampled about once per second from the device's I/O busy time, smoothed with an exponential moving average (α = 0.2). The monitored device comes from `LNND_DISK_DEVICE`; if unset, lnnd uses the block device holding its own executable, falling back to the one holding `~/.lnn` (e.g. when started with `go run`, which builds into a tmpfs). The device is resolved through `/proc/self/mountinfo`, so btrfs subvolumes and LUKS/LVM volumes map to their kernel name (`dm-0`). If no device can be found, disk monitoring is disabled and the disk load stays `Normal`. Every state change wakes the scheduler so limits are re-applied immediately.

**Sources and weights.** Each write source has a weight; heavier sources are served first when the disk is under pressure.

| Source          | Weight | Expected write size |
|-----------------|--------|---------------------|
| `FilesID`       | 4.0    | 1 MiB               |
| `RequestsID`    | 2.0    | 16 KiB              |
| `RequestsCacheID` | 1.0  | 4 KiB               |

`RecordWriteResult` keeps a per-source baseline of write latency (normalized to the expected size, EMA with α = 0.1) for adaptive tuning.

**Backpressure.** Each queue has a pressure flag (`UpdateQueuePressure`), raised when its queued bytes reach the queue's `MaxQueuedBytes` (owned by `queues.QueueConfig`; for requests, `LNND_REQ_QUEUE_MAX_BYTES`, default ¼ of `LNND_MAX_MEM`). `SubmitRequest` adds each request's size after enqueueing and `NextRequest` / `TryNextRequest` subtract it after dequeueing. While a queue is pressured, `SubmitRequest` waits up to 2 s for it to clear and then fails with `ErrOverloaded`.

**Timeouts** (`sched/config.go`): enqueue 100 ms, write confirmation 10 s, pressure wait 2 s.

### Queues

`internal/queues` holds the in-memory hand-off between network handlers and writers.

- **`RequestsQueue`** — bounded channel of `PendingRequest`. Capacity comes from `LNND_REQ_QUEUE_SIZE` (default 4096). Supports non-blocking and timed enqueue, and blocking / non-blocking dequeue.
- **`PendingRequest`** — `{From, Req, Written, EnqueuedAt, Tag}`. It is transient: it exists only until the writer reports the persistence result on `Written`. The on-disk file is the source of truth afterwards.
- **`FilesQueue`** — placeholder for file transfers.

### Storage

`internal/store` persists inbound requests under `~/.lnn/`.

```
~/.lnn/
  requests          write-ahead log of inbound requests (mode 0600)
  requests.cache    local request id → wire tag map
```

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

#### Requests cache (`requests.cache`)

Fixed 6-byte slots: `[req_id:4][tag:2]`. It maps each local request id to the tag of the wire message that carried it.

The tag is what lets the daemon re-establish a conversation after a crash: if a request entry survives but the exchange with the peer was interrupted, the daemon sends a `RECOVER_REQUEST` carrying that tag so the peer knows which request is meant. The peer's own request id (`rmt_req_id`) is deliberately not used for recovery — after a crash there is no reliable way to know whether it is consistent, up to date, or even exists. `rmt_req_id` is only used to tell the peer whether a request was accepted or rejected.

#### Startup recovery

1. **Header repair** (`OpenRequestsFile`): a wrong magic is rewritten; an invalid `lco` is recomputed by scanning for the last valid checkpoint; if neither fixes the header, the file is reset.
2. **Validation** (`Run` → `checkEntries`): the daemon validates either the tail written after the last checkpoint (entry framing) or, if the file ends at a checkpoint, the region that checkpoint closes (framing + checksum).
3. **Outcome:**
   - *Valid* — nothing to do.
   - *Damaged but recoverable* (a request entry still has its peer UUID and request id) — the daemon looks up each request's tag in the cache and sends `RECOVER_REQUEST` to the peer if a session exists.
   - *Damaged and not recoverable* — `removeTail` truncates everything after the last valid checkpoint; if that checkpoint is itself corrupt, the file is rolled back to the previous one.

---

## Identity storage

Keys and UUID are persisted under `~/.lnn/` (directory mode `0700`, since it holds private key material):

```
~/.lnn/
  uuid              node UUID (plain text, mode 0600)
  id_ed25519        Ed25519 private key (raw bytes, mode 0600)
  id_ed25519.pub    Ed25519 public key (base64, mode 0644)
  lnnd.conf         daemon configuration (see Configuration)
```

Generated automatically on first run if missing. The request log files described under [Storage](#storage) live in the same directory.

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
