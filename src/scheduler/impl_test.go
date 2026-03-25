package scheduler

import (
	"testing"
	"time"

	timeline "github.com/wnk/timeline/src"
)

func newTestSchedulerConfig(mode timeline.Mode) timeline.SchedulerConfig {
	return timeline.SchedulerConfig{
		Mode:           mode,
		Weights:        timeline.NewDefaultPenaltyWeights(mode),
		DP:             timeline.NewDefaultDPConfig(mode),
		TotalDuration:  0,
		MaxNegotiation: 2,
	}
}

func TestSchedule_EmptyInput(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{},
		TTSSegments:    []timeline.TTSSegment{},
		Config:         cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Segments) != 0 {
		t.Errorf("Expected 0 segments, got %d", len(result.Segments))
	}
	if result.Status != timeline.StatusOK {
		t.Errorf("Expected status OK, got %s", result.Status)
	}
}

func TestSchedule_SingleSegment(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 800 * time.Millisecond},
		},
		Config: cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(result.Segments))
	}
	if result.Status != timeline.StatusOK {
		t.Errorf("Expected status OK, got %s", result.Status)
	}
}

func TestSchedule_PerfectMatch(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	// TTS audio fits perfectly in original slots
	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1200 * time.Millisecond, End: 2200 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1000 * time.Millisecond},
			{Index: 1, NaturalLen: 1000 * time.Millisecond},
		},
		Config: cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify no overlaps
	for i := 1; i < len(result.Segments); i++ {
		if result.Segments[i-1].EndTime() > result.Segments[i].StartTime {
			t.Errorf("Segments %d and %d overlap", i-1, i)
		}
	}

	// Should use R=1.0 for all
	for i, seg := range result.Segments {
		if seg.Rate != 1.0 {
			t.Errorf("Segment %d: Expected rate 1.0, got %f", i, seg.Rate)
		}
	}
}

func TestSchedule_SegmentMismatch(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 800 * time.Millisecond},
			{Index: 1, NaturalLen: 900 * time.Millisecond}, // Extra segment
		},
		Config: cfg,
	}

	_, err := s.Schedule(input)
	if err == nil {
		t.Error("Expected error for segment mismatch")
	}
}

func TestSchedule_SubProblemDecomposition(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	// Create segments with a 2s gap between them (breakpoint)
	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 3000 * time.Millisecond, End: 4000 * time.Millisecond}}, // 2s gap
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 800 * time.Millisecond},
			{Index: 1, NaturalLen: 800 * time.Millisecond},
		},
		Config: cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result.Segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(result.Segments))
	}
}

func TestSchedule_ModeA_vs_ModeB(t *testing.T) {
	// Create input that would benefit from different modes
	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 500 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 550 * time.Millisecond, End: 1050 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 600 * time.Millisecond}, // Longer than original
			{Index: 1, NaturalLen: 600 * time.Millisecond},
		},
	}

	// Test Mode A (video mode - prefers compression over shift)
	sA := NewScheduler()
	cfgA := newTestSchedulerConfig(timeline.ModeA)
	input.Config = cfgA
	resultA, err := sA.Schedule(input)
	if err != nil {
		t.Fatalf("Mode A error: %v", err)
	}

	// Test Mode B (podcast mode - prefers shift over compression)
	sB := NewScheduler()
	cfgB := newTestSchedulerConfig(timeline.ModeB)
	input.Config = cfgB
	resultB, err := sB.Schedule(input)
	if err != nil {
		t.Fatalf("Mode B error: %v", err)
	}

	// Both should produce valid results without overlaps
	for i := 1; i < len(resultA.Segments); i++ {
		if resultA.Segments[i-1].EndTime() > resultA.Segments[i].StartTime {
			t.Errorf("Mode A: Segments %d and %d overlap", i-1, i)
		}
	}
	for i := 1; i < len(resultB.Segments); i++ {
		if resultB.Segments[i-1].EndTime() > resultB.Segments[i].StartTime {
			t.Errorf("Mode B: Segments %d and %d overlap", i-1, i)
		}
	}

	// Mode B should generally have lower max rate (prefers to shift rather than compress)
	// but this depends on the specific input and constraints
	t.Logf("Mode A max rate: %.2f", maxRate(resultA.Segments))
	t.Logf("Mode B max rate: %.2f", maxRate(resultB.Segments))
}

func maxRate(segments []timeline.SegmentPlan) float64 {
	max := 0.0
	for _, s := range segments {
		if s.Rate > max {
			max = s.Rate
		}
	}
	return max
}

func TestSchedule_NeedsCompression(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	// TTS audio is longer than original slot, with a second segment that forces compression
	// Without the second segment, a single segment can extend freely without compression
	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 500 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 550 * time.Millisecond, End: 1050 * time.Millisecond}}, // tight gap
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 700 * time.Millisecond}, // 40% longer than original slot
			{Index: 1, NaturalLen: 500 * time.Millisecond},
		},
		Config: cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// First segment should have some compression because it would otherwise overlap with segment 1
	if result.Segments[0].Rate <= 1.0 {
		t.Errorf("Expected rate > 1.0 for compression, got %f", result.Segments[0].Rate)
	}

	// Verify no overlaps
	for i := 1; i < len(result.Segments); i++ {
		if result.Segments[i-1].EndTime() > result.Segments[i].StartTime {
			t.Errorf("Segments %d and %d overlap", i-1, i)
		}
	}
}

func TestSchedule_IndexPreservation(t *testing.T) {
	s := NewScheduler()
	cfg := newTestSchedulerConfig(timeline.ModeA)

	// Create segments with non-zero global indices
	input := &timeline.PipelineInput{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1200 * time.Millisecond, End: 2200 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 800 * time.Millisecond},
			{Index: 1, NaturalLen: 900 * time.Millisecond},
		},
		Config: cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify indices are preserved
	for i, seg := range result.Segments {
		if seg.Index != i {
			t.Errorf("Segment %d has index %d, expected %d", i, seg.Index, i)
		}
	}
}
