package solver

import (
	"math"
	"testing"
	"time"

	timeline "github.com/wnk/timeline/src"
	"github.com/wnk/timeline/src/penalty"
)

func newSmoothTestSolver() *solverImpl {
	return &solverImpl{
		penaltyCalc: penalty.NewPenaltyCalculator(),
	}
}

// TestSolveGreedySmooth_EmptyInput verifies smooth solver handles empty input
func TestSolveGreedySmooth_EmptyInput(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{},
		TTSSegments:    []timeline.TTSSegment{},
	}

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Segments) != 0 {
		t.Errorf("Expected 0 segments, got %d", len(result.Segments))
	}
}

// TestSolveGreedySmooth_SingleSegment verifies no crash on single segment
func TestSolveGreedySmooth_SingleSegment(t *testing.T) {
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

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(result.Segments))
	}
	if result.Segments[0].Rate != 1.0 {
		t.Errorf("Expected rate 1.0, got %f", result.Segments[0].Rate)
	}
}

// TestSolveGreedySmooth_ReducesRateVariance verifies that smooth solver
// produces less rate variance than plain greedy when compression is needed
func TestSolveGreedySmooth_ReducesRateVariance(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Scenario: 5 consecutive segments where the middle one needs significant compression.
	// Plain greedy will create a spike at segment 2 (rate=1.3+), while smooth
	// should distribute the compression more evenly.
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1050 * time.Millisecond, End: 2050 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 2100 * time.Millisecond, End: 3100 * time.Millisecond}},
			{Index: 3, Interval: timeline.TimeInterval{Start: 3150 * time.Millisecond, End: 4150 * time.Millisecond}},
			{Index: 4, Interval: timeline.TimeInterval{Start: 4200 * time.Millisecond, End: 5200 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1050 * time.Millisecond}, // Slightly over
			{Index: 1, NaturalLen: 1200 * time.Millisecond}, // Needs compression
			{Index: 2, NaturalLen: 1300 * time.Millisecond}, // Needs more compression
			{Index: 3, NaturalLen: 1200 * time.Millisecond}, // Needs compression
			{Index: 4, NaturalLen: 1050 * time.Millisecond}, // Slightly over
		},
	}

	// Get plain greedy result
	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}

	// Get smooth result
	smoothResult, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Smooth error: %v", err)
	}

	// Calculate rate variance for both
	greedyVar := rateVariance(greedyResult.Segments)
	smoothVar := rateVariance(smoothResult.Segments)

	t.Logf("Greedy rates: %v (variance: %.6f)", extractRates(greedyResult.Segments), greedyVar)
	t.Logf("Smooth rates: %v (variance: %.6f)", extractRates(smoothResult.Segments), smoothVar)

	// Smooth result should have less or equal variance
	if smoothVar > greedyVar+1e-6 {
		t.Errorf("Smooth variance (%.6f) should be <= greedy variance (%.6f)", smoothVar, greedyVar)
	}

	// Both results must be feasible (no overlaps, rates in bounds)
	verifyFeasibility(t, smoothResult.Segments, &cfg)
}

// TestSolveGreedySmooth_PenaltyImproves verifies that smooth solver produces
// lower or equal total penalty compared to plain greedy
func TestSolveGreedySmooth_PenaltyImproves(t *testing.T) {
	s := newTestSolver()
	pc := penalty.NewPenaltyCalculator()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Scenario: Dense packing where greedy creates a "domino push" with
	// varying rates — smooth should reduce total penalty
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 800 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 850 * time.Millisecond, End: 1650 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 1700 * time.Millisecond, End: 2500 * time.Millisecond}},
			{Index: 3, Interval: timeline.TimeInterval{Start: 2550 * time.Millisecond, End: 3350 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 900 * time.Millisecond},
			{Index: 1, NaturalLen: 950 * time.Millisecond},
			{Index: 2, NaturalLen: 900 * time.Millisecond},
			{Index: 3, NaturalLen: 850 * time.Millisecond},
		},
	}

	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}

	smoothResult, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Smooth error: %v", err)
	}

	greedyPenalty := pc.CalculateTotal(sub.SourceSegments, greedyResult.Segments, &weights)
	smoothPenalty := pc.CalculateTotal(sub.SourceSegments, smoothResult.Segments, &weights)

	t.Logf("Greedy penalty: %.4f  rates: %v", greedyPenalty, extractRates(greedyResult.Segments))
	t.Logf("Smooth penalty: %.4f  rates: %v", smoothPenalty, extractRates(smoothResult.Segments))

	// Smooth penalty should be <= greedy penalty (or very close)
	if smoothPenalty > greedyPenalty+0.01 {
		t.Errorf("Smooth penalty (%.4f) should be <= greedy penalty (%.4f)", smoothPenalty, greedyPenalty)
	}

	verifyFeasibility(t, smoothResult.Segments, &cfg)
}

// TestSolveGreedySmooth_MaintainsFeasibility verifies constraints are preserved
func TestSolveGreedySmooth_MaintainsFeasibility(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Stress test: many segments with tight packing
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 500 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 550 * time.Millisecond, End: 1050 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 1100 * time.Millisecond, End: 1600 * time.Millisecond}},
			{Index: 3, Interval: timeline.TimeInterval{Start: 1650 * time.Millisecond, End: 2150 * time.Millisecond}},
			{Index: 4, Interval: timeline.TimeInterval{Start: 2200 * time.Millisecond, End: 2700 * time.Millisecond}},
			{Index: 5, Interval: timeline.TimeInterval{Start: 2750 * time.Millisecond, End: 3250 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 600 * time.Millisecond},
			{Index: 1, NaturalLen: 650 * time.Millisecond},
			{Index: 2, NaturalLen: 700 * time.Millisecond},
			{Index: 3, NaturalLen: 550 * time.Millisecond},
			{Index: 4, NaturalLen: 600 * time.Millisecond},
			{Index: 5, NaturalLen: 500 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Logf("Smooth rates: %v", extractRates(result.Segments))

	verifyFeasibility(t, result.Segments, &cfg)
}

// TestSolveGreedySmooth_NoRegressionPerfectMatch verifies that when greedy
// already finds perfect solution, smooth doesn't make it worse
func TestSolveGreedySmooth_NoRegressionPerfectMatch(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	weights := newTestWeights()

	// Perfect match: all TTS fits exactly
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1200 * time.Millisecond, End: 2200 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 2400 * time.Millisecond, End: 3400 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 800 * time.Millisecond},
			{Index: 1, NaturalLen: 900 * time.Millisecond},
			{Index: 2, NaturalLen: 850 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for i, seg := range result.Segments {
		if seg.Rate != 1.0 {
			t.Errorf("Segment %d: expected rate 1.0, got %f", i, seg.Rate)
		}
	}
}

// TestSolveGreedySmooth_SmoothDeltaRespected verifies SmoothDelta constraint
func TestSolveGreedySmooth_SmoothDeltaRespected(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	cfg.SmoothDelta = 0.10 // Tighter constraint
	weights := newTestWeights()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 800 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 850 * time.Millisecond, End: 1650 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 1700 * time.Millisecond, End: 2500 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 900 * time.Millisecond},
			{Index: 1, NaturalLen: 1000 * time.Millisecond},
			{Index: 2, NaturalLen: 850 * time.Millisecond},
		},
	}

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Logf("Smooth rates: %v", extractRates(result.Segments))

	// Verify SmoothDelta constraint
	for i := 1; i < len(result.Segments); i++ {
		diff := math.Abs(result.Segments[i].Rate - result.Segments[i-1].Rate)
		if diff > cfg.SmoothDelta+1e-6 {
			t.Errorf("Rate diff between segments %d and %d is %.4f, exceeds SmoothDelta %.4f",
				i-1, i, diff, cfg.SmoothDelta)
		}
	}

	verifyFeasibility(t, result.Segments, &cfg)
}

// TestSolveGreedySmooth_ModeB_WideWindow verifies smooth works with Mode B's wide window
func TestSolveGreedySmooth_ModeB_WideWindow(t *testing.T) {
	s := newTestSolver()
	cfg := timeline.NewDefaultDPConfig(timeline.ModeB)
	weights := timeline.NewDefaultPenaltyWeights(timeline.ModeB)

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 2000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 2100 * time.Millisecond, End: 4100 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 4200 * time.Millisecond, End: 6200 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 2500 * time.Millisecond}, // Much longer than slot
			{Index: 1, NaturalLen: 2300 * time.Millisecond}, // Longer than slot
			{Index: 2, NaturalLen: 2000 * time.Millisecond}, // Fits
		},
	}

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Logf("Mode B smooth rates: %v", extractRates(result.Segments))

	// In Mode B, should prefer shifting over compression
	for i, seg := range result.Segments {
		if seg.Rate > 1.10 {
			t.Logf("Warning: Segment %d has high rate %.2f in Mode B (prefer shift)", i, seg.Rate)
		}
	}

	verifyFeasibility(t, result.Segments, &cfg)
}

// TestEqualizeRates_ConsecutiveAcceleratedSegments tests rate equalization
func TestEqualizeRates_ConsecutiveAcceleratedSegments(t *testing.T) {
	s := newSmoothTestSolver()
	cfg := newTestConfig()

	// Manually create plans with uneven rates in consecutive accelerated segments
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1050 * time.Millisecond, End: 2050 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 2100 * time.Millisecond, End: 3100 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1200 * time.Millisecond}, // Needs 1.2x
			{Index: 1, NaturalLen: 1300 * time.Millisecond}, // Needs 1.3x
			{Index: 2, NaturalLen: 1100 * time.Millisecond}, // Needs 1.1x
		},
	}

	// Create initial greedy plans
	weights := newTestWeights()
	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}

	t.Logf("Before equalize: %v", extractRates(greedyResult.Segments))

	// Apply equalization
	equalized := s.equalizeRates(greedyResult.Segments, sub, &cfg)

	t.Logf("After equalize:  %v", extractRates(equalized))

	// The max rate should decrease (or stay the same) after equalization
	maxBefore := maxRate(greedyResult.Segments)
	maxAfter := maxRate(equalized)

	if maxAfter > maxBefore+0.01 {
		t.Errorf("Equalization increased max rate: before=%.4f, after=%.4f", maxBefore, maxAfter)
	}
}

// TestLocalPenalty verifies local penalty calculation
func TestLocalPenalty(t *testing.T) {
	s := newSmoothTestSolver()
	weights := newTestWeights()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1100 * time.Millisecond, End: 2100 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1000 * time.Millisecond},
			{Index: 1, NaturalLen: 1000 * time.Millisecond},
		},
	}

	plans := []timeline.SegmentPlan{
		{Index: 0, StartTime: 0, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
		{Index: 1, StartTime: 1100 * time.Millisecond, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
	}

	// With rate=1.0 and original positions, penalty should be minimal (just shift=0)
	p0 := s.localPenalty(plans, 0, sub, &weights)
	p1 := s.localPenalty(plans, 1, sub, &weights)

	t.Logf("Local penalty seg 0: %.4f", p0)
	t.Logf("Local penalty seg 1: %.4f", p1)

	// Penalty should be 0 or very small for perfect placement
	if p0 > 0.1 {
		t.Errorf("Expected near-zero penalty for perfect placement, got %.4f", p0)
	}

	// Now test with a rate change
	plans[1].Rate = 1.2
	naturalLen := float64(1000 * time.Millisecond)
	plans[1].PlayDuration = time.Duration(naturalLen / 1.2)
	p1High := s.localPenalty(plans, 1, sub, &weights)

	t.Logf("Local penalty seg 1 (rate=1.2): %.4f", p1High)

	// Higher rate should produce higher penalty
	if p1High <= p1 {
		t.Errorf("Expected higher penalty for rate=1.2 (%.4f) vs rate=1.0 (%.4f)", p1High, p1)
	}
}

// TestSolveGreedySmooth_NoOverlapAfterRelax directly tests Problem 1:
// lowering a rate must not cause overlap with the next segment.
//
//	Scenario: S[i]=1000ms, R=1.3→D=770ms→end=1770ms
//	          S[i+1]=1800ms (only 30ms gap before next)
//	If R is lowered to 1.1: D=909ms→end=1909ms — would overlap S[i+1]!
func TestSolveGreedySmooth_NoOverlapAfterRelax(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	cfg.WindowRight = 200 * time.Millisecond // Keep narrow
	weights := newTestWeights()

	// Tight layout: segment 0 needs high compression, segment 1 starts very close after
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{
				Start: 1000 * time.Millisecond, End: 2000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{
				Start: 2050 * time.Millisecond, End: 3050 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{
				Start: 3100 * time.Millisecond, End: 4100 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1300 * time.Millisecond}, // Needs ~1.3x in 1000ms slot
			{Index: 1, NaturalLen: 800 * time.Millisecond},  // Fits easily
			{Index: 2, NaturalLen: 900 * time.Millisecond},  // Fits easily
		},
	}

	result, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Logf("Rates: %v", extractRates(result.Segments))
	for i, seg := range result.Segments {
		t.Logf("  Seg %d: start=%v end=%v rate=%.3f",
			i, seg.StartTime, seg.EndTime(), seg.Rate)
	}

	// CRITICAL: verify no overlap
	for i := 1; i < len(result.Segments); i++ {
		prevEnd := result.Segments[i-1].EndTime()
		currStart := result.Segments[i].StartTime
		if prevEnd > currStart {
			t.Fatalf("OVERLAP: seg %d ends at %v but seg %d starts at %v",
				i-1, prevEnd, i, currStart)
		}
		gap := currStart - prevEnd
		if gap < cfg.MinGap-1*time.Millisecond {
			t.Errorf("Gap between seg %d and %d is %v, less than MinGap %v",
				i-1, i, gap, cfg.MinGap)
		}
	}

	verifyFeasibility(t, result.Segments, &cfg)
}

// TestSolveGreedySmooth_NoCumulativeWindowViolation tests Problem 2:
// lowering R[2] from 1.30→1.15 must not push the segment's end past
// the window boundary.
//
//	Original: R=[1.0, 1.15, 1.30] (greedy upward propagation)
//	Smoothed: R=[1.0, 1.15, 1.15] — D[2] grows, must still fit in window
func TestSolveGreedySmooth_NoCumulativeWindowViolation(t *testing.T) {
	s := newTestSolver()
	cfg := newTestConfig()
	cfg.WindowRight = 300 * time.Millisecond // Moderate window
	weights := newTestWeights()

	// Build a scenario where greedy assigns R=[~1.0, ~1.15, ~1.30]
	// due to progressively longer TTS and tight slots
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{
				Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{
				Start: 1050 * time.Millisecond, End: 2050 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{
				Start: 2100 * time.Millisecond, End: 3100 * time.Millisecond}},
			{Index: 3, Interval: timeline.TimeInterval{
				Start: 3150 * time.Millisecond, End: 4150 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1000 * time.Millisecond}, // R=1.0
			{Index: 1, NaturalLen: 1150 * time.Millisecond}, // R~1.15
			{Index: 2, NaturalLen: 1300 * time.Millisecond}, // R~1.30
			{Index: 3, NaturalLen: 1000 * time.Millisecond}, // R=1.0 (after the chain)
		},
	}

	// Get greedy result first to see the baseline
	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}
	t.Logf("Greedy rates: %v", extractRates(greedyResult.Segments))
	for i, seg := range greedyResult.Segments {
		t.Logf("  Greedy seg %d: start=%v end=%v rate=%.3f",
			i, seg.StartTime, seg.EndTime(), seg.Rate)
	}

	// Get smooth result
	smoothResult, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Smooth error: %v", err)
	}
	t.Logf("Smooth rates: %v", extractRates(smoothResult.Segments))
	for i, seg := range smoothResult.Segments {
		t.Logf("  Smooth seg %d: start=%v end=%v rate=%.3f",
			i, seg.StartTime, seg.EndTime(), seg.Rate)
	}

	// CRITICAL: verify all window constraints
	for i, seg := range smoothResult.Segments {
		src := sub.SourceSegments[i]
		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		if seg.StartTime < windowMin-1*time.Millisecond {
			t.Errorf("Seg %d starts at %v, before windowMin %v", i, seg.StartTime, windowMin)
		}
		if seg.StartTime > windowMax+1*time.Millisecond {
			t.Errorf("Seg %d starts at %v, after windowMax %v", i, seg.StartTime, windowMax)
		}
	}

	verifyFeasibility(t, smoothResult.Segments, &cfg)
}

// TestValidateConstraints tests the safety-net validation function directly
func TestValidateConstraints(t *testing.T) {
	s := newSmoothTestSolver()
	cfg := newTestConfig()

	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 1100 * time.Millisecond, End: 2100 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 1000 * time.Millisecond},
			{Index: 1, NaturalLen: 1000 * time.Millisecond},
		},
	}

	// Valid plans
	validPlans := []timeline.SegmentPlan{
		{Index: 0, StartTime: 0, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
		{Index: 1, StartTime: 1100 * time.Millisecond, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
	}
	if !s.validateConstraints(validPlans, sub, &cfg) {
		t.Error("Expected valid plans to pass validation")
	}

	// Overlapping plans
	overlapPlans := []timeline.SegmentPlan{
		{Index: 0, StartTime: 0, Rate: 1.0, PlayDuration: 1200 * time.Millisecond},
		{Index: 1, StartTime: 1100 * time.Millisecond, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
	}
	if s.validateConstraints(overlapPlans, sub, &cfg) {
		t.Error("Expected overlapping plans to fail validation")
	}

	// Rate out of bounds
	rateOOBPlans := []timeline.SegmentPlan{
		{Index: 0, StartTime: 0, Rate: 1.5, PlayDuration: 667 * time.Millisecond},
		{Index: 1, StartTime: 1100 * time.Millisecond, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
	}
	if s.validateConstraints(rateOOBPlans, sub, &cfg) {
		t.Error("Expected rate-OOB plans to fail validation")
	}

	// Window violation
	windowPlans := []timeline.SegmentPlan{
		{Index: 0, StartTime: 0, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
		{Index: 1, StartTime: 2000 * time.Millisecond, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
	}
	if s.validateConstraints(windowPlans, sub, &cfg) {
		t.Error("Expected window-violation plans to fail validation")
	}
}

// --- Helper functions ---

func extractRates(plans []timeline.SegmentPlan) []float64 {
	rates := make([]float64, len(plans))
	for i, p := range plans {
		rates[i] = math.Round(p.Rate*1000) / 1000
	}
	return rates
}

func rateVariance(plans []timeline.SegmentPlan) float64 {
	if len(plans) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range plans {
		sum += p.Rate
	}
	mean := sum / float64(len(plans))
	variance := 0.0
	for _, p := range plans {
		diff := p.Rate - mean
		variance += diff * diff
	}
	return variance / float64(len(plans))
}

func maxRate(plans []timeline.SegmentPlan) float64 {
	max := 0.0
	for _, p := range plans {
		if p.Rate > max {
			max = p.Rate
		}
	}
	return max
}

func verifyFeasibility(t *testing.T, plans []timeline.SegmentPlan, cfg *timeline.DPConfig) {
	t.Helper()
	for i, seg := range plans {
		// Rate bounds
		if seg.Rate < cfg.RateMin-1e-6 || seg.Rate > cfg.RateMax+1e-6 {
			t.Errorf("Segment %d: rate %.4f out of bounds [%.2f, %.2f]",
				i, seg.Rate, cfg.RateMin, cfg.RateMax)
		}

		// No overlaps and min gap
		if i > 0 {
			prevEnd := plans[i-1].EndTime()
			gap := seg.StartTime - prevEnd
			if gap < 0 {
				t.Errorf("Segments %d and %d overlap by %v", i-1, i, -gap)
			} else if gap < cfg.MinGap-1*time.Millisecond { // 1ms tolerance for rounding
				t.Errorf("Gap between segments %d and %d is %v, less than MinGap %v",
					i-1, i, gap, cfg.MinGap)
			}
		}

		// SmoothDelta constraint
		if cfg.SmoothDelta > 0 && i > 0 {
			diff := math.Abs(seg.Rate - plans[i-1].Rate)
			if diff > cfg.SmoothDelta+0.01 { // Small tolerance
				t.Errorf("Rate diff between segments %d and %d is %.4f, exceeds SmoothDelta %.4f",
					i-1, i, diff, cfg.SmoothDelta)
			}
		}
	}
}

// TestSolveGreedySmooth_IsolatedBottleneck verifies that the algorithm
// handles the case where a single compressed segment is surrounded by
// uncompressed segments (rate=1.0). This is the domino_push scenario.
func TestSolveGreedySmooth_IsolatedBottleneck(t *testing.T) {
	s := newSmoothTestSolver()
	cfg := timeline.NewDefaultDPConfig(timeline.ModeB)
	weights := timeline.NewDefaultPenaltyWeights(timeline.ModeB)

	// Recreate the domino_push_podcast scenario
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 1000 * time.Millisecond, End: 3000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 3500 * time.Millisecond, End: 5500 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 6000 * time.Millisecond, End: 8000 * time.Millisecond}},
			{Index: 3, Interval: timeline.TimeInterval{Start: 8500 * time.Millisecond, End: 10500 * time.Millisecond}},
			{Index: 4, Interval: timeline.TimeInterval{Start: 11000 * time.Millisecond, End: 13000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 4000 * time.Millisecond},
			{Index: 1, NaturalLen: 3500 * time.Millisecond},
			{Index: 2, NaturalLen: 4200 * time.Millisecond},
			{Index: 3, NaturalLen: 3800 * time.Millisecond},
			{Index: 4, NaturalLen: 4000 * time.Millisecond},
		},
	}

	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}

	smoothResult, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Smooth error: %v", err)
	}

	t.Logf("Greedy rates: %v", extractRates(greedyResult.Segments))
	t.Logf("Smooth rates: %v", extractRates(smoothResult.Segments))

	// Verify feasibility
	verifyFeasibility(t, smoothResult.Segments, &cfg)

	// Calculate penalties
	greedyPenalty := s.penaltyCalc.CalculateTotal(sub.SourceSegments, greedyResult.Segments, &weights)
	smoothPenalty := s.penaltyCalc.CalculateTotal(sub.SourceSegments, smoothResult.Segments, &weights)

	t.Logf("Greedy penalty: %.4f", greedyPenalty)
	t.Logf("Smooth penalty: %.4f", smoothPenalty)

	// Smooth should not make things worse
	if smoothPenalty > greedyPenalty*1.05 {
		t.Errorf("Smooth penalty (%.4f) significantly worse than greedy (%.4f)", smoothPenalty, greedyPenalty)
	}

	// Check max rate improvement or equality
	greedyMaxRate := 0.0
	smoothMaxRate := 0.0
	for i := 0; i < len(greedyResult.Segments); i++ {
		if greedyResult.Segments[i].Rate > greedyMaxRate {
			greedyMaxRate = greedyResult.Segments[i].Rate
		}
		if smoothResult.Segments[i].Rate > smoothMaxRate {
			smoothMaxRate = smoothResult.Segments[i].Rate
		}
	}

	t.Logf("Greedy max rate: %.3f", greedyMaxRate)
	t.Logf("Smooth max rate: %.3f", smoothMaxRate)

	// The improved algorithm should distribute compression more evenly
	// We expect smooth to not have a significantly higher max rate than greedy
	if smoothMaxRate > greedyMaxRate+0.05 {
		t.Errorf("Smooth max rate (%.3f) significantly higher than greedy (%.3f)", smoothMaxRate, greedyMaxRate)
	}
}
