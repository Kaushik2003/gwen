# Conventions

Binding for all code in this repository. Where this file and your instincts disagree, this file
wins — consistency across parallel agents matters more than any individual preference.

## Rules for agents

1. **Implement only your work package.** Its boundaries and owned paths are in
   [10-build-plan.md](10-build-plan.md). Do not edit files owned by another package, even to fix an
   obvious bug — note it in your summary instead. Parallel agents editing shared files is the main
   way this build can fail.
2. **Do not make architecture decisions.** Everything you need is specified. If it genuinely is not,
   stop and report it rather than inventing an answer; see
   [README.md](README.md#change-protocol).
3. **Do not add dependencies.** The allowed module list is in
   [02-architecture.md](02-architecture.md#dependencies). Anything else needs a spec change first.
4. **`make check` must pass before you call a package done.** That is build, vet, test, and
   `docs-check`.
5. **No placeholder implementations.** No `panic("not implemented")`, no stub that returns fake
   data, in anything you report as complete. If you cannot finish, say precisely what is missing.

## Language and layout

Go 1.26. Module path `github.com/kzark/gwen`. Standard layout: `cmd/` for binaries, `internal/` for
everything else. Nothing is exported from the module — there are no external consumers, so
`internal/` is correct for all packages.

Formatting is `gofmt -s`. Linting is `go vet` plus `staticcheck`. Both are wired into `make check`.

## Naming

- Packages: short, lowercase, no underscores, no plurals — `store`, `notify`, `timeengine`.
- Never name a package `util`, `common`, `helpers`, or `shared`. If something has no natural home,
  it is a sign the boundary is wrong.
- Interfaces are named for the role, not the `-er` suffix reflex where it reads badly:
  `ActivityMonitor`, `Notifier`, `Clock`.
- Test helpers live in `_test.go` files; shared fixtures in `internal/testutil`.

## Time

**All instants are `int64` Unix milliseconds UTC** at the storage and wire boundary. In Go code they
become `time.Time` as early as possible and go back to `int64` as late as possible. Never store or
transmit a formatted local timestamp. Local calendar dates (`YYYY-MM-DD`) appear only in the columns
listed in [03-data-model.md](03-data-model.md#time-representation), which also owns the DST rules.

**Never call `time.Now()` outside of `main`.** Take a `Clock`:

```go
type Clock interface {
    Now() time.Time
    NewTimer(d time.Duration) Timer
}
```

The real implementation lives in `internal/clock`; tests use `clock.NewFake(t0)`, which advances
only when told to. This is what makes the time engine testable without sleeping, and it is not
optional — a test that sleeps is a test that is flaky on the Pi.

## Errors

Return errors, never log-and-continue in library code. Wrap with `%w` and enough context to locate
the failure without a stack trace:

```go
if err != nil {
    return fmt.Errorf("close segment %s: %w", id, err)
}
```

Sentinel errors for conditions callers branch on, declared in the owning package:

```go
var ErrNotFound = errors.New("not found")
var ErrInvalidState = errors.New("invalid state for this operation")
```

Do not use `panic` for expected conditions. `panic` is only for programmer error that cannot be
recovered (an impossible switch default, a corrupt embedded asset).

The API layer maps sentinels to the wire codes in
[04-api-contract.md](04-api-contract.md#errors). That mapping is the only place errors become HTTP
status codes.

## Logging

Stdlib `log/slog` only. Structured key-value, never formatted prose.

```go
slog.Info("segment closed", "segment_id", id, "kind", kind, "duration_ms", d.Milliseconds())
```

- `Debug` — per-event detail, off by default.
- `Info` — state transitions and lifecycle. Should read as a useful audit trail on its own.
- `Warn` — recovered problems (backend fallback taken, sync retry).
- `Error` — the operation failed and the user may lose something.

Never log secrets: no OAuth tokens, no refresh tokens, no ntfy topic, no LLM API key. When a config
struct is logged, it goes through its `LogValue()` which redacts those fields.

The daemon writes JSON to `~/.local/state/gwen/gwend.log` (rotated by lumberjack, 10 MB × 3) and
human-readable text to stderr when running in the foreground.

## Database access

`database/sql` with hand-written SQL in the repository layer. No ORM, no query builder, no codegen.

- Every repository method takes `ctx context.Context` first.
- Every multi-statement write goes through a transaction helper; no bare multi-write sequences.
- SQL lives in Go string constants next to the method that runs it, named `qInsertSegment` etc.
- Always use bound parameters. String-concatenated SQL is never acceptable.
- Repositories return domain structs from `internal/model`, never `*sql.Rows`.

## Testing

Stdlib `testing` plus `stretchr/testify/require`. Prefer `require` over `assert` — a failed
assertion should stop the test rather than cascade.

Table-driven tests are the default. Each case gets a `name` and is run under `t.Run`.

Tests must be deterministic and parallel-safe: fake clock, temp-file database per test, no network,
no sleeping. Full strategy and the required scenarios are in [11-testing.md](11-testing.md).

## Commits

Conventional commits, with the work package as scope:

```text
feat(W1): retroactive reclaim on hard idle threshold

The open work segment is now truncated back to idle_since rather than
to the moment the threshold fired, so a long absence does not bank the
grace period as work.
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `build`, `chore`. Subject in the imperative, under
72 characters. Body explains *why* when it is not obvious; the diff already shows what.

One work package per branch, named `w1-time-engine`, `w4-notifications`, and so on.
