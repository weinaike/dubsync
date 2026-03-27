package penalty

import (
	timeline "github.com/weinaike/dubsync/src"
)

// penaltyCalculatorImpl implements the PenaltyCalculator interface
type penaltyCalculatorImpl struct{}

// NewPenaltyCalculator creates a new PenaltyCalculator instance
func NewPenaltyCalculator() timeline.PenaltyCalculator {
	return &penaltyCalculatorImpl{}
}

// CalculateTotal calculates the total penalty for a complete schedule
// Includes: shift penalty, speed penalty, smooth penalty, and gap penalty
func (p *penaltyCalculatorImpl) CalculateTotal(
	sources []timeline.SourceSegment,
	plans []timeline.SegmentPlan,
	weights *timeline.PenaltyWeights,
) float64 {
	var total float64
	n := len(sources)

	for i := 0; i < n; i++ {
		// Shift penalty
		shift := plans[i].StartTime - sources[i].OrigStart()
		total += p.ShiftPenalty(shift, weights)

		// Speed penalty
		total += p.SpeedPenalty(plans[i].Rate, weights)

		// Smooth penalty (compare with previous segment)
		if i > 0 {
			total += p.SmoothPenalty(plans[i-1].Rate, plans[i].Rate, weights)
		}

		// Gap penalty (compare with original gap)
		if i > 0 {
			originalGap := sources[i].OrigStart() - sources[i-1].OrigEnd()
			actualGap := plans[i].StartTime - plans[i-1].EndTime()
			total += p.GapPenalty(originalGap, actualGap, weights)
		}
	}

	return total
}

// ShiftPenalty calculates the penalty for time displacement
// Formula: W_shift * |shift| in seconds
func (p *penaltyCalculatorImpl) ShiftPenalty(
	shift timeline.Duration,
	weights *timeline.PenaltyWeights,
) float64 {
	if shift < 0 {
		shift = -shift
	}
	return weights.WShift * shift.Seconds()
}

// SpeedPenalty calculates the penalty for playback rate deviation from 1.0
// Uses piecewise linear penalty based on SpeedBandsAccel (rate >= 1.0) or
// SpeedBandsDecel (rate < 1.0)
func (p *penaltyCalculatorImpl) SpeedPenalty(
	rate float64,
	weights *timeline.PenaltyWeights,
) float64 {
	if rate >= 1.0 {
		return p.calculateSpeedPenaltyBands(rate, weights.SpeedBandsAccel, 1.0)
	}
	return p.calculateSpeedPenaltyBandsDecel(rate, weights.SpeedBandsDecel)
}

// calculateSpeedPenaltyBands calculates piecewise linear penalty for acceleration
func (p *penaltyCalculatorImpl) calculateSpeedPenaltyBands(
	rate float64,
	bands []timeline.SpeedPenaltyBand,
	baseRate float64,
) float64 {
	if len(bands) == 0 {
		return 0
	}

	penalty := 0.0
	prevBound := baseRate

	for _, band := range bands {
		if rate <= band.RateUpperBound {
			// Rate is within this band
			penalty += band.MarginalSlope * (rate - prevBound)
			break
		}
		// Rate exceeds this band, accumulate full band penalty
		penalty += band.MarginalSlope * (band.RateUpperBound - prevBound)
		prevBound = band.RateUpperBound
	}

	return penalty
}

// calculateSpeedPenaltyBandsDecel calculates piecewise linear penalty for deceleration
// For decel bands, RateUpperBound represents the lower bound (e.g., 0.95, 1.0)
func (p *penaltyCalculatorImpl) calculateSpeedPenaltyBandsDecel(
	rate float64,
	bands []timeline.SpeedPenaltyBand,
) float64 {
	if len(bands) == 0 {
		return 0
	}

	// Bands for deceleration are ordered from low to high (e.g., 0.95, 1.0)
	// We need to find which band the rate falls into
	penalty := 0.0

	for i := len(bands) - 1; i >= 0; i-- {
		band := bands[i]
		if rate <= band.RateUpperBound {
			// Rate is within or below this band
			if i > 0 {
				prevBound := bands[i-1].RateUpperBound
				penalty += band.MarginalSlope * (rate - prevBound)
			} else {
				// Rate is in the lowest band
				penalty += band.MarginalSlope * (rate - 0) // Assume 0 as absolute minimum
			}
			break
		}
	}

	return penalty
}

// SmoothPenalty calculates the penalty for rate changes between adjacent segments
// Formula: W_smooth * |rate1 - rate2|
func (p *penaltyCalculatorImpl) SmoothPenalty(
	rate1, rate2 float64,
	weights *timeline.PenaltyWeights,
) float64 {
	diff := rate2 - rate1
	if diff < 0 {
		diff = -diff
	}
	return weights.WSmooth * diff
}

// GapPenalty calculates the penalty for gap deviation from original
// Formula: W_gap * |actualGap - originalGap| in seconds
// Bidirectional: both gap expansion and shrinkage are penalized
func (p *penaltyCalculatorImpl) GapPenalty(
	originalGap, actualGap timeline.Duration,
	weights *timeline.PenaltyWeights,
) float64 {
	diff := actualGap - originalGap
	if diff < 0 {
		diff = -diff
	}
	return weights.WGap * diff.Seconds()
}
