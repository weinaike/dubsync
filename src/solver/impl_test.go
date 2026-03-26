package solver

import (
	"testing"
	"time"

	timeline "github.com/wnk/timeline/src"
	"github.com/wnk/timeline/src/penalty"
)

func newTestSolver() timeline.Solver {
	return NewSolver(penalty.NewPenaltyCalculator())
}

func newTestConfig() timeline.DPConfig {
	return timeline.DPConfig{
		TimeStep:       10 * time.Millisecond,
		RateMin:        1.0,
		RateMax:        1.4,
		RateStep:       0.01,
		WindowLeft:     500 * time.Millisecond,
		WindowRight:    500 * time.Millisecond,
		SmoothDelta:    0.15,
		MinGap:         50 * time.Millisecond,
		BreakThreshold: 1500 * time.Millisecond,
	}
}

func newTestWeights() timeline.PenaltyWeights {
	return timeline.PenaltyWeights{
		WShift:  10.0,
		WSmooth: 5.0,
		WGap:    3.0,
		SpeedBandsAccel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 1.05, MarginalSlope: 1.0},
			{RateUpperBound: 1.15, MarginalSlope: 3.0},
			{RateUpperBound: 1.25, MarginalSlope: 8.0},
			{RateUpperBound: 1.40, MarginalSlope: 20.0},
		},
		SpeedBandsDecel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 0.95, MarginalSlope: 2.0},
			{RateUpperBound: 1.00, MarginalSlope: 1.0},
		},
	}
}

func TestSolveGreedy_EmptyInput(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{},
		TTSSegments:    []timeline.TTSSegment{},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Segments) != 0 {
		t.Errorf("Expected 0 segments, got %d", len(result.Segments))
	}
}

func TestSolveGreedy_SingleSegment(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 800 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(result.Segments))
	}

	// Should start at original position with R=1.0
	if result.Segments[0].StartTime != 0 {
		t.Errorf("Expected start time 0, got %v", result.Segments[0].StartTime)
	}
	if result.Segments[0].Rate != 1.0 {
		t.Errorf("Expected rate 1.0, got %f", result.Segments[0].Rate)
	}
}

func TestSolveGreedy_PerfectMatch(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// TTS audio fits exactly in original slots
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1200 * time.Millisecond, End: 2200 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1000 * time.Millisecond},
			{Index: 1, NaturalLen: 1000 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify no overlaps
	if result.Segments[0].EndTime() > result.Segments[1].StartTime {
		t.Error("Segments overlap!")
	}

	// Should use R=1.0 for all segments
	for i, seg := range result.Segments {
		if seg.Rate != 1.0 {
			t.Errorf("Segment %d: Expected rate 1.0, got %f", i, seg.Rate)
		}
	}
}

func TestSolveGreedy_SingleSegmentExtends(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Single segment with TTS longer than original slot
	// Since it's the last (and only) segment, Rate=1.0 is optimal (no downstream constraint)
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1200 * time.Millisecond}, // 20% longer
		},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Single segment can extend beyond original slot - Rate=1.0 is optimal
	if result.Segments[0].Rate != 1.0 {
		t.Errorf("Expected rate 1.0 for last segment, got %f", result.Segments[0].Rate)
	}
	// Should start at window min (0)
	if result.Segments[0].StartTime != 0 {
		t.Errorf("Expected StartTime 0, got %v", result.Segments[0].StartTime)
	}
	// Duration should match TTS natural length
	if result.Segments[0].PlayDuration != 1200*time.Millisecond {
		t.Errorf("Expected PlayDuration 1200ms, got %v", result.Segments[0].PlayDuration)
	}
}

func TestSolveGreedy_NeedsSpeedChange(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Dense scenario: first segment must compress to fit before next segment's window
	// Segment 0: slot 0-1000ms, TTS=1200ms
	// Segment 1: slot 1100-2100ms, TTS=1000ms
	// Segment 0 can only use 1100-500-50 = 550ms max (due to segment 1's window)
	// So segment 0 must compress: rate = 1200/550 = 2.18, capped at RateMax=1.3
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1100 * time.Millisecond, End: 2100 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1200 * time.Millisecond}, // 20% longer, needs compression
			{Index: 1, NaturalLen: 1000 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Segment 0 needs compression because segment 1's window restricts available time
	// Segment 1 can start at earliest: 1100 - 500 = 600ms
	// So segment 0 must end by: 600 - 50 = 550ms
	// Available time for segment 0: 550ms
	// TTS: 1200ms, so rate = 1200/550 = 2.18, but capped at RateMax=1.3
	if result.Segments[0].Rate <= 1.0 {
		t.Errorf("Expected rate > 1.0 for segment 0, got %f", result.Segments[0].Rate)
	}
}

func TestSolveGreedy_GuaranteeFeasible(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Dense packing scenario
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 500 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 550 * time.Millisecond, End: 1050 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 1100 * time.Millisecond, End: 1600 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 600 * time.Millisecond}, // Slightly longer
			{Index: 1, NaturalLen: 600 * time.Millisecond},
			{Index: 2, NaturalLen: 600 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify no overlaps
	for i := 1; i < len(result.Segments); i++ {
		prevEnd := result.Segments[i-1].EndTime()
		currStart := result.Segments[i].StartTime
		if prevEnd > currStart {
			t.Errorf("Segments %d and %d overlap: prevEnd=%v, currStart=%v",
				i-1, i, prevEnd, currStart)
		}
		gap := currStart - prevEnd
		if gap < cfg.MinGap {
			t.Errorf("Gap between segments %d and %d is %v, less than MinGap %v",
				i-1, i, gap, cfg.MinGap)
		}
	}
}

func TestSolveGreedy_RespectsConstraints(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 500 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 600 * time.Millisecond, End: 1100 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 500 * time.Millisecond},
			{Index: 1, NaturalLen: 500 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify no overlaps
	if result.Segments[0].EndTime() > result.Segments[1].StartTime {
		t.Error("Segments overlap!")
	}

	// Verify rate within bounds
	for i, seg := range result.Segments {
		if seg.Rate < cfg.RateMin || seg.Rate > cfg.RateMax {
			t.Errorf("Segment %d: rate %f out of bounds [%f, %f]",
				i, seg.Rate, cfg.RateMin, cfg.RateMax)
		}
	}

	// Verify smooth constraint
	if cfg.SmoothDelta > 0 {
		rateDiff := result.Segments[1].Rate - result.Segments[0].Rate
		if rateDiff < 0 {
			rateDiff = -rateDiff
		}
		if rateDiff > cfg.SmoothDelta {
			t.Errorf("Rate diff %f exceeds SmoothDelta %f", rateDiff, cfg.SmoothDelta)
		}
	}
}
