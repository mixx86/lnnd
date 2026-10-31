# Coding style

`gofmt` is mandatory and not up for discussion. This document covers the
rules `gofmt` does not enforce.

## 1. Line length: 80 columns

No line may exceed **80 columns**, counting a tab as **4 columns**.

`gofmt` has no line-length setting and never wraps lines; it only decides
indentation and alignment. Line length has to be handled separately:

- **`make golines`** — [`golines`](https://github.com/segmentio/golines)
  wraps long lines and then runs `gofmt` on the result. It cannot split
  comments, so those have to be fixed by hand.
- **`make lll`** — the `lll` linter in
  [`golangci-lint`](https://golangci-lint.run) reports every line over the
  limit. Settings live in [`.golangci.yml`](../.golangci.yml).
- **`make fmt`** — plain `gofmt`.

```bash
go install github.com/segmentio/golines@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

When a line is too long, restructure it instead of breaking it at an
arbitrary point: name an intermediate value, move a long condition into a
variable, or put one argument per line.

```go
// Bad: breaks in the middle of an expression.
if err := s.persistRequests(batch,
    time.Now()); err != nil {

// Good: one argument per line, closing paren on its own line.
err := s.persistRequests(
    batch,
    time.Now(),
)
if err != nil {
```

Exceptions: URLs and string literals that cannot be split meaningfully
(e.g. a long error message that must stay greppable). Struct tags count too,
but may exceed the limit if splitting them is impossible.

## 2. Comments explain *why*, never *what*

The code already says what it does. A comment is only justified when it
carries information the code cannot: the reason behind a choice, a
constraint coming from outside, a trade-off, or a trap for the next reader.

```go
// Bad: repeats the name and signature.
// NewService creates a new IdentityService by loading or generating
// local credentials.
func NewService() (*Service, error) {

// Bad: narrates the next line.
// Increment the tag.
tag = s.Tag.Add(1)
```

```go
// Good: explains a decision the code cannot express.
// Reply only after fsync: OK promises the peer the request survives
// a crash, so acknowledging earlier would be a lie.
<-req.Written

// Good: justifies a design constraint.
// ipc receives app.Services instead of *daemon.Daemon: daemon already
// imports ipc, so importing it back would create a cycle.
func HandleConn(conn net.Conn, s app.Services) {

// Good: records an external fact.
// 4096 is Linux PATH_MAX; longer paths could never be created anyway.
const maxPathLen = 4096
```

Rules:

- **No doc comments that restate the identifier.** `// Foo does foo` is
  noise. If an exported identifier needs a doc comment, it must say
  something the name does not (preconditions, ownership, concurrency
  guarantees, units, why it exists).
- **No section banners or commented-out code.** Git keeps history.
- **No `TODO` without context.** Write what is missing and why it is not
  done yet, or open an issue and reference it.
- If a *what* comment feels necessary, the code is probably unclear:
  rename, extract a function or introduce a named constant instead.
- Comments that justify a non-obvious protocol or storage decision should
  also be reflected in `docs/`; the comment points at the code, the doc
  explains the design.

Note: linters such as `revive`'s `exported` rule demand a doc comment on
every exported identifier. That rule is **disabled** in this project because
it pushes towards *what* comments.

## 3. Naming consistency

One concept, one name, everywhere: package names, file names, types,
functions, variables, wire names and documentation.

- **Pick one word per concept and use it in every layer.** If the wire
  protocol calls it a *message*, it is not a *msg* in one file, a *frame*
  in another and *messaging* in a file name. Abbreviations are allowed only
  if they are used consistently (`Msg` everywhere or `Message`
  everywhere, never both).
- **File names use the same noun as the code they hold.** `message.go` in
  one package and `messaging.go` in another for the same concept is wrong.
- **Same word for the same kind of thing.** If the message type enum is a
  `Kind`, the method returning it is `Kind()`, not `Type()`.
- **Follow Go initialisms:** `OK`, `ID`, `UUID`, `IP`, `TCP`, `IPC`, `WAL`
  stay fully upper case (`RequestID`, not `RequestId`; `OK`, not `Ok`).
- **Compound words follow the standard library:** `Symlink` (as in
  `os.Symlink`), `Checksum`, `Hostname` — not `SymLink`, `CheckSum`.
- **Don't repeat the package name.** `ipc.HandleConn`, not
  `ipc.HandleConnUNIX`; `identity.Service`, not `identity.IdentityService`.
- **Singular vs plural is a decision, not an accident.** Collections and
  the files/queues that hold many items are plural (`RequestsQueue`,
  `requests.go`); a single item is singular (`PendingRequest`). Apply it the
  same way in every package.
- **Docs follow the code.** When a name changes, update `docs/` in the same
  commit.

## 4. Errors and the rest

- Error strings are lower case and have no trailing punctuation
  (`fmt.Errorf("open requests file: %w", err)`).
- Wrap with `%w` and a short context describing the operation that
  failed, not the function name.
- Everything not covered here follows
  [Effective Go](https://go.dev/doc/effective_go) and the
  [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
