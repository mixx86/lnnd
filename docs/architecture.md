# Architecture

lnn is split into two binaries that communicate over a local Unix socket:

- **`lnn`** — CLI client written in C. Drop-in replacement for `ln`, extended with network syntax.
- **`lnnd`** — Background daemon written in Go. Owns all network I/O, peer state, and link persistence.

```
 User
  │
  ▼
┌──────┐   Unix socket (JSON)   ┌────────────────────────────────────┐
│ lnn  │ ─────────────────────► │              lnnd                  │
│ (C)  │                        │                                    │
└──────┘                        │  ┌──────────┐   ┌───────────────┐  │
                                │  │   IPC    │   │    Network    │  │
                                │  │ listener │   │   listener    │  │
                                │  └──────────┘   └───────┬───────┘  │
                                │                         │ TCP      │
                                └─────────────────────────┼──────────┘
                                                          │
                                                    Remote peers
```

---

## lnnd internals

### Package map

```
cmd/lnnd/           Entry point
internal/
  config/           Env file loader (configs/lnnd.conf)
  daemon/           Daemon orchestrator — wires services together
  identity/         Node identity: UUID + Ed25519 keypair (~/.lnn/)
  ipc/              Unix socket listener, JSON request parsing/routing
  network/          TCP listener, handshake, message dispatch loop
  peer/             Thread-safe peer registry, peer state machine
  protocol/         Wire format: encoding, parsing, constants
  logger/           Colored loggers (lnnd / lnnp prefixes)
```

### Startup sequence

```
main()
 └─ config.LoadEnv("configs/lnnd.conf")   load env vars
 └─ daemon.New()
     ├─ identity.NewService()             load/generate UUID + Ed25519 keys
     └─ peer.NewPeerManager()             init empty peer registry
 └─ daemon.Start()
     ├─ go network.ListenTCP()            TCP server (goroutine)
     └─ ipc.ListenIPC()                   Unix socket server (blocks main)
```

---

## Protocol

### Wire format

Every message starts with a fixed 10-byte header:

```
 0       4    5    6           10
 ┌───────┬────┬────┬───────────┐
 │ Magic │ Ver│Type│  Length   │  ...payload...
 └───────┴────┴────┴───────────┘
   4 B    1 B  1 B    4 B (BE)
```

| Field  | Size | Value             |
|--------|------|-------------------|
| Magic  | 4 B  | `0x4C 0x4E 0x4E 0x00` (`LNN\0`) |
| Version| 1 B  | `0x01`            |
| Type   | 1 B  | see message types |
| Length | 4 B  | payload size (big-endian) |

### Message types

| Code | Name              | Payload size | Description                          |
|------|-------------------|--------------|--------------------------------------|
| `0`  | HELLO             | 48 B         | UUID (16) + Ed25519 public key (32)  |
| `1`  | CHALLENGE         | 32 B         | Random nonce                         |
| `2`  | CHALLENGE_RESP    | 64 B         | Ed25519 signature over nonce         |
| `3`  | ERROR             | 43 B         | Error code (int64) + message (35 B)  |
| `4`  | LINK_SYM          | 512 B        | Direct symlink: source + target |
| `5`  | LINK_HARD         | 512 B        | Direct hard link: source + target |
| `6`  | LINK_SYM_REQUEST  | variable (max 8196 B) | Request peer to create a symlink. Includes src and an optional dst on the receiver's machine. |
| `7`  | LINK_HARD_REQUEST | variable (max 8196 B) | Request peer to create a hard link. Includes src and an optional dst on the receiver's machine. |
| `8`  | OK                | 0 B          | Success acknowledgment               |

Path fields (source/target) are fixed at 256 bytes, null-terminated.

### Handshake

Mutual authentication via Ed25519 challenge-response. Both sides send their credentials and prove ownership of their private key before any application message is exchanged.

```
  Initiator (outbound)          Responder (inbound)
        │                              │
        │──── HELLO ──────────────────►│
        │──── CHALLENGE ──────────────►│  (nonce_A)
        │                              │
        │◄─── HELLO ───────────────────│
        │◄─── CHALLENGE ───────────────│  (nonce_B)
        │◄─── CHALLENGE_RESP ──────────│  sign(privKey_B, nonce_A)
        │                              │
        │──── CHALLENGE_RESP ─────────►│  sign(privKey_A, nonce_B)
        │                              │
     [verify sig_B]               [verify sig_A]
        │                              │
      StateVerified               StateVerified
```

After the handshake both peers are registered in the `PeerManager` with state `StateVerified` and enter the application message loop.

### Peer state machine

```
StateNew ──► StateHandshake ──► StateVerified ──► StateClosing
```

| State          | Description                          |
|----------------|--------------------------------------|
| `StateNew`     | Connection just opened               |
| `StateHandshake` | HELLO / CHALLENGE in progress      |
| `StateVerified`  | Authentication complete, ready     |
| `StateClosing`   | Tearing down connection            |

Handshake messages received in `StateVerified` are rejected with `ERROR(ErrInvalidState)`. Unknown messages are rejected with `ERROR(ErrInvalidMessage)`.

---

## Trust model

Every authenticated peer has a permission set that controls what it can do on the local machine without going through an explicit request.

### Message flow by permission

```
No path permissions              Local daemon
      │                               │
      │──── LINK_SYM_REQUEST ────────►│  queued, user must --accept or --reject
      │     (src + optional dst)      │  if sender included dst, receiver cannot change it
      │                               │  if sender omitted dst, receiver specifies it on --accept
      │◄─── OK (after accept) ────────│

With path permissions            Local daemon
      │                               │
      │──── LINK_SYM ───────────────►│  executed immediately if dst is in allowed paths
      │◄─── OK / ERROR ───────────────│
```

### Permission templates

When you run `--add-peer`, the peer is assigned a permission set copied from a **template**. The template is a snapshot — it initialises the peer's permissions, but subsequent changes to the template do not affect already-added peers.

Templates live in `~/.lnn/templates/`. Each template starts with an **anti-flood rule** on the first line, followed by allowed paths:

```
<anti-flood-rule>
/path/one
/path/two
```

Three templates are provided by default:

**`unknown`** — applied to every peer that has not been explicitly added, and to peers reset with `--remove-peer id`. Contains only the anti-flood rule, no path permissions. Requests from these peers are queued and require `--accept`.

**`trusted`** (default for `--add-peer`) — anti-flood rule plus a curated list of safe directories shipped with the install. Peers added with this template can create direct links only inside those paths.

**`max`** (used with `--add-peer --max`) — anti-flood rule plus `/*`. The peer can write to any path. Not recommended to modify; create a custom template instead.

### Anti-flood rules

The first line of every template (including `unknown`) defines a request rate limit for that peer:

```
<duration>;<count>requests
```

Duration format — use one field or combine them with `:`:

Two formats are supported:

**Simple** — one value + unit:

```
4hours;3requests         # max 3 requests per 4 hours
3days;1requests          # max 1 request per 3 days
1week;9requests          # max 9 requests per week
never;900requests        # after 900 requests total, block permanently
```

**Compound** — `YY:MM:WW:DD:HH:SS:MS;Xrequests`, for precise intervals. Empty fields can be skipped with `::`:

```
03::01::03:30:10:01;4requests   # 3 years, 1 week, 3h 30m 10s 1ms; max 4 requests
```

| Position | Unit         |
|----------|--------------|
| `YY`     | years        |
| `MM`     | months       |
| `WW`     | weeks        |
| `DD`     | days         |
| `HH`     | hours        |
| `SS`     | seconds      |
| `MS`     | milliseconds |

Use the simple format for typical rate limits. The compound format is there when you need sub-second precision or multi-unit windows.

When a peer exceeds their rate limit an alert is generated. To change the anti-flood value of an existing peer:

```bash
lnn --peer 192.168.1.34 --anti-flooding 4hours;3requests
```

This updates only that peer's entry — the template is not touched.

### Per-peer permissions

After `--add-peer`, you can grant a specific peer access to additional paths not in any template. These extra paths are stored only on that peer's entry:

```bash
lnn --peer 192.168.1.34 --allow /path/to/specific/dir
```

### Removing permissions

| Command                                          | Effect                                                                 |
|--------------------------------------------------|------------------------------------------------------------------------|
| `--remove-peer id`                               | Strips all path permissions. Internally applies the `unknown` template (anti-flood only). Does **not** change the anti-flood value. |
| `--remove-peer id --template my-template`        | Removes only the paths listed in that template. Extra per-peer paths and anti-flood value are unchanged. |
| `--remove-peer id /*`                            | Removes all path permissions, same as bare `--remove-peer id`.         |
| `--remove-peer id /path/to/specific`             | Removes only that one path from the peer's entry.                      |

### Custom templates

```bash
lnn --add-peer 192.168.1.34 --template my-template
```

`~/.lnn/templates/my-template`:
```
1week;5requests
/srv/shared
/var/data/exports
```

This lets you maintain different permission profiles without touching the built-in templates.

### Path matching

| Pattern      | Meaning                              |
|--------------|--------------------------------------|
| `/foo/bar`   | Exactly that directory               |
| `/foo/*`     | All paths under `/foo/`              |
| `/*`         | All paths on the filesystem          |

Incoming `LINK_SYM` / `LINK_HARD` dst paths are checked against the peer's allowed paths before execution. A mismatch is rejected with `ERROR`. `LINK_SYM_REQUEST` / `LINK_HARD_REQUEST` bypass this check and go to the queue — the user decides on `--accept`.

### Alert system

Alerts are generated when:
- A peer exceeds their anti-flood request rate
- An inbound `LINK_SYM` / `LINK_HARD` targets a path outside the peer's allowed set
- A path in a link message contains suspicious or malformed patterns

```bash
lnn --alert          # show unread alerts
lnn --alert all      # show full alert history
```

Alerts are stored in `~/.lnn/alerts`.

---

## IPC

The CLI sends JSON over a Unix socket (path set by `LNND_IPC_SOCKET`). The daemon dispatches on the `op` field.

```json
{ "op": "link", "type": "sym", "src": "192.168.1.34://path/to/file", "dst": "./local_link" }
```

Source format: `<ip>://<path>` — the IPC handler validates and normalizes the IP and path before forwarding to the network layer.

---

## Identity storage

Keys and UUID are persisted under `~/.lnn/`:

```
~/.lnn/
  uuid              node UUID (plain text)
  id_ed25519        Ed25519 private key (raw bytes, mode 0600)
  id_ed25519.pub    Ed25519 public key (base64, mode 0644)
```

Generated automatically on first run if missing.

---

## Configuration

`configs/lnnd.conf` — loaded at startup, sets environment variables:

| Variable         | Purpose                          |
|------------------|----------------------------------|
| `TCP_PORT`       | Port for the TCP peer listener   |
| `LNND_IPC_SOCKET`| Path to the Unix IPC socket      |
