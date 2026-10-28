# Plan (v3): sched as disk write-time scheduler

## 1. The model

The unit of control is the writer (whoever writes to disk), not the queue:

| Writer | Writes to | Priority (subsystem constant) |
|---|---|---|
| Cache flush (`requests.cache`, the `go func()` in `persistRequests`) | `requests.cache` | 0 |
| WAL writer (`requests`) | `requests` | 1 |
| FilesWriter (future) | files | 2 |

Cycle of each writer: ask for a turn → write the full session (batch → flush → syncs → checkpoint) → report completion + latency → release the turn.

## 2. Saturation detection (reference threshold)

Same as v2: completion latency > threshold → high pressure on that source (level 0/1, hysteresis: it clears when the completion comes back under threshold). Secondary signal: in-flight write data vs `LNND_MAX_MEM`. ⚠️ Still open whether the reference threshold is latency only or also `LNND_MAX_MEM`.

## 3. Write-time assignment (corrected point)

Two modes in sched, depending on the saturation state:

```
No saturation → the writer asks and writes (the turn only provides mutual exclusion)
Saturation    → sched SHARES OUT the write time between subsystems:
                each writer receives a write-time budget
                per window, proportional to its priority
```

- Proposed mechanism: budget per window (token bucket). Each window (e.g. 100 ms) divides the available write time according to the weights derived from the priority constants (cache : WAL : files ≈ 3 : 2 : 1 — configurable proportion). A writer only starts a session if it has budget left; if not, `WaitWriteTurn` blocks it until the next window.
- Sessions in progress are not preempted (an fsync cannot be canceled): the budget is debited at the end by the actually reported duration; if the session overshoots the window, it is paid back in the next one.
- Priority = proportion of time, not exclusion: the highest-priority writer receives more disk time; the lowest-priority one also receives it, in its proportion. "Which writers write and which do not" is resolved per instant: whoever has no budget does not write — until its next assignment.
- Self-correction: if a writer is idle, its budget is not consumed; the saturation signal (latency) drops on its own and budgets stop binding. No manual redistribution needed.
- The `IsBackPressured` cascade stops being the arbitration mechanism; the priority constants are reused as the weights of the share.

## 4. Anti-starvation (two layers)

1. Structural: the per-window share guarantees a minimum quota to all registered writers — none can stay at zero.
2. Dynamic (aging): a writer blocked longer than `LNND_ANTI_STARVATION_MS` → raises its weight for the next window(s) (and/or a direct pass), until it unblocks. The weight returns to its base value once the writer completes a session.

## 5. Changes per file

| File | Change |
|---|---|
| `internal/sched/service.go` | Turn arbiter: `WaitWriteTurn` / `EndWriteTurn` / `ReportCompletion`; saturation state; per-writer budgets with per-window refill (ticker or lazy); aging with `blockedSince`; `Stop()` with broadcast |
| `internal/sched/types.go` | Thresholds (latency, in-flight bytes), window, weights per `BPSourceID` |
| `internal/store/requests_persistence.go` | Turn before writing; measurement and reporting of per-session latency; remove the internal `for s.IsBackPressured(...)` loop; the cache flush goes through the gate with `BPRequestsCacheID` |
| `internal/store/requests.go` | `Run` uses a turn per batch |
| `SubmitRequest` | The pre-enqueue `WaitForPressureRelease` is removed (producers take no part: they discard on full queue or hold in RAM). Consequence: prolonged saturation → `ErrWriteTimeout` at 10 s |
| `internal/queues/*` | No changes |

## 6. Configuration

| Variable | Role |
|---|---|
| `LNND_MAX_MEM` | In-flight write ceiling (secondary saturation signal) — pending confirmation |
| `LNND_WRITE_LATENCY_MS` (new) | Reference saturation threshold — to be calibrated |
| `LNND_SCHED_WINDOW_MS` (new) | Share window (default ~100 ms) |
| `LNND_ANTI_STARVATION_MS` (new) | The `X` of the aging bypass |

## 7. Tests

- Share: under simulated saturation, each writer receives its proportion of session time per window.
- Without saturation: budgets do not bind (throughput equal to today's).
- Saturation entry/exit via latency, with hysteresis.
- Aging: a writer blocked longer than X receives a weight boost and writes; weight returns to base.
- Invariant: a writer with pending work never waits for zero assigned time across N consecutive windows.
- `requests_recovery_test.go` untouched.

## 8. Risks

- Long sessions (batch + checkpoint + 3 syncs) can overshoot their window — the overshoot is accepted and debited afterwards; alternative: split the batch, later.
- Today only 2 real writers compete (cache and WAL); the share is barely noticeable until FilesWriter exists — but the mechanism remains generic from day one.
- Calibration of the latency threshold on a real disk (TODO.md already anticipates this).
