package splitter

import (
	"testing"
	"time"

	timeline "github.com/wnk/timeline/src"
)

func TestSplitter_EmptyInput(t *testing.T) {
	s := NewSplitter()
	result := s.Split([]timeline.SourceSegment{}, []timeline.TTSSegment{}, 1500*time.Millisecond)

	if len(result) != 0 {
		t.Errorf("Expected 0 sub-problems for empty input, got %d", len(result))
	}
}

func TestSplitter_SingleSegment(t *testing.T) {
	s := NewSplitter()
	sources := []timeline.SourceSegment{
		{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
	}
	tts := []timeline.TTSSegment{
		{Index: 0, NaturalLen: 800 * time.Millisecond},
	}

	result := s.Split(sources, tts, 1500*time.Millisecond)

	if len(result) != 1 {
		t.Errorf("Expected 1 sub-problem, got %d", len(result))
	}
	if len(result[0].SourceSegments) != 1 {
		t.Errorf("Expected 1 source segment in sub-problem, got %d", len(result[0].SourceSegments))
	}
}

func TestSplitter_NoBreakpoints(t *testing.T) {
	s := NewSplitter()
	// Segments with 200ms gaps between them (less than 1.5s threshold)
	sources := []timeline.SourceSegment{
		{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		{Index: 1, Interval: timeline.TimeInterval{Start: 1200 * time.Millisecond, End: 2200 * time.Millisecond}},
		{Index: 2, Interval: timeline.TimeInterval{Start: 2400 * time.Millisecond, End: 3400 * time.Millisecond}},
	}
	tts := []timeline.TTSSegment{
		{Index: 0, NaturalLen: 800 * time.Millisecond},
		{Index: 1, NaturalLen: 900 * time.Millisecond},
		{Index: 2, NaturalLen: 850 * time.Millisecond},
	}

	result := s.Split(sources, tts, 1500*time.Millisecond)

	if len(result) != 1 {
		t.Errorf("Expected 1 sub-problem (no breakpoints), got %d", len(result))
	}
	if len(result[0].SourceSegments) != 3 {
		t.Errorf("Expected 3 source segments in sub-problem, got %d", len(result[0].SourceSegments))
	}
}

func TestSplitter_MultipleBreakpoints(t *testing.T) {
	s := NewSplitter()
	// Segments with 2s gaps at positions 1 and 3 (breakpoints)
	sources := []timeline.SourceSegment{
		{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},      // gap after: 2000ms
		{Index: 1, Interval: timeline.TimeInterval{Start: 3000 * time.Millisecond, End: 4000 * time.Millisecond}}, // gap after: 500ms
		{Index: 2, Interval: timeline.TimeInterval{Start: 4500 * time.Millisecond, End: 5500 * time.Millisecond}}, // gap after: 2000ms
		{Index: 3, Interval: timeline.TimeInterval{Start: 7500 * time.Millisecond, End: 8500 * time.Millisecond}},
	}
	tts := []timeline.TTSSegment{
		{Index: 0, NaturalLen: 800 * time.Millisecond},
		{Index: 1, NaturalLen: 900 * time.Millisecond},
		{Index: 2, NaturalLen: 850 * time.Millisecond},
		{Index: 3, NaturalLen: 800 * time.Millisecond},
	}

	result := s.Split(sources, tts, 1500*time.Millisecond)

	if len(result) != 3 {
		t.Errorf("Expected 3 sub-problems, got %d", len(result))
	}

	// First sub-problem: segment 0
	if len(result[0].SourceSegments) != 1 {
		t.Errorf("Expected 1 segment in first sub-problem, got %d", len(result[0].SourceSegments))
	}

	// Second sub-problem: segments 1-2
	if len(result[1].SourceSegments) != 2 {
		t.Errorf("Expected 2 segments in second sub-problem, got %d", len(result[1].SourceSegments))
	}

	// Third sub-problem: segment 3
	if len(result[2].SourceSegments) != 1 {
		t.Errorf("Expected 1 segment in third sub-problem, got %d", len(result[2].SourceSegments))
	}
}

func TestSplitter_AllBreakpoints(t *testing.T) {
	s := NewSplitter()
	// Each segment separated by 2s gap
	sources := []timeline.SourceSegment{
		{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		{Index: 1, Interval: timeline.TimeInterval{Start: 3000 * time.Millisecond, End: 4000 * time.Millisecond}},
		{Index: 2, Interval: timeline.TimeInterval{Start: 6000 * time.Millisecond, End: 7000 * time.Millisecond}},
	}
	tts := []timeline.TTSSegment{
		{Index: 0, NaturalLen: 800 * time.Millisecond},
		{Index: 1, NaturalLen: 900 * time.Millisecond},
		{Index: 2, NaturalLen: 850 * time.Millisecond},
	}

	result := s.Split(sources, tts, 1500*time.Millisecond)

	if len(result) != 3 {
		t.Errorf("Expected 3 sub-problems (all breakpoints), got %d", len(result))
	}
}
