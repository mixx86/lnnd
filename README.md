# lnn

**lnn** extends the Unix `ln` command with network capabilities — create persistent links to files on remote machines as if they were local.

> Work in progress — core protocol implemented, link operations in development.

```bash
lnn 192.168.1.34://var/log/app.log ./app.log   # symlink to a remote file
lnn --add-peer 192.168.1.34                   # trust a peer
lnn --peers                                   # list known peers
```

## How it works

lnn consists of two components:

- **`lnn`** — CLI client (C). Drop-in replacement for `ln` with network extensions.
- **`lnnd`** — Background daemon (Go). Manages peer connections, the wire protocol, and link persistence.

The CLI sends requests to the local daemon over a Unix socket. The daemon handles all TCP communication with remote peers, authenticating each one via Ed25519 mutual handshake before exchanging any data.

See [docs/architecture.md](docs/architecture.md) for a full breakdown.

## Build

**Requirements:** GCC, Go 1.21+, Make

```bash
git clone https://github.com/d3xxdev/lnn.git
cd lnn
make
```

## Usage

### Remote links

```bash
lnn 192.168.1.34://path/to/file.txt ./local_link    #  explicit hardlink to a remote file
lnn -s 192.168.1.34://path/to/file.txt ./local_link # explicit symlink to a remote file
lnn 192.168.1.34 ./local_link                      # open request — remote chooses the file
```

### Peer management

```bash
lnn --add-peer 192.168.1.34                      # add peer using default trusted template
lnn --add-peer 192.168.1.34 --max                # add peer with max trust (any path)
lnn --add-peer 192.168.1.34 --template mytempl   # add peer using a custom template
lnn --peer 192.168.1.34 --allow /path/to/dir     # grant extra path to an existing peer
lnn --peer 192.168.1.34 --anti-flooding 4hours;3requests  # set rate limit for a peer
lnn --block 192.168.1.34                         # block a peer
lnn --peers                                      # list known peers
lnn --peers 192.168.1.34                         # show peer info, paths, and rate limit
```

```bash
lnn --remove-peer 192.168.1.34                   # strip all paths (revert to unknown template)
lnn --remove-peer 192.168.1.34 --template tmpl   # remove only the paths from that template
lnn --remove-peer 192.168.1.34 /path/to/dir      # remove a single path
```

Trusted peers can create links directly (no request needed) inside the paths their template grants them. The template is a snapshot — modifying the template later does not change already-added peers. See [docs/architecture.md](docs/architecture.md) for the full permission model.

### Alerts

```bash
lnn --alert        # show unread alerts
lnn --alert all    # show full alert history
```

Alerts are generated when a peer exceeds their request rate limit or attempts a link to an unauthorized path.

### Incoming requests

```bash
lnn --requests                       # list pending requests
lnn --requests all                   # list all requests
lnn --accept 192.168.1.34:id /path   # accept and resolve a request
```

### Daemon

```bash
lnnd start    # start the daemon
lnnd stop     # stop the daemon
lnnd status   # show status
```

### Local (`ln` compatible)

```bash
lnn file.txt link.txt       # hard link
lnn -s file.txt link.txt    # symbolic link
```

## Status

| Feature                        | Status      |
|-------------------------------|-------------|
| Wire protocol + message types  | Done        |
| Ed25519 mutual handshake       | Done        |
| Peer state machine             | Done        |
| TCP message dispatch loop      | Done        |
| IPC (CLI ↔ daemon)             | Done        |
| Remote symlinks                | In progress |
| Remote hard links              | In progress |
| Peer management (`--add-peer`) | Planned     |
| Persistent link database       | Planned     |
| TLS encryption                 | Planned     |
| Sync mode (`--sync`)           | Planned     |
| Smart links                    | Planned     |

Full roadmap: [docs/roadmap.md](docs/roadmap.md)

## License

[Apache 2.0](LICENSE)
