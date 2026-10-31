# Filesystem presentation (planned)

How remote links are exposed on the local filesystem. This is owned by lnnd; the [`lnn`](https://github.com/mixx86/lnn) CLI only creates and manages the links.

Remote links will appear on the local filesystem as entries pointing into `~/.lnn/<ip>:/`. The directory `~/.lnn/<ip>:/` is a FUSE mount managed by lnnd.

## Default layout

```
./link -> ~/.lnn/192.168.1.5:/path/to/file
```

## IP change handling

When a peer's IP changes, lnnd renames the mount directory:

```
mv ~/.lnn/192.168.1.5:/ ~/.lnn/192.168.1.6:/
```

Instantaneous — directory rename on the same filesystem, no data moves. All existing local entries remain valid because the kernel resolves paths lazily at access time. lnnd detects IP changes via the handshake: UUID and public key are verified on every connection.

## Peer naming

Users can assign a friendly name to a peer:

```bash
lnn --name raspi 192.168.1.5
```

lnnd renames `~/.lnn/192.168.1.5:/` to `~/.lnn/raspi/`. The UUID remains the canonical identifier internally.

## Optional drivers

**UX driver** — the base installation presents links as `~/.lnn/192.168.1.5:/file`. This optional kernel module changes how entries appear to the VFS: instead of the `~/.lnn/<ip>:/` path, entries appear directly as `192.168.1.5://file` in `ls` and other standard tools. Also surfaces extra metadata (link type, peer name, sync status) natively via the VFS without any wrapper. This is the only way to achieve the `ip://path` presentation without a custom `ls`.

**Performance driver** — the base installation uses io_uring + FUSE, which already eliminates most traditional FUSE overhead. This optional driver goes further by replacing FUSE with a native VFS module, enabling two optimizations that io_uring + FUSE cannot provide:

- **True zero-copy**: data from the remote peer goes directly from the network socket to the kernel page cache without passing through lnnd in userspace. With FUSE, lnnd must receive the data, then pass it back to the kernel.
- **Native page cache integration**: remote files are treated as first-class entries in the kernel page cache, enabling kernel-managed read-ahead and cache coherency. With FUSE, the kernel cannot apply these optimizations transparently.

Recommended for high-throughput environments: large file transfers at wire speed or high-concurrency small file workloads. Not needed for typical personal or low-volume use.

Both drivers require kernel module installation. On Secure Boot systems the signing key must be enrolled once.
