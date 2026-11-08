# AGENTS.md — instructions for AI coding agents working on lnnd

> **FIRST AND HIGHEST-PRIORITY INSTRUCTION: before doing absolutely anything
> else, read [`docs/coding-style.md`](docs/coding-style.md) in full.**

## 1. Learn the project before changing it

1. Read [README.md](README.md) and [docs/architecture.md](docs/architecture.md),
   then the documents of the area you will touch (the README's documentation
   table lists them). [trust-model.md](docs/trust-model.md) and
   [filesystem.md](docs/filesystem.md) describe planned features: nothing in
   them exists unless the code says so.
2. Read the code you will change, its callers and its tests.
3. Read the history of what you touch (`git log -p -- <path>`); comments and
   commit bodies record why things are the way they are.
4. Record a baseline before changing anything: `go build ./...`,
   `go vet ./...`, `go test ./...`. You need to know what already failed.

## 2. Describe intent, not the current instance

The code implements part of a broader design. Describe every component by
the role it is designed for. What exists today is one case among others:
present it as an example or as status, never as the first or the main one:

- `SYMLINK_REQUEST` is one request type among the many to come: describe the
  request path for requests, not for symlinks.
- The store persists incoming content that must survive a crash, not
  "requests": requests are one kind of that content among others it may
  hold in the future, such as links.

Before writing a description, a name or a comment, ask what the component
is for and whether the sentence will still be true when the next request
type or the next kind of content lands. Real names stay exact
(`RequestsQueue`, the `requests` log), but they do not define the concept.

## 3. Architecture

lnnd is layered: listeners at the top, peer sessions and the write scheduler
in the middle, storage at the bottom.
[docs/architecture.md](docs/architecture.md) describes the layers and their
packages.

- **Take the structure from the code.** Describe the layers that the
  packages and the data path actually form. Do not impose groupings,
  taxonomies or abstractions (interfaces, registries, plugin systems) that
  the code does not have: `daemon.New` wires every component explicitly and
  `network.HandleTCPMessage` dispatches every message explicitly.
- **Import cycles shape the packages**: `recovery` is separate so `network`
  and `store` do not import each other; `ipc` takes `app.Services` because
  `daemon` imports `ipc`.

## 4. Respect intentional design decisions

Unconventional is not incorrect. Before calling something a bug, look for its
reason in comments, docs and history. Deliberate choices in this codebase
include:

- `SessionManager.Register` silently replaces the session of a UUID: a
  reconnecting peer must replace its stale session.
- `OK` to a request is sent only once its entry and checkpoint are fsynced.
  It means "stored and pending", not "accepted".
- Crash recovery identifies requests by per-peer tag through the requests
  cache, never by `rmt_req_id` (rationale in
  `internal/store/requests_file.go`).
- Tags are allocated per peer, not per connection, skip 0, and jump ahead on
  every start.
- The outbox drops a peer's oldest message when its window is full; a
  dropped tag answers `ErrUnknownTag`.
- The requests cache is written in the background without a write turn.
- Without a detectable disk device the daemon runs without disk monitoring
  instead of refusing to start.
- `config.EnsureConfig` uses `O_EXCL`, so an existing config is never
  overwritten.
- The handshake proves possession of the advertised key; pinning keys to
  known peers is not implemented yet.

Change intentional behaviour only when you can show that it violates a
documented requirement or causes a concrete defect. When you cannot tell
whether something is intentional, investigate further and report the
uncertainty instead of "fixing" it. Never impose your preferred architecture
or style.

## 5. Write meaningful tests

- **Every test needs a failure hypothesis.** Before writing it, state the
  contract it protects, the realistic defect it catches and why the expected
  result is correct. If you cannot, do not write the test. A few high-value
  tests beat many shallow ones; coverage is not a goal.
- **Test contracts, not implementations.** Go through the public API and
  documented behaviour. Reach into internals only to isolate a real failure
  that is otherwise impractical to reach (for example, calling
  `persistRequests` with an abandoned `PendingRequest` instead of waiting
  out the write confirmation timeout), or to synchronize.
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
- **Mind the hard-coded timeouts** in `internal/sched/config.go`: tests that
  wait for them cost real time, so keep them few.
- **Treat peer input as hostile.** `ReadFrame` parses every frame before the
  handshake checks its type. When you add or change a message type, extend
  `FuzzReadFrame` (`internal/network/framing_test.go`) and run it:
  `go test -run '^$' -fuzz '^FuzzReadFrame$' -fuzztime 60s ./internal/network/`.
- **Prove each test can fail**: break the protected behaviour on purpose,
  watch the test fail for the right reason, restore the code.
- **Regression tests for real defects**: understand the root cause,
  reproduce it with a minimal failing test, apply the smallest fix, watch the
  test pass. Assert the contract, not the shape of the fix.

## 6. Investigate and classify findings

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

## 7. Verify and report honestly

Before calling work done:

- `gofmt -l ./cmd ./internal` prints nothing and `make lll` is clean; CI
  runs `lll` with the build and tests on pushes and pull requests to `main`
  and `dev`.
- `go build ./...`, `go vet ./...` and `go test ./...` pass; use
  `go test -race ./...` for concurrent code and repeat timing-sensitive
  tests (`-count=20`) to catch flakiness.
- Inspect the final diff and make sure no unrelated behaviour changed.

Report what ran and its result, what did not run and why, and the known
limitations. Never claim a test passed or a defect was reproduced unless it
actually was, and keep verified facts apart from assumptions.

## 8. Keep the documentation accurate

- Update `docs/` in the same commit as the change it describes.
- Document what the code does today and what it is for (section 2). When
  docs and code disagree, read the code and document the verified behaviour.
- Mark status honestly: implemented, in progress, planned. Never present
  planned work as existing.
- Link to the document that owns a fact instead of restating it, and keep
  each catalog (packages, files, settings) in its table instead of listing
  its members again in prose: copies and lists go stale as the project
  grows.
- A documentation change must not alter protocol semantics, wire formats or
  the IPC contract.
- Other documents link to headings in `architecture.md`; keep those anchors
  working when you rename a heading.

## 9. Commits and history

- Commit only when asked. Never push, force-push or rewrite history without
  explicit authorization: branches are published on `origin`, so rewriting
  them requires a force-push.
- Check `git status` first, stage explicit paths, read `git diff --cached`
  before every commit, and leave unrelated or local files out.
- Never amend or rewrite a commit that is already on the remote; put the
  follow-up change in a new commit.
- One purpose per commit (for example, tests and documentation in separate
  commits). Subjects follow the `type(scope): summary` style of the history
  (`feat`, `fix`, `docs`, `refactor`, `test`, `ci`, `style`, `build`,
  `chore`).
- End every commit you author with a `Co-authored-by` trailer that
  identifies you. If the history already contains your identity, reproduce
  it exactly; never invent an identity or an email address, and never add
  your trailer to commits you did not author. Only the user knows who wrote
  past commits: confirm with them before attributing any.

## 10. Never

- Invent requirements, functionality, abstractions (interfaces, registries,
  frameworks) or architectural behaviour that the code does not have.
- State that a file, command, mechanism or convention exists without
  checking it.
- Assume that unconventional means incorrect.
- Change production code just to make testing easier, unless the change is
  necessary and justified.
- Hide uncertainty or fabricate verification results.
