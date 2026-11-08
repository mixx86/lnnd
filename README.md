# lnnd

**lnnd** is the background daemon of [lnn](https://github.com/mixx86/lnn) (**LiNk + Networks**): it links files across machines, peer to peer, over authenticated connections speaking **LNNP** (the *LiNk + Networks Protocol*).

Users interact with lnn through the [`lnn`](https://github.com/mixx86/lnn) CLI; the CLI sends its requests to lnnd over a local Unix socket.

> Work in progress — core protocol, handshake and the durable inbound request pipeline are implemented; link operations are in development.

## How it works

lnnd is made of functional subsystems, each owning one domain, built on a common core.

**Core**

- Listens on a **Unix socket** for requests from the local `lnn` CLI.
- Handles all **TCP** communication with remote peers, authenticating each one via an **Ed25519 mutual handshake** before exchanging any data, and hands each message to the subsystem that owns it.
- Runs a **write scheduler** that arbitrates disk writes between subsystems, monitors disk load and applies backpressure to peers when the daemon is overloaded.

**Functional subsystems**

- **Requests** — the most developed one: persists incoming link requests to an on-disk **write-ahead log** before acknowledging them, so they survive crashes and restarts.
- **Outbox** — keeps the frames sent to each peer until the peer answers them, so they can be sent again after a crash (IPC does not send through it yet).
- **Recovery** — after a crash, asks peers to resend the requests whose log entries were damaged.
- More are planned; see the [roadmap](docs/roadmap.md).

See [docs/architecture.md](docs/architecture.md) for the full breakdown.

## Build

**Requirements:** Go 1.26+

```bash
go build -o lnnd ./cmd/lnnd
```

## Configuration

`lnnd` reads `~/.lnn/lnnd.conf` at startup. The file uses `KEY=VALUE` lines; each pair is exported as an environment variable. Lines without `=` are ignored. If the file does not exist, lnnd creates it from [`configs/lnnd.conf`](configs/lnnd.conf), which is embedded in the binary.

```
TCP_PORT=7878
LNND_IPC_SOCKET=/tmp/lnnd.sock
```

| Variable              | Purpose                                                          | Default |
|-----------------------|------------------------------------------------------------------|---------|
| `TCP_PORT`            | Port for the TCP peer listener                                   | —       |
| `LNND_IPC_SOCKET`     | Path to the Unix IPC socket                                      | —       |
| `LNND_REQ_QUEUE_SIZE` | Capacity of the inbound requests queue (1–65534)                 | `4096`  |
| `LNND_MAX_MEM`        | Memory limit; plain bytes or with a `KB`/`MB`/`GB` suffix        | `64MB`  |
| `LNND_REQ_QUEUE_MAX_BYTES` | Bytes queued in the requests queue before backpressure; same format as `LNND_MAX_MEM` | ¼ of `LNND_MAX_MEM` |
| `LNND_DISK_DEVICE`    | Block device whose load drives the write scheduler (`nvme0n1`, `/dev/sda`, …) | device holding the `lnnd` executable, else `~/.lnn` |
| `QUIC_PORT`           | Reserved for the upcoming QUIC transport (not used yet)          | —       |

## Running

```bash
lnnd    # runs the daemon in the foreground
```

Runtime state lives in `~/.lnn/`: the node identity (UUID + Ed25519 keys, generated on first run) and the files each subsystem persists. See [On-disk state](docs/architecture.md#on-disk-state) and [Identity storage](docs/architecture.md#identity-storage).

## Documentation

| Document                                   | Contents |
|--------------------------------------------|----------|
| [docs/architecture.md](docs/architecture.md) | Core and functional subsystems, package map, startup, on-disk state, write scheduler, each subsystem in detail, configuration |
| [docs/protocol.md](docs/protocol.md)         | LNNP wire format, message types, error codes, handshake, session states |
| [docs/ipc.md](docs/ipc.md)                   | The Unix-socket contract used by the CLI |
| [docs/trust-model.md](docs/trust-model.md)   | Peer permissions, templates, anti-flood rules, alerts *(planned)* |
| [docs/filesystem.md](docs/filesystem.md)     | How remote links are presented on the local filesystem *(planned)* |
| [docs/roadmap.md](docs/roadmap.md)           | Planned work, in phases |
| [docs/coding-style.md](docs/coding-style.md) | Line length, comments and naming rules |

## Status

| Feature                                      | Status      |
|----------------------------------------------|-------------|
| Wire protocol + tagged message frames        | Done        |
| Ed25519 mutual handshake                     | Done        |
| TCP message dispatch loop                    | Done        |
| IPC (CLI ↔ daemon)                           | Done        |
| Write scheduler (write turns, backpressure)  | In progress |
| Inbound requests queue                       | In progress |
| Durable request log (WAL) + crash recovery   | In progress |
| Session state machine                        | In progress |
| Remote symlinks                              | In progress |
| Remote hard links                            | In progress |
| Request resolution (accept / reject)         | In progress |
| Peer management + trust model                | Planned     |
| Persistent link database                     | Planned     |
| FUSE filesystem presentation                 | Planned     |
| QUIC transport                               | Planned     |
| TLS encryption                               | Planned     |
| Sync mode                                    | Planned     |
| Smart links                                  | Planned     |

## License

[AGPLv3](LICENSE)
