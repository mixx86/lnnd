# lnn

lnn extends the Unix ln command with network capabilities, allowing you to create persistent links to files on remote machines as if they were local.

> ⚠️ **Work in progress** — core protocol in development.

## Features

### Core
- All original `ln` features
- Remote file linking over custom encrypted protocol
- Persistent links with automatic reconnection
- Peer identification via UUID

### Planned
- Configurable encryption method
- Sync mode (`--sync`)
- Smart links (rule-based linking)
- `--analyze` — show hard links, inode info
- `--graph` — visualize link relationships
- High performance, thousands of concurrent links

## Architecture

lnn consists of two components:

- **`lnn`** — the CLI client, written in C. Drop-in replacement for ln with network extensions.
- **`lnnd`** — the background daemon, written in Go. Manages peer connections, the custom protocol, and link persistence.

The CLI communicates with the local daemon via Unix socket (IPC). The daemon handles all network communication with remote peers.

## Installation

### Build from source

**Requirements:**
- GCC
- Go 1.21+
- Make
```bash
git clone https://github.com/d3xxdev/lnn.git
cd lnn
make
```

## Usage

### Remote links
```bash
lnn 192.168.1.34:/path/to/file.txt ./local_link   # symlink to specific remote file
lnn -s 192.168.1.34:/path/to/file.txt ./local_link # explicit symlink
lnn 192.168.1.34 ./local_link                      # request — remote chooses the file
```

### Peer management
```bash
lnn --add-peer 192.168.1.34    # add a trusted peer
lnn --block 192.168.1.34       # block a peer
lnn --peers                    # list known peers
lnn --peers 192.168.1.34       # show info about a peer
```

### Requests
```bash
lnn --requests                          # list pending requests
lnn --requests all                      # list all requests
lnn --accept 192.168.1.34:id /path      # accept a request
```

### Daemon management
```bash
lnnd start     # start the daemon
lnnd stop      # stop the daemon
lnnd status    # show daemon status
```

### Local (original ln features)
```bash
lnn file.txt link.txt          # hard link
lnn -s file.txt link.txt       # symbolic link
```

### Remote hard links (planned)
```bash
lnn 192.168.1.34:/path/to/file.txt ./local_link  # hard link to remote file
```

## Roadmap

- [ ] Core protocol (HELLO, peer identification)
- [ ] TLS encryption
- [ ] Basic daemon (peer management, connections)
- [ ] Remote symlinks
- [ ] Original `ln` features (CLI)
- [ ] IPC (CLI ↔ daemon communication)
- [ ] `--add-peer` / `--block` / `--requests`
- [ ] Sync mode (`--sync`)
- [ ] Remote hard links
- [ ] Smart links (rule-based)
- [ ] `--analyze` / `--graph`
- [ ] Configurable encryption

## License

[Apache 2.0](LICENSE)
