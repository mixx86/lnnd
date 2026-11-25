# IPC — CLI ↔ daemon contract

This is the interface between the [`lnn`](https://github.com/mixx86/lnn) CLI and lnnd. lnnd is the server; any client that follows this contract can drive it.

The CLI talks to the daemon over a Unix socket (path set by `LNND_IPC_SOCKET`). Each connection carries exactly one exchange: the client sends one JSON message and **half-closes its write side** (the daemon reads until EOF), then reads one JSON response. The message may be at most 100 KiB (102400 bytes) and must be complete within 2 s of connecting.

## Messages

Every message carries these fields; the rest depend on its `type`.

| Field  | Meaning |
|--------|---------|
| `type` | Message type. Currently `request`. |
| `op`   | Operation, required. Which ops exist depends on the type. |
| `uuid` | A peer's UUID (optional). Must parse when present. |

### Requests (`"type": "request"`)

A request is something a peer is asked to do. Fields common to every request:

| Field  | Meaning |
|--------|---------|
| `kind` | What is asked, for `send` (see the kinds below). `list` will take it as a filter. |
| `ip`   | The peer's IP address, required by `send`. Hostnames are not accepted. |

Every op acts on requests, so every one goes in `"type": "request"`. Accepting or rejecting is not a type of its own: the `RESOLUTION` the peer gets is what the op causes on the wire, not what the client asks for.

| Op       | Status |
|----------|--------|
| `send`   | Asks the peer named by `ip` (or by `uuid`, see below) for a request of the given `kind`. |
| `accept` | Planned: accepts a pending request, named by its `id`. |
| `reject` | Planned: rejects a pending request, named by its `id`. |
| `list`   | Planned: lists requests, those of `kind` only when it is given. |

`accept` and `reject` take no `kind`: the request they name already has one. The planned ops are recognized, but for now the daemon only logs a TODO and closes the connection without a response; `id` is not a field yet.

```json
{ "type": "request", "op": "send",   "kind": "symlink", "ip": "192.168.1.34", "src": "/path/to/file", "dst": "/home/me/link" }
{ "type": "request", "op": "accept", "id": 42 }
{ "type": "request", "op": "reject", "id": 42 }
{ "type": "request", "op": "list",   "kind": "symlink" }
```

| Kind       | Fields         | Status |
|------------|----------------|--------|
| `symlink`  | `src`, `dst`   | Sent to the peer as a [`SYMLINK_REQUEST`](protocol.md#message-types). |
| `hardlink` | `src`, `dst`   | Validated but not implemented: the daemon closes the connection without a response. |

Link fields:

| Field | Meaning |
|-------|---------|
| `src` | Path of the file on this machine, the sender's, required. It is cleaned and made absolute; a relative path is taken from `/`. |
| `dst` | Destination path on the peer's machine (optional). The peer rejects a relative `dst`, which the client sees as `ErrPeer`. |

To send, the daemon reuses the session of the peer named by `uuid` if one is open, and otherwise dials `ip` and runs the outbound handshake. It then logs the request in the [requests WAL](architecture.md#requests-wal-requests) and waits until it is durable, puts the `SYMLINK_REQUEST` in the [outbox](architecture.md#outbox), sends it and waits for the peer's answer; dialing and the exchange with the peer have 5 s together, which the logging also counts against. Code `0` means the peer answered `OK`: it stored the request durably and the request is pending, not accepted (see [the inbound request lifecycle](architecture.md#inbound-request-lifecycle)).

## Response

```json
{ "code": 0 }
{ "code": 8, "err": "invalid message" }
```

`code` is always present and is what a client should act on; `0` means success. `err` is present only on failure and is a human-readable reason, not meant to be matched.

| Code | Name                 | Sent when |
|------|----------------------|-----------|
| `0`  | `OK`                 | The operation succeeded. |
| `1`  | `ErrInvalidMessage`  | The message is not valid JSON, has no `op`, or has a `uuid` that does not parse. |
| `2`  | `ErrMessageTooLarge` | The message exceeds 100 KiB. |
| `3`  | `ErrInvalidRequest`  | A request field fails validation (`ip`, `src`), or `kind` is unknown. |
| `4`  | `ErrUnknownOp`       | The `op` is unknown for the message type. |
| `5`  | `ErrUnknownType`     | `type` is missing or unknown. |
| `6`  | `ErrPeerUnreachable` | There is no session with the peer and none could be opened. Nothing was sent. |
| `7`  | `ErrNoReply`         | Sending failed or the peer did not answer in time. Whether the peer stored the request is unknown. |
| `8`  | `ErrPeer`            | The peer answered `ERROR`; `err` carries the peer's message. |
| `9`  | `ErrInternal`        | The request could not be logged before sending, for example because the daemon is overloaded. Nothing was sent. |
| `10` | `ErrTimeout`         | The message could not be read, usually because the client did not half-close within 2 s. |
