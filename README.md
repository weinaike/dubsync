# Dubsync

[![Go Reference](https://pkg.go.dev/badge/github.com/weinaike/dubsync.svg)](https://pkg.go.dev/github.com/weinaike/dubsync)
[![Go Report Card](https://goreportcard.com/badge/github.com/weinaike/dubsync)](https://goreportcard.com/report/github.com/weinaike/dubsync)

A Go library for **audio translation timeline scheduling** - schedules TTS-generated translated audio segments onto an original timeline while balancing audio compression/stretching with temporal displacement.

## Overview

When translating audio content (e.g., video dubbing, podcast translation), the translated speech often has different duration than the original. This library solves the problem of how to fit translated audio segments into the original timeline by:

1. **Shifting** segments to optimal positions
2. **Time-stretching** (compressing/expanding) audio when needed
3. **Balancing** between synchronization accuracy and natural listening experience

The algorithm uses a **greedy approach** that is mathematically proven optimal for this chain-constrained optimization problem.

## Features

- **Two Business Modes**:
  - **ModeA (Video)**: Prioritizes lip-sync accuracy - "compress rather than shift"
  - **ModeB (Podcast)**: Prioritizes natural listening - "shift rather than compress"

- **Optimal Greedy Algorithm**: O(N) time complexity, proven optimal for chain-constrained problems

- **Flexible Penalty System**: Configurable weights for shift, speed, smoothness, and gap penalties

- **Degradation Handling**: Three-tier fallback strategy when constraints cannot be satisfied

- **Quality Alerts**: Detailed diagnostics for segments requiring degradation

## Installation

```bash
go get github.com/weinaike/dubsync
```

## Quick Start

```go
package main

import (
    "fmt"
    "time"

    timeline "github.com/weinaike/dubsync/src"
    "github.com/weinaike/dubsync/src/scheduler"
)

func main() {
    // Create a scheduler
    sched := scheduler.NewScheduler()

    // Define original audio segments (from VAD detection)
    sourceSegments := []timeline.SourceSegment{
        {Index: 0, Interval: timeline.TimeInterval{Start: 0 * time.Second, End: 2 * time.Second}},
        {Index: 1, Interval: timeline.TimeInterval{Start: 2.5 * time.Second, End: 5 * time.Second}},
        {Index: 2, Interval: timeline.TimeInterval{Start: 5.5 * time.Second, End: 8 * time.Second}},
    }

    // Define TTS-generated translated segments
    ttsSegments := []timeline.TTSSegment{
        {Index: 0, NaturalLen: 1800 * time.Millisecond}, // Translated audio is shorter
        {Index: 1, NaturalLen: 2800 * time.Millisecond}, // Slightly longer
        {Index: 2, NaturalLen: 2200 * time.Millisecond}, // Much shorter
    }

    // Create configuration for video mode
    config := timeline.NewDefaultConfig(timeline.ModeA)
    config.TotalDuration = 10 * time.Second

    // Build input and schedule
    input := &timeline.PipelineInput{
        SourceSegments: sourceSegments,
        TTSSegments:    ttsSegments,
        Config:         config,
    }

    blueprint, err := sched.Schedule(input)
    if err != nil {
        panic(err)
    }

    // Output the scheduling result
    fmt.Printf("Status: %s\n", blueprint.Status)
    for _, seg := range blueprint.Segments {
        fmt.Printf("Segment %d: Start=%v, Rate=%.2f, Duration=%v\n",
            seg.Index, seg.StartTime, seg.Rate, seg.PlayDuration)
    }
}
```

## API Reference

### Core Types

#### Input Types

```go
// Original audio segment from VAD detection
type SourceSegment struct {
    Index    int          // Segment index
    Interval TimeInterval // Original timing [Start, End)
}

// TTS-generated translated segment
type TTSSegment struct {
    Index        int      // Segment index (must match SourceSegment)
    NaturalLen   Duration // Natural TTS audio length
    AudioFileRef string   // Reference to audio file
}
```

#### Output Types

```go
// Complete scheduling blueprint
type Blueprint struct {
    Status             ScheduleStatus   // "ok", "degraded", or "failed"
    Segments           []SegmentPlan    // All segment scheduling results
    InfeasibleSegments []InfeasibleInfo // Problem segments (if any)
    DegradationLevel   int              // 0-3 severity
    Recommendation     string           // Suggested actions
}

// Individual segment result
type SegmentPlan struct {
    Index        int        // Segment index
    StartTime    Duration   // Scheduled start time
    Rate         float64    // Playback rate (1.0 = normal, 1.2 = 20% faster)
    PlayDuration Duration   // Actual playback duration
    Degraded     bool       // Whether degradation was applied
    QualityAlert *QualityAlert // Quality warning (if any)
}
```

### Configuration

```go
// Create default configuration for a mode
config := timeline.NewDefaultConfig(timeline.ModeA)

// Customize weights
config.Weights.WShift = 15.0  // Increase shift penalty
config.Weights.WSmooth = 8.0  // Increase smoothness penalty

// Customize DP parameters
config.DP.RateMax = 1.3       // Maximum allowed speed
config.DP.WindowRight = 300 * time.Millisecond // Narrow search window

// Set total duration
config.TotalDuration = 60 * time.Second
```

### Penalty Weights

| Weight | ModeA (Video) | ModeB (Podcast) | Description |
|--------|---------------|-----------------|-------------|
| `WShift` | 10.0/sec | 0.5/sec | Penalty for temporal displacement |
| `WSmooth` | 5.0 | 2.0 | Penalty for rate changes between segments |
| `WGap` | 1.0 | 3.0 | Penalty for gap deviation from original |

### Speed Penalty Bands

Speed penalties are piecewise linear, allowing fine-grained control:

```go
// ModeA speed bands (acceleration)
SpeedBandsAccel: []SpeedPenaltyBand{
    {RateUpperBound: 1.05, MarginalSlope: 1.0},   // Barely noticeable
    {RateUpperBound: 1.15, MarginalSlope: 3.0},   // Acceptable
    {RateUpperBound: 1.25, MarginalSlope: 8.0},   // Noticeable
    {RateUpperBound: 1.40, MarginalSlope: 20.0},  // Emergency only
}
```

## Business Modes

### ModeA (Video Dubbing)

**Philosophy**: "Compress rather than shift"

- High shift penalty (10.0/sec) - strict lip-sync
- Narrow search window (±500ms)
- Lower speed penalties - compression acceptable
- No slowdown allowed (RateMin = 1.0)

Use for: Video dubbing, content where lip-sync is critical

### ModeB (Podcast/Audiobook)

**Philosophy**: "Shift rather than compress"

- Low shift penalty (0.5/sec) - timing flexibility
- Wide right window (5000ms) - can push segments far
- High speed penalties - natural speed preserved
- Slowdown allowed (RateMin = 0.95)

Use for: Podcasts, audiobooks, any audio-only translation

## Algorithm

The library uses a **greedy algorithm** that is mathematically optimal for this problem due to:

1. **Chain Constraints**: Each segment's position only depends on the previous segment
2. **Convex Penalty Functions**: Local optimum equals global optimum
3. **Time Complexity**: O(N) - processes each segment once

The algorithm:
1. Tries ideal placement: `StartTime = OriginalStart, Rate = 1.0`
2. If overlap detected, shifts right to minimum valid position
3. Calculates required rate to fit available time slot
4. Applies rate smoothing if configured
5. Continues to next segment

### Objective Function

Minimizes the weighted sum of:
- **Shift Penalty**: Temporal displacement from original position
- **Speed Penalty**: Deviation from natural playback rate
- **Smooth Penalty**: Rate changes between adjacent segments
- **Gap Penalty**: Deviation from original inter-segment gaps

For full algorithm specification, see [docs/algorithm.md](docs/algorithm.md).

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./src/solver/...
```

## Project Structure

```
timeline/
├── src/
│   ├── types.go          # Core data types
│   ├── config.go         # Configuration factory
│   ├── scheduler.go      # Scheduler interface
│   ├── solver.go         # Solver interface
│   ├── penalty.go        # Penalty calculator interface
│   ├── degradation.go    # Degradation handler interface
│   ├── errors.go         # Error definitions
│   ├── scheduler/        # Scheduler implementation
│   ├── solver/           # Greedy algorithm implementation
│   ├── penalty/          # Penalty calculation
│   ├── degradation/      # Fallback strategies
│   └── testutil/         # Testing utilities
└── docs/
    └── algorithm.md      # Full algorithm specification
```

## License

MIT License
