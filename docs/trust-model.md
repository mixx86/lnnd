# Trust model (planned)

The permission model is stored and enforced by lnnd. The commands shown are issued with the [`lnn`](https://github.com/mixx86/lnn) CLI.

> Not implemented yet. Today every peer that completes the handshake is treated as `unknown`: its requests go through the requests queue and wait for resolution.

Every authenticated peer will have a permission set that controls what it can do on the local machine without going through an explicit request.

## Message flow by permission

```
No path permissions              Local daemon
      │                               │
      │──── SYMLINK_REQUEST ─────────►│  persisted + queued, user must --accept or --reject
      │     (src + optional dst)      │  if sender included dst, receiver cannot change it
      │                               │  if sender omitted dst, receiver specifies it on --accept
      │◄─── OK ───────────────────────│  as soon as the request is durably stored
      │                               │
      │◄─── RESOLUTION ───────────────│  after the user accepts or rejects

With path permissions            Local daemon
      │                               │
      │──── LINK_SYM ────────────────►│  executed immediately if dst is in allowed paths
      │◄─── OK / ERROR ───────────────│
```

## Permission templates

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

## Anti-flood rules

The first line of every template (including `unknown`) defines a request rate limit for that peer:

```
<duration>;<count>requests
```

Two duration formats are supported:

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

## Per-peer permissions

After `--add-peer`, you can grant a specific peer access to additional paths not in any template. These extra paths are stored only on that peer's entry:

```bash
lnn --peer 192.168.1.34 --allow /path/to/specific/dir
```

## Removing permissions

| Command                                          | Effect                                                                 |
|--------------------------------------------------|------------------------------------------------------------------------|
| `--remove-peer id`                               | Strips all path permissions. Internally applies the `unknown` template (anti-flood only). Does **not** change the anti-flood value. |
| `--remove-peer id --template my-template`        | Removes only the paths listed in that template. Extra per-peer paths and anti-flood value are unchanged. |
| `--remove-peer id /*`                            | Removes all path permissions, same as bare `--remove-peer id`.         |
| `--remove-peer id /path/to/specific`             | Removes only that one path from the peer's entry.                      |

## Custom templates

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

## Path matching

| Pattern      | Meaning                              |
|--------------|--------------------------------------|
| `/foo/bar`   | Exactly that directory               |
| `/foo/*`     | All paths under `/foo/`              |
| `/*`         | All paths on the filesystem          |

Incoming `LINK_SYM` / `LINK_HARD` dst paths are checked against the peer's allowed paths before execution. A mismatch is rejected with `ERROR`. `SYMLINK_REQUEST` / `HARDLINK_REQUEST` bypass this check and go to the queue — the user decides on `--accept`.

## Alert system

Alerts are generated when:
- A peer exceeds their anti-flood request rate
- An inbound `LINK_SYM` / `LINK_HARD` targets a path outside the peer's allowed set
- A path in a link message contains suspicious or malformed patterns

```bash
lnn --alert          # show unread alerts
lnn --alert all      # show full alert history
```

Alerts are stored in `~/.lnn/alerts`.
