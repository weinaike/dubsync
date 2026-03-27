# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Installation

```bash
go get github.com/weinaike/dubsync
```

## Build and Test Commands

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./src/solver/...
go test ./src/scheduler/...

# Run a single test
go test -run TestSolveGreedy_PerfectMatch ./src/solver/...

# Run tests with verbose output
go test -v ./...

# Run benchmarks
go test -bench=. ./src/solver/...
```

## Architecture Overview

This is an **audio translation timeline scheduling system** that schedules TTS-generated translated audio segments onto an original timeline. The core challenge is balancing audio compression/stretching with temporal displacement.

### Pipeline Flow

```
Input (SourceSegments + TTSSegments)
    → Solver (Greedy algorithm - optimal for chain-constrained problems)
    → DegradationHandler (fallback if infeasible)
    → Blueprint (scheduling result)
```

### Core Components

| Component | Interface | Implementation | Purpose |
|-----------|-----------|----------------|---------|
| **Scheduler** | `src/scheduler.go` | `scheduler/impl.go` | Orchestrates the full pipeline |
| **Solver** | `src/solver.go` | `solver/impl.go` | Greedy algorithm (optimal for chain-constrained problems) |
| **PenaltyCalculator** | `src/penalty.go` | `penalty/impl.go` | Computes objective function penalties |
| **DegradationHandler** | `src/degradation.go` | `degradation/impl.go` | Three-tier fallback strategy |

### Two Business Modes

| | ModeA (Video) | ModeB (Podcast) |
|---|---|---|
| **Priority** | Lip-sync accuracy | Natural listening experience |
| **Philosophy** | "Compress rather than shift" | "Shift rather than compress" |
| **W_shift** | 10.0/sec | 0.5/sec |
| **WindowRight** | 500ms | 5000ms |
| **RateMin** | 1.0 (no slowdown) | 0.95 (allows slowdown) |
| **Speed penalty slopes** | Lower (compression acceptable) | Higher (compression avoided) |

## Algorithm Details

### Solver Two-Phase Strategy

**Phase 1 - Greedy (O(N)):**
1. Try ideal placement: `S[i] = OrigStart`, `R[i] = 1.0`
2. If overlap, shift right to `max(currentTime + MinGap, windowMin)`
3. Calculate required rate to fit TTS audio
4. Guarantees feasible solution (no overlaps)

**Phase 2 - DP Optimization:**
- State: `dp(i, s, r)` = min penalty for first i segments with segment i at time s with rate r
- Time discretized to 10ms steps
- Rate candidates: `[RateMin, RateMax]` with 0.01 step
- Pruning: upperBound pruning + TopK per time step
- Backtrack to reconstruct solution

### Penalty Functions

| Penalty | Formula |
|---------|---------|
| Shift | `W_shift * |shift| / 1000` (seconds) |
| Speed | Piecewise linear based on `SpeedBandsAccel/Decel` |
| Smooth | `W_smooth * |rate1 - rate2|` |
| Gap | `W_gap * |actualGap - originalGap| / 1000` |

### Degradation Tiers

1. **Constraint Relaxation**: Widen windows, increase RateMax to 1.5
2. **TTS Negotiation**: Request shorter audio from upstream pipeline
3. **Hard Fallback**: Force placement with quality alerts

## Key Types

```go
// Core input
type SourceSegment struct { Index, Interval }  // Original audio timing
type TTSSegment struct { Index, NaturalLen }    // Translated audio length

// Core output
type Blueprint struct { Status, Segments, InfeasibleSegments }
type SegmentPlan struct { Index, StartTime, Rate, PlayDuration }
```

## Test Utilities

Use `testutil.TestDataBuilder` for constructing test scenarios:

```go
input := testutil.NewTestDataBuilder().
    ModeA().
    AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 2200*time.Millisecond).
    AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2300*time.Millisecond).
    Build()
```

Pre-built scenarios: `PerfectMatch()`, `NeedSpeedChange()`, `NeedShift()`, `DominoPush()`, `Infeasible()`, `WithBreakPoints()`

## Full Algorithm Specification

See `docs/algorithm.md` for the complete mathematical formulation and algorithm design rationale.

## Module

```
github.com/weinaike/dubsync
```
