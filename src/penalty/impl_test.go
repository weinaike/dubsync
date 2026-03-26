package penalty

import (
	"testing"
	"time"

	timeline "github.com/weinaike/timeline/src"
)

func TestShiftPenalty_Zero(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WShift: 10.0}

	penalty := p.ShiftPenalty(0, weights)
	if penalty != 0 {
		t.Errorf("Expected 0 penalty for zero shift, got %f", penalty)
	}
}

func TestShiftPenalty_Positive(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WShift: 10.0}

	// 500ms shift = 0.5s, penalty = 10.0 * 0.5 = 5.0
	penalty := p.ShiftPenalty(500*time.Millisecond, weights)
	if penalty != 5.0 {
		t.Errorf("Expected 5.0 penalty for 500ms shift, got %f", penalty)
	}
}

func TestShiftPenalty_Negative(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WShift: 10.0}

	// -500ms shift should have same penalty as +500ms
	penalty := p.ShiftPenalty(-500*time.Millisecond, weights)
	if penalty != 5.0 {
		t.Errorf("Expected 5.0 penalty for -500ms shift, got %f", penalty)
	}
}

func TestSpeedPenalty_Rate1(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{
		SpeedBandsAccel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 1.05, MarginalSlope: 1.0},
			{RateUpperBound: 1.15, MarginalSlope: 3.0},
		},
	}

	penalty := p.SpeedPenalty(1.0, weights)
	if penalty != 0 {
		t.Errorf("Expected 0 penalty for rate 1.0, got %f", penalty)
	}
}

func TestSpeedPenalty_Rate1_1(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{
		SpeedBandsAccel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 1.05, MarginalSlope: 1.0},
			{RateUpperBound: 1.15, MarginalSlope: 3.0},
		},
	}

	// Rate 1.1: in second band
	// First band: 1.0 * (1.05 - 1.0) = 0.05
	// Second band: 3.0 * (1.1 - 1.05) = 0.15
	// Total: 0.20
	penalty := p.SpeedPenalty(1.1, weights)
	if penalty < 0.19 || penalty > 0.21 {
		t.Errorf("Expected ~0.20 penalty for rate 1.1, got %f", penalty)
	}
}

func TestSpeedPenalty_Decel(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{
		SpeedBandsDecel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 0.95, MarginalSlope: 2.0},
			{RateUpperBound: 1.00, MarginalSlope: 1.0},
		},
	}

	// Rate 0.97: should have some penalty
	penalty := p.SpeedPenalty(0.97, weights)
	if penalty <= 0 {
		t.Errorf("Expected positive penalty for rate 0.97, got %f", penalty)
	}
}

func TestSmoothPenalty_Zero(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WSmooth: 5.0}

	penalty := p.SmoothPenalty(1.0, 1.0, weights)
	if penalty != 0 {
		t.Errorf("Expected 0 penalty for same rates, got %f", penalty)
	}
}

func TestSmoothPenalty_Diff(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WSmooth: 5.0}

	// |1.2 - 1.0| * 5.0 = 1.0
	penalty := p.SmoothPenalty(1.0, 1.2, weights)
	if penalty < 0.99 || penalty > 1.01 {
		t.Errorf("Expected 1.0 penalty for rate diff 0.2, got %f", penalty)
	}
}

func TestGapPenalty_Equal(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WGap: 3.0}

	penalty := p.GapPenalty(200*time.Millisecond, 200*time.Millisecond, weights)
	if penalty != 0 {
		t.Errorf("Expected 0 penalty for equal gaps, got %f", penalty)
	}
}

func TestGapPenalty_Diff(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{WGap: 3.0}

	// |0.3s - 0.2s| * 3.0 = 0.3
	penalty := p.GapPenalty(200*time.Millisecond, 300*time.Millisecond, weights)
	if penalty < 0.29 || penalty > 0.31 {
		t.Errorf("Expected 0.3 penalty for gap diff 100ms, got %f", penalty)
	}
}

func TestCalculateTotal(t *testing.T) {
	p := NewPenaltyCalculator()
	weights := &timeline.PenaltyWeights{
		WShift:  10.0,
		WSmooth: 5.0,
		WGap:    3.0,
		SpeedBandsAccel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 1.05, MarginalSlope: 1.0},
			{RateUpperBound: 1.40, MarginalSlope: 3.0},
		},
		SpeedBandsDecel: []timeline.SpeedPenaltyBand{
			{RateUpperBound: 0.95, MarginalSlope: 2.0},
			{RateUpperBound: 1.00, MarginalSlope: 1.0},
		},
	}

	sources := []timeline.SourceSegment{
		{Index: 0, Interval: timeline.TimeInterval{Start: 0, End: 1000 * time.Millisecond}},
		{Index: 1, Interval: timeline.TimeInterval{Start: 1200 * time.Millisecond, End: 2200 * time.Millisecond}},
	}

	plans := []timeline.SegmentPlan{
		{Index: 0, StartTime: 0, Rate: 1.0, PlayDuration: 1000 * time.Millisecond},
		{Index: 1, StartTime: 1300 * time.Millisecond, Rate: 1.1, PlayDuration: 909 * time.Millisecond},
	}

	total := p.CalculateTotal(sources, plans, weights)
	if total <= 0 {
		t.Errorf("Expected positive total penalty, got %f", total)
	}
}
