# Protocol

The wire protocol is **LNNP** — the *LiNk + Networks Protocol*. The name traces the project's origin: Unix `ln` (link) extended with an `n` for *network*, surfaced as the capitals in **LiNk** + **Networks** → `lnn`. The connector `+` is decorative; the acronym is **LNN** (`lnn` CLI, `lnnd` daemon, `lnnp` protocol).

## Wire format

Every message starts with a fixed 14-byte header:

```
 offset  0       4    5    6          10         14
         ┌───────┬────┬────┬───────────┬───────────┐
         │ Magic │ Ver│Type│    Tag    │  Length   │  ...payload...
         └───────┴────┴────┴───────────┴───────────┘
           4 B    1 B  1 B   4 B (BE)    4 B (BE)
```

| Field   | Size | Value                                            |
|---------|------|--------------------------------------------------|
| Magic   | 4 B  | `0x4C 0x4E 0x4E 0x00` (`LNN\0`)                  |
| Version | 1 B  | `0x01`                                           |
| Type    | 1 B  | see message types                                |
| Tag     | 4 B  | request/response correlation id (big-endian)     |
| Length  | 4 B  | payload size (big-endian)                        |

**Tags.** A sender picks a tag for a request; replies (`OK`, `ERROR`) carry the tag of the request they answer. Tag `0` means "untagged".

Tags are allocated **per peer**, not per connection, so they keep their meaning across reconnects and restarts (see [Recovery](#recovery)). The counter is circular and skips `0`; after a crash it jumps ahead 1024 positions so a tag that reached a peer but not the local disk is never reused.

The receiver validates the announced length against the message type before reading the payload.

## Message types

| Code | Name               | Payload size           | Description |
|------|--------------------|------------------------|-------------|
| `0`  | HELLO              | 48 B                   | UUID (16) + Ed25519 public key (32) |
| `1`  | CHALLENGE          | 32 B                   | Random nonce |
| `2`  | CHALLENGE_RESP     | 64 B                   | Ed25519 signature over the peer's nonce |
| `3`  | ERROR              | 43 B                   | Error code (int64, 8 B) + message (35 B, zero-padded) |
| `4`  | LINK_SYM           | reserved               | Direct symlink creation (peers with path permissions) |
| `5`  | LINK_HARD          | reserved               | Direct hard link creation (peers with path permissions) |
| `6`  | SYMLINK_REQUEST    | variable, 7–8196 B     | Ask a peer to create a symlink. Carries `src` and an optional `dst` on the receiver's machine. |
| `7`  | HARDLINK_REQUEST   | reserved               | Ask a peer to create a hard link |
| `8`  | OK                 | 0 B                    | Success acknowledgment (echoes the request tag) |
| `9`  | RECOVER_REQUEST    | 0 B                    | After a crash, asks a peer to resume the exchange identified by the header tag |
| `10` | RESOLUTION         | 5 B                    | Tells a peer whether its request was accepted or rejected: `[sender_req_id:4][accepted:1]`, where `sender_req_id` is the one the peer sent the request with |

**Request payloads** start with `[sender_req_id:4]`, the sender's id for the request, whatever the request's type. It is never `0`: a request with `sender_req_id` 0 is answered with `ERROR(ErrInvalidMessage)`. The receiver stores it as `rmt_req_id`; it is what a `RESOLUTION`, not sent yet, names the request by.

**SYMLINK_REQUEST payload:** `[sender_req_id:4][src_len:2][src][dst_len:2][dst]`. The `dst_len`/`dst` pair is omitted when the sender doesn't choose a destination. Paths must be absolute and are cleaned on receipt; `src` is capped at 4096 bytes (Linux `PATH_MAX`). Variable length avoids padding every request to a fixed size.

## Error codes

| Code | Name                 | Message                              |
|------|----------------------|--------------------------------------|
| `-1` | `ErrInvalidMessage`  | invalid message                      |
| `-2` | `ErrInvalidState`    | invalid state                        |
| `-3` | `ErrRequestRejected` | request rejected                     |
| `-4` | `ErrOverloaded`      | daemon overloaded                    |
| `-5` | `ErrTooManyPending`  | *(no message — internal limit)*      |
| `-6` | `ErrPendingTimeout`  | timed out waiting for a response     |
| `-7` | `ErrInternal`        | internal error                       |
| `-8` | `ErrUnknownTag`      | unknown tag                          |

## Handshake

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
```

- The responder applies a 5-second deadline to the handshake; it is cleared once the handshake succeeds.
- TCP keep-alive is configured per connection (default period 30 s).
- On success a `Session` is created and registered in the `SessionManager`, keyed by the peer's UUID. Registering an existing UUID silently replaces the old session, which is the correct behaviour on reconnect.
- The handshake proves that the remote holds the private key matching the public key it advertises. Pinning keys to known peers arrives with peer management.

## Message dispatch

After the handshake each connection runs a read loop (`ReadFrame` → `HandleTCPMessage`):

| Message received                          | Action |
|-------------------------------------------|--------|
| `OK`                                      | Removes the tag from the outbox and resolves the pending message |
| `ERROR` with a tag                        | Same as `OK`, logged; on `ErrUnknownTag` the matching recovery entry is dropped. The connection stays open |
| `ERROR` without a tag                     | Logged as a peer error. The connection stays open |
| `RECOVER_REQUEST`                         | Resends the outbox frame with that tag, or replies `ERROR(ErrUnknownTag)` |
| `SYMLINK_REQUEST`                         | Submitted to the scheduler (see [lifecycle](architecture.md#inbound-request-lifecycle)); replies `OK` or `ERROR` with the request's tag. If the tag was owed by a recovery, the entry is resolved once the request is durable |
| `HELLO` / `CHALLENGE` / `CHALLENGE_RESP`  | Replied with `ERROR(ErrInvalidState)`. The connection stays open |
| a payload that fails to parse             | Replied with `ERROR(ErrInvalidMessage)` with the frame's tag. The connection stays open: the payload was read whole, so the next frame starts where expected |

The connection closes only when the stream can no longer be followed: the connection fails, or a header is malformed, announces an unknown type or an invalid payload length. No `ERROR` is sent then, since the frame's tag cannot be trusted.

Frames are written under a per-session mutex (`Session.Mu`) because the network read goroutine and IPC handlers can write to the same connection concurrently. Before the handshake completes, the connection belongs to a single goroutine and unlocked writes are used.

## Recovery

Recovery lets a daemon that crashed while persisting a request get it back from the peer that sent it.

```
  Sender                                   Receiver (crashed)
     │                                            │
     │──── SYMLINK_REQUEST (tag 42) ─────────────►│  WAL write interrupted
     │                                            │  restart: damaged entry,
     │                                            │  lcl_req_id → tag 42 from cache
     │◄─── RECOVER_REQUEST (tag 42) ──────────────│
     │                                            │
     │──── SYMLINK_REQUEST (tag 42) ─────────────►│  resend from outbox
     │◄─── OK (tag 42) ───────────────────────────│  durable; recovery resolved
     │                                            │
     │   or, if tag 42 is no longer in the outbox:│
     │──── ERROR(ErrUnknownTag, tag 42) ─────────►│  recovery entry dropped
```

**Sender — outbox.** Every outgoing request is kept per peer, keyed by tag, until the peer answers with `OK` or `ERROR`. The receiver only sends `OK` after the request and its checkpoint are fsynced, so a confirmed request can never need recovery. The window holds 256 requests per peer; when it is full the oldest one is dropped, and asking for it yields `ErrUnknownTag`. The outbox is mirrored to `~/.lnn/outbox` so the sender can answer even after its own crash.

**Receiver — pending recoveries.** At startup, damaged request entries whose `peer_uuid` and `lcl_req_id` survived are looked up in the requests cache for their tag and written to `~/.lnn/requests.recovery` before the damaged WAL region is truncated. An entry leaves the list when the peer resends the request (once it is durable again) or answers `ErrUnknownTag`.

After every handshake, inbound or outbound, the receiver sends one `RECOVER_REQUEST` per pending entry of that peer. Entries are not removed when sent, so if the connection drops before the peer answers, they are asked for again on the next connection.

## Session states

```
StateNew ──► StateHandshake ──► StateVerified ──► StateClosing
```

| State            | Description                        |
|------------------|------------------------------------|
| `StateNew`       | Connection just opened             |
| `StateHandshake` | HELLO / CHALLENGE in progress      |
| `StateVerified`  | Authentication complete, ready     |
| `StateClosing`   | Tearing down connection            |

## Transports

The network layer talks to a `Transport` interface (`Listen`, `Dial`), selected by `config.NetworkConfig.Transport`. TCP is the only implementation today; QUIC is planned (its config fields already exist).

## Inspecting traffic

`tools/lnnp.lua` is a tshark/Wireshark dissector for LNNP: it decodes the header and the payloads of the message types above. `make sniff` captures a live session and prints one line per message, for example:

```
0.021145460  127.0.0.1  47690  127.0.0.1  5771  [LNNP] SYMLINK_REQUEST tag=3 sender_req_id=5 src="/path/file.txt" dst="/local"
0.025038776  127.0.0.1  5771  127.0.0.1  47690  [LNNP] OK tag=3
```

It listens on `lo`, port `5771`, by default; override with `make sniff IFACE=wlp0s20f0u3 PORT=7878`. Capturing needs `sudo`. In Wireshark, load the dissector with `-X lua_script:tools/lnnp.lua` or copy it into the personal Lua plugins folder.
