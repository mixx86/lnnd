# Protocol

The wire protocol is **LNNP** — the *LiNk + Networks Protocol*. The name traces the project's origin: Unix `ln` (link) extended with an `n` for *network*, surfaced as the capitals in **LiNk** + **Networks** → `lnn`. The connector `+` is decorative; the acronym is **LNN** (`lnn` CLI, `lnnd` daemon, `lnnp` protocol).

## Wire format

Every message starts with a fixed 12-byte header:

```
 offset  0       4    5    6      8          12
         ┌───────┬────┬────┬──────┬───────────┐
         │ Magic │ Ver│Type│ Tag  │  Length   │  ...payload...
         └───────┴────┴────┴──────┴───────────┘
           4 B    1 B  1 B   2 B     4 B (BE)
```

| Field   | Size | Value                                            |
|---------|------|--------------------------------------------------|
| Magic   | 4 B  | `0x4C 0x4E 0x4E 0x00` (`LNN\0`)                  |
| Version | 1 B  | `0x01`                                           |
| Type    | 1 B  | see message types                                |
| Tag     | 2 B  | request/response correlation id (big-endian)     |
| Length  | 4 B  | payload size (big-endian)                        |

**Tags.** A sender picks a tag for a request; replies (`OK`, `ERROR`) carry the tag of the request they answer. Each session tracks outstanding tagged messages so responses can be matched, with a cap of 65 536 pending messages per session.

## Message types

| Code | Name               | Payload size           | Description |
|------|--------------------|------------------------|-------------|
| `0`  | HELLO              | 48 B                   | UUID (16) + Ed25519 public key (32) |
| `1`  | CHALLENGE          | 32 B                   | Random nonce |
| `2`  | CHALLENGE_RESP     | 64 B                   | Ed25519 signature over the peer's nonce |
| `3`  | ERROR              | 43 B                   | Error code (int64, 8 B) + message (35 B, zero-padded) |
| `4`  | LINK_SYM           | reserved               | Direct symlink creation (peers with path permissions) |
| `5`  | LINK_HARD          | reserved               | Direct hard link creation (peers with path permissions) |
| `6`  | SYMLINK_REQUEST    | variable, 3–8192 B     | Ask a peer to create a symlink. Carries `src` and an optional `dst` on the receiver's machine. |
| `7`  | HARDLINK_REQUEST   | reserved               | Ask a peer to create a hard link |
| `8`  | OK                 | 0 B                    | Success acknowledgment (echoes the request tag) |
| `9`  | RECOVER_REQUEST    | 0 B                    | After a crash, asks a peer to resume the exchange identified by the header tag |
| `10` | RESOLUTION         | 5 B                    | Tells a peer whether its request was accepted or rejected: `[rmt_req_id:4][accepted:1]` |

**SYMLINK_REQUEST payload:** `[src_len:2][src][dst_len:2][dst]`. The `dst_len`/`dst` pair is omitted when the sender doesn't choose a destination. Paths must be absolute and are cleaned on receipt; `src` is capped at 4096 bytes (Linux `PATH_MAX`). Variable length avoids padding every request to a fixed size.

## Error codes

| Code | Name                 | Message                              |
|------|----------------------|--------------------------------------|
| `-1` | `ErrInvalidMessage`  | Invalid message.                     |
| `-2` | `ErrInvalidState`    | Invalid state.                       |
| `-3` | `ErrRequestRejected` | Request rejected.                    |
| `-4` | `ErrOverloaded`      | Daemon overloaded.                   |
| `-5` | `ErrTooManyPending`  | *(no message — internal limit)*      |
| `-6` | `ErrPendingTimeout`  | Timed out waiting for a response.    |
| `-7` | `ErrInternal`        | Internal error.                      |

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
| `OK`                                      | Ignored |
| `ERROR`                                   | Logged as a peer error; connection closes |
| `SYMLINK_REQUEST`                         | Submitted to the scheduler (see [lifecycle](architecture.md#inbound-request-lifecycle)); replies `OK` or `ERROR` with the request's tag |
| `HELLO` / `CHALLENGE` / `CHALLENGE_RESP`  | Replied with `ERROR(ErrInvalidState)`; connection closes |
| anything else                             | Replied with `ERROR(ErrInvalidMessage)`; connection closes |

Frames are written under a per-session mutex (`Session.Mu`) because the network read goroutine and IPC handlers can write to the same connection concurrently. Before the handshake completes, the connection belongs to a single goroutine and unlocked writes are used.

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
