# IPC — CLI ↔ daemon contract

This is the interface between the [`lnn`](https://github.com/mixx86/lnn) CLI and lnnd. lnnd is the server; any client that follows this contract can drive it.

The CLI talks to the daemon over a Unix socket (path set by `LNND_IPC_SOCKET`). Each connection carries exactly one exchange: the client sends one JSON object and **half-closes its write side** (the daemon reads until EOF), then reads one JSON response.

Request:

```json
{ "op": "link", "type": "symreq", "src": "192.168.1.34://path/to/file", "dst": "./local_link" }
```

| Field  | Meaning |
|--------|---------|
| `op`   | Operation. Currently `link`. |
| `type` | `symreq` — ask a peer to create a symlink. `hard` is not implemented yet. |
| `src`  | `<ip>://<path>`. The IP must parse; the path is cleaned and made absolute. |
| `dst`  | Destination path (optional). |
| `uuid` | The peer's UUID (optional). If it names a peer with an open session, the daemon sends over that session instead of dialing `<ip>`. |

Response:

```json
{ "ok": true }
{ "ok": false, "err": "unsupported transport: quic" }
```

An invalid request (bad `src` or `uuid`) gets `ok: false` with the reason.

For a `symreq`, the daemon reuses the peer's session or dials the peer and runs the outbound handshake, puts the `SYMLINK_REQUEST` in the [outbox](architecture.md#outbox) and sends it, then reports `ok: true`. `ok: true` means the frame was written to the connection, not that the peer stored or accepted it.
