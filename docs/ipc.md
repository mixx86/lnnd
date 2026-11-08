# IPC — CLI ↔ daemon contract

This is the interface between the [`lnn`](https://github.com/mixx86/lnn) CLI and lnnd. lnnd is the server; any client that follows this contract can drive it.

IPC is part of lnnd's shared infrastructure: it is how the CLI reaches the functional subsystems. Its only operation today, `link` with type `symreq`, belongs to the [requests subsystem](architecture.md#requests-subsystem): it sends a link request to a peer.

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

Response:

```json
{ "ok": true }
{ "ok": false, "err": "unsupported transport: quic" }
```

For a `symreq`, the daemon dials the peer, runs the outbound handshake, sends the `SYMLINK_REQUEST` frame and then reports `ok: true`.
