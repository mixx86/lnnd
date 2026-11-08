# AGENTS.md — instructions for AI coding agents working on lnnd

> **FIRST AND HIGHEST-PRIORITY INSTRUCTION: before doing absolutely anything
> else, read [`docs/coding-style.md`](docs/coding-style.md) in full.**
>
> Read it before opening any other file, running any command, planning,
> investigating or changing anything. No other instruction in this file and
> no task you are given takes precedence over it. Its rules — `gofmt`, the
> 80-column limit with tabs counted as 4, comments that explain *why* and
> never *what*, one name per concept in every layer, error style — apply to
> every line you write: code, tests, comments and documentation.

Everything below assumes you have done that.

## 1. Learn the project before changing it

1. Read [README.md](README.md) and [docs/architecture.md](docs/architecture.md):
   subsystems and shared infrastructure, package map, startup sequence,
   on-disk state. Then read the document of the area you will touch:
   [protocol.md](docs/protocol.md) (wire format, handshake, message
   dispatch, recovery), [ipc.md](docs/ipc.md) (CLI contract),
   [roadmap.md](docs/roadmap.md) (what is done and what is planned).
   [trust-model.md](docs/trust-model.md) and
   [filesystem.md](docs/filesystem.md) describe planned features: nothing in
   them exists unless the code says so.
2. Read the code you will change, its callers and its tests.
3. Read the history of what you touch (`git log -p -- <path>`); comments and
   commit bodies record why things are the way they are.
4. Record a baseline before changing anything: `go build ./...`,
   `go vet ./...`, `go test ./...`. You need to know what already failed.

## 2. Architecture: subsystems built on shared infrastructure

lnnd is a daemon made of **functional subsystems**, each owning one domain,
built on **shared infrastructure**. Keep that model in code and in docs.

- **Shared infrastructure**: the orchestrator (`internal/daemon`,
  `app.Services` in `internal/services.go`), configuration (`config`,
  `configs/`), logging (`logger`), node identity (`identity`), wire protocol
  (`protocol`), network transport, handshake, peer sessions and message
  dispatch (`network`, `peer`), the CLI interface (`ipc`), the write
  scheduler with its disk-load monitor, backpressure and queues (`sched`,
  `queues`), filesystem utilities (`fsutil`).
- **Functional subsystems**: **requests** (the first and most developed),
  **outbox**, **recovery**; file transfers exist only as placeholders
  (`FilesID`, `FilesQueue`); link management, peer trust and filesystem
  presentation are planned. The subsystem table in
  [docs/architecture.md](docs/architecture.md#functional-subsystems) is the
  reference for what exists.
- **Requests are one domain, not the architecture.** Do not describe the
  scheduler, the queues, network dispatch or persistence as request
  machinery, and do not present request-specific pieces as universal ones.
  Request-specific today: `RequestsQueue`, `PendingRequest`, the request
  operations `SubmitRequest` / `NextRequest` / `TryNextRequest`, write source
  `RequestsID`, the requests WAL (`requests`), the requests cache, startup
  repair and the `LNND_REQ_*` settings. Recovery currently serves requests
  only.
- **Do not classify by package alone.** `sched`, `queues`, `network` and
  `ipc` are shared but hold the parts of a subsystem that plug into them;
  `store` hosts persistence domains and has only one today (requests).
- **There is no subsystem interface, registry or plugin mechanism.**
  `daemon.New` wires every component explicitly and
  `network.HandleTCPMessage` dispatches every message explicitly. Do not
  introduce such an abstraction unless asked, and never document one.
- **Subsystems differ.** Not every subsystem has a queue, takes write turns
  or follows the request lifecycle. `outbox` and `recovery` show the
  existing pattern: their own package, their own file under `~/.lnn/`,
  explicit wiring in `daemon.New`, and only the shared services they need.
- **Import cycles shape the packages**: `recovery` is separate so `network`
  and `store` do not import each other; `ipc` takes `app.Services` because
  `daemon` imports `ipc`.

## 3. Respect intentional design decisions

Unconventional is not incorrect. Before calling something a bug, look for its
reason in comments, docs and history. Deliberate choices in this codebase
include:

- `SessionManager.Register` silently replaces the session of a UUID: a
  reconnecting peer must replace its stale session.
- `OK` to a `SYMLINK_REQUEST` is sent only once the entry and its checkpoint
  are fsynced. It means "stored and pending", not "accepted".
- Crash recovery identifies requests by per-peer tag through the requests
  cache, never by `rmt_req_id` (rationale in
  `internal/store/requests_file.go`).
- Tags are allocated per peer, not per connection, skip 0, and jump ahead
  1024 on every start.
- The outbox keeps 256 frames per peer and drops the oldest when full; a
  dropped tag answers `ErrUnknownTag`.
- The requests cache is written in the background without a write turn.
- Without a detectable disk device the daemon runs without disk monitoring
  instead of refusing to start.
- `config.EnsureConfig` uses `O_EXCL`, so an existing config is never
  overwritten.
- The handshake proves possession of the advertised key; pinning keys to
  known peers is not implemented yet.
- `revive`'s `exported` rule is disabled on purpose (see the coding style).

Change intentional behaviour only when you can show that it violates a
documented requirement or causes a concrete defect. When you cannot tell
whether something is intentional, investigate further and report the
uncertainty instead of "fixing" it. Never impose your preferred architecture
or style.

## 4. Write meaningful tests

- **Every test needs a failure hypothesis.** Before writing it, state the
  contract it protects, the realistic defect it catches and why the expected
  result is correct. If you cannot, do not write the test. A few high-value
  tests beat many shallow ones; coverage is not a goal.
- **Test contracts, not implementations.** Go through the public API and
  documented behaviour. Reach into internals only to isolate a real failure
  that is otherwise impractical to reach (for example, calling
  `persistRequests` with an abandoned `PendingRequest` instead of waiting
  out the 10 s write confirmation), or to synchronize.
- **Follow the existing conventions**: standard `testing` only (do not add
  test frameworks or dependencies), tests in the package they test,
  table-driven tests when cases share a shape, `t.Helper()` in helpers,
  `t.TempDir()` for files, `t.Setenv` for configuration (such tests cannot
  call `t.Parallel()`), `net.Pipe` for connections — its writes block until
  the other side reads.
- **Never touch the real `~/.lnn`.** Point `HOME` at `t.TempDir()` (see
  `newIdentity` in `internal/network/handshake_test.go`) or pass temporary
  paths.
- **Stay deterministic.** Do not use sleeps as synchronization: poll a
  condition with a deadline. When asserting that something does *not*
  happen, use a short window that can only cause false passes, never false
  failures. Make a broken implementation fail fast instead of hanging, for
  example by draining the peer side of a `net.Pipe`.
- **Mind the hard-coded timeouts** in `internal/sched/config.go` (100 ms
  enqueue, 2 s pressure wait, 10 s write confirmation): tests that wait for
  them cost real time, so keep them few.
- **Treat peer input as hostile.** `ReadFrame` parses every frame before the
  handshake checks its type. When you add or change a message type, extend
  `FuzzReadFrame` (`internal/network/framing_test.go`) and run it:
  `go test -run '^$' -fuzz '^FuzzReadFrame$' -fuzztime 60s ./internal/network/`.
- **Prove each test can fail**: break the protected behaviour on purpose,
  watch the test fail for the right reason, restore the code.
- **Regression tests for real defects**: understand the root cause,
  reproduce it with a minimal failing test, apply the smallest fix, watch the
  test pass. Assert the contract, not the shape of the fix.

## 5. Investigate and classify findings

A failing test is not a bug yet. First check that its expectation follows
from an actual requirement or contract, that the behaviour is not
intentional, that the test makes no invalid assumption, that the failure
does not come from the environment, fixtures or mocks, and that it
reproduces. Fix or discard a test built on a wrong assumption; never weaken a
valid test to make the suite green, and never change correct production
behaviour to satisfy an unjustified assertion.

Classify every finding:

- **Confirmed bug** — a reproducible violation of a documented requirement
  or contract, with a demonstrated wrong outcome.
- **Potential bug** — a plausible failure that has not been demonstrated;
  say what evidence is missing.
- **Test gap** — an important behaviour or invariant without protection, no
  defect shown.
- **Design observation** — another approach is possible but the current one
  is not incorrect. Never present it as a bug or as a required change.

Do not fill categories for the sake of it, and do not report preferences as
findings.

## 6. Verify and report honestly

Before calling work done:

- `gofmt -l ./cmd ./internal` prints nothing (`make fmt` rewrites).
- No line exceeds 80 columns, tabs counted as 4: `make lll` (needs
  `golangci-lint`; CI runs it with the build and tests on pushes and pull
  requests to `main` and `dev`).
- `go build ./...`, `go vet ./...` and `go test ./...` pass; use
  `go test -race ./...` for concurrent code and repeat timing-sensitive
  tests (`-count=20`) to catch flakiness.
- Inspect the final diff and make sure no unrelated behaviour changed.

Report what ran and its result, what did not run and why, and the known
limitations. Never claim a test passed or a defect was reproduced unless it
actually was, and keep verified facts apart from assumptions.

## 7. Keep the documentation accurate

- Docs follow the code: update `docs/` in the same commit as the change it
  describes (the coding style requires it for renames).
- Document what the code does today. When docs and code disagree, read the
  code and document the verified behaviour.
- Mark status honestly: implemented, in progress, planned. Never present
  planned work (trust model, filesystem presentation, file transfers,
  request resolution, QUIC, TLS) as existing.
- Generalize architectural concepts, not implementation details: the
  requests WAL stays the requests WAL and `SubmitRequest` stays a request
  operation. Never replace "request" with "subsystem" mechanically, and do
  not make subsystems sound identical when their lifecycles differ.
- A documentation change must not alter protocol semantics, wire formats or
  the IPC contract.
- README, `roadmap.md`, `protocol.md` and `ipc.md` link to headings in
  `architecture.md`; keep those anchors working when you rename a heading.

## 8. Commits and history

- Commit only when asked. Never push, force-push or rewrite history without
  explicit authorization: branches are published on `origin`, so rewriting
  them requires a force-push.
- Check `git status` first, stage explicit paths, read `git diff --cached`
  before every commit, and leave unrelated or local files out.
- One purpose per commit (for example, tests and documentation in separate
  commits). Subjects follow the `type(scope): summary` style of the history
  (`feat`, `fix`, `docs`, `refactor`, `test`, `ci`, `style`, `build`,
  `chore`).
- End every commit you author with a `Co-authored-by` trailer that
  identifies you. If the history already contains your identity, reproduce
  it exactly; never invent an identity or an email address, and never add
  your trailer to commits you did not author.

## 9. Never

- Invent requirements, functionality, abstractions (interfaces, registries,
  frameworks) or architectural behaviour that the code does not have.
- State that a file, command, mechanism or convention exists without
  checking it.
- Assume that unconventional means incorrect.
- Change production code just to make testing easier, unless the change is
  necessary and justified.
- Hide uncertainty or fabricate verification results.
