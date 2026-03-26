package solver

import (
	"math"

	timeline "github.com/weinaike/timeline/src"
)

// smoothRates performs a post-processing pass on greedy results to optimize
// rate smoothness. The algorithm has three phases:
//
//  1. Rate Equalization: Identifies runs of consecutive accelerated segments
//     and redistributes compression demand evenly across them.
//  2. Forward-Backward Relaxation: Iteratively relaxes rates toward 1.0 using
//     alternating forward and backward sweeps, reducing unnecessary "inertia"
//     from the greedy SmoothDelta clamp.
//  3. Penalty-Aware Fine-Tuning: For each segment, searches a small neighborhood
//     of rate candidates to minimize the local penalty contribution (speed +
//     smooth + gap), while maintaining feasibility.
//
// All phases preserve the no-overlap and min-gap constraints.
func (s *solverImpl) smoothRates(
	plans []timeline.SegmentPlan,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
	weights *timeline.PenaltyWeights,
) []timeline.SegmentPlan {
	n := len(plans)
	if n <= 1 {
		return plans
	}

	// Snapshot the greedy result — this is our safety net
	greedy := make([]timeline.SegmentPlan, n)
	copy(greedy, plans)

	// Work on a copy to avoid mutating during iteration
	result := make([]timeline.SegmentPlan, n)
	copy(result, plans)

	// Phase 1: Rate Equalization — spread compression across consecutive segments
	result = s.equalizeRates(result, sub, cfg)

	// Phase 2: Forward-Backward Relaxation — relax rates toward 1.0
	const maxRelaxIter = 3
	for iter := 0; iter < maxRelaxIter; iter++ {
		changed := false
		// Forward pass: try to lower each rate toward 1.0
		for i := 0; i < n; i++ {
			if s.tryRelaxRate(result, i, sub, cfg, weights) {
				changed = true
			}
		}
		// Backward pass: try to lower each rate toward 1.0
		for i := n - 1; i >= 0; i-- {
			if s.tryRelaxRate(result, i, sub, cfg, weights) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Phase 3: Penalty-Aware Fine-Tuning — local search for better rates
	result = s.fineTuneRates(result, sub, cfg, weights)

	// Safety net: if post-processing broke any hard constraint, fall back to greedy
	if !s.validateConstraints(result, sub, cfg) {
		return greedy
	}

	return result
}

// equalizeRates finds runs of consecutive accelerated segments and distributes
// the total compression demand evenly. For example, if segments [2,3,4] have
// rates [1.3, 1.1, 1.0], and the total time budget allows it, we may reassign
// them to [1.13, 1.13, 1.13] which is smoother.
//
// Enhanced version: Also handles "isolated bottleneck" cases where a single
// segment with rate > 1.0 is surrounded by rate = 1.0 segments. In such cases,
// we extend the equalization range to include preceding segments to share
// the compression burden.
func (s *solverImpl) equalizeRates(
	plans []timeline.SegmentPlan,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) []timeline.SegmentPlan {
	n := len(plans)
	result := make([]timeline.SegmentPlan, n)
	copy(result, plans)

	// First, try the standard approach: find consecutive runs
	i := 0
	for i < n {
		if result[i].Rate <= 1.0+1e-9 {
			i++
			continue
		}

		runStart := i
		runEnd := i

		// Extend right while segments are accelerated
		for runEnd+1 < n && result[runEnd+1].Rate > 1.0+1e-9 {
			runEnd++
		}

		runLen := runEnd - runStart + 1
		if runLen >= 2 {
			// Standard case: consecutive run of compressed segments
			s.tryEqualizeRange(result, runStart, runEnd, sub, cfg)
		}
		i = runEnd + 1
	}

	// Second pass: check for isolated bottleneck (single compressed segment)
	// This handles cases like [1.0, 1.0, 1.0, 1.15, 1.0]
	bottleneckIdx := s.findBottleneckSegment(result)
	if bottleneckIdx >= 0 && s.isIsolatedBottleneck(result, bottleneckIdx) {
		// Try to equalize from the beginning to the bottleneck
		// This allows preceding segments to share the compression burden
		s.tryEqualizeRange(result, 0, bottleneckIdx, sub, cfg)
	}

	return result
}

// findBottleneckSegment finds the segment with the maximum rate.
// Returns -1 if no segment has rate > 1.0.
func (s *solverImpl) findBottleneckSegment(plans []timeline.SegmentPlan) int {
	maxRate := 1.0
	maxIdx := -1
	for i, plan := range plans {
		if plan.Rate > maxRate {
			maxRate = plan.Rate
			maxIdx = i
		}
	}
	return maxIdx
}

// isIsolatedBottleneck checks if the bottleneck segment is isolated,
// i.e., surrounded by segments with rate close to 1.0.
func (s *solverImpl) isIsolatedBottleneck(plans []timeline.SegmentPlan, idx int) bool {
	n := len(plans)
	if idx < 0 || idx >= n {
		return false
	}

	// Check if left neighbor has rate close to 1.0
	leftIsOne := idx == 0 || plans[idx-1].Rate <= 1.0+1e-9
	// Check if right neighbor has rate close to 1.0
	rightIsOne := idx == n-1 || plans[idx+1].Rate <= 1.0+1e-9

	// Isolated if both neighbors are at 1.0 (or boundary)
	return leftIsOne && rightIsOne
}

// tryEqualizeRange attempts to equalize rates across a range of segments.
// It calculates the optimal uniform rate and applies it if feasible.
func (s *solverImpl) tryEqualizeRange(
	plans []timeline.SegmentPlan,
	runStart, runEnd int,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) bool {
	if runStart > runEnd {
		return false
	}

	// Calculate total natural length for this range
	totalNaturalLen := timeline.Duration(0)
	for j := runStart; j <= runEnd; j++ {
		totalNaturalLen += sub.TTSSegments[j].NaturalLen
	}

	// Calculate total available play time
	// = last segment's end - first segment's start - total gaps
	firstStart := plans[runStart].StartTime
	lastEnd := plans[runEnd].EndTime()

	// Consider the constraint from the segment after the run
	if runEnd+1 < len(plans) {
		// The run must end before the next segment starts (minus gap)
		maxAllowedEnd := plans[runEnd+1].StartTime - cfg.MinGap
		if maxAllowedEnd < lastEnd {
			lastEnd = maxAllowedEnd
		}
	}

	// Calculate total gaps within the run
	totalGaps := timeline.Duration(0)
	for j := runStart; j < runEnd; j++ {
		gap := plans[j+1].StartTime - plans[j].EndTime()
		if gap > 0 {
			totalGaps += gap
		}
	}

	// Total play time available
	totalPlayTime := lastEnd - firstStart - totalGaps
	if totalPlayTime <= 0 {
		return false
	}

	// Calculate target equalized rate
	equalRate := float64(totalNaturalLen) / float64(totalPlayTime)

	// Clamp to valid range
	if equalRate < cfg.RateMin {
		equalRate = cfg.RateMin
	}
	if equalRate > cfg.RateMax {
		equalRate = cfg.RateMax
	}

	// Check if equalization would actually improve the max rate
	maxOldRate := 0.0
	for j := runStart; j <= runEnd; j++ {
		if plans[j].Rate > maxOldRate {
			maxOldRate = plans[j].Rate
		}
	}

	// Only apply if it would reduce the maximum rate
	if equalRate >= maxOldRate-0.005 {
		return false
	}

	// Apply equalization
	s.applyEqualizedRates(plans, runStart, runEnd, equalRate, sub, cfg)
	return true
}

// applyEqualizedRates applies the equalized rate to a run of segments,
// recomputing start times and play durations to maintain feasibility.
// Uses snapshot/rollback: if any constraint is violated, all changes are reverted.
func (s *solverImpl) applyEqualizedRates(
	plans []timeline.SegmentPlan,
	runStart, runEnd int,
	targetRate float64,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) {
	// Snapshot the original plans for rollback
	snapshot := make([]timeline.SegmentPlan, runEnd-runStart+1)
	copy(snapshot, plans[runStart:runEnd+1])

	rollback := func() {
		copy(plans[runStart:runEnd+1], snapshot)
	}

	// Determine the earliest possible start for the run
	// (constrained by the segment before runStart)
	var currentTime timeline.Duration
	if runStart > 0 {
		currentTime = plans[runStart-1].EndTime() + cfg.MinGap
	}

	for i := runStart; i <= runEnd; i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]

		// Compute window
		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		// Start time: max of currentTime, windowMin
		startTime := maxDuration(currentTime, windowMin)

		// Constraint: start must be within search window
		if startTime > windowMax {
			rollback()
			return
		}

		// Calculate play duration with target rate
		rate := targetRate
		newPlayDuration := timeline.Duration(float64(tts.NaturalLen) / rate)

		// Constraint: must not overlap with the segment after the run
		if i == runEnd && runEnd+1 < len(plans) {
			nextStart := plans[runEnd+1].StartTime
			if startTime+newPlayDuration+cfg.MinGap > nextStart {
				// Try adjusting rate to fit
				availableTime := nextStart - cfg.MinGap - startTime
				if availableTime <= 0 {
					rollback()
					return
				}
				rate = float64(tts.NaturalLen) / float64(availableTime)
				if rate > cfg.RateMax {
					rollback()
					return
				}
				newPlayDuration = timeline.Duration(float64(tts.NaturalLen) / rate)
			}
		}

		plans[i].StartTime = startTime
		plans[i].Rate = rate
		plans[i].PlayDuration = newPlayDuration
		currentTime = startTime + newPlayDuration + cfg.MinGap
	}

	// Final validation: verify no overlap with the segment after the run
	if runEnd+1 < len(plans) {
		if plans[runEnd].EndTime()+cfg.MinGap > plans[runEnd+1].StartTime {
			rollback()
			return
		}
	}
}

// tryRelaxRate attempts to move a segment's rate closer to 1.0 while maintaining
// feasibility (no overlap, min gap, window constraints).
// Returns true if the rate was changed.
func (s *solverImpl) tryRelaxRate(
	plans []timeline.SegmentPlan,
	idx int,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
	weights *timeline.PenaltyWeights,
) bool {
	n := len(plans)
	currentRate := plans[idx].Rate

	// Already at 1.0, nothing to relax
	if math.Abs(currentRate-1.0) < 1e-9 {
		return false
	}

	tts := sub.TTSSegments[idx]
	src := sub.SourceSegments[idx]

	// Calculate the maximum play duration allowed at this position
	maxPlayDuration := timeline.Duration(0)
	if idx < n-1 {
		maxPlayDuration = plans[idx+1].StartTime - cfg.MinGap - plans[idx].StartTime
	} else {
		// Last segment: can extend freely (within window)
		maxPlayDuration = src.OrigStart() + cfg.WindowRight + tts.NaturalLen - plans[idx].StartTime
	}

	if maxPlayDuration <= 0 {
		return false
	}

	// The minimum rate allowed by available space
	minRateBySpace := float64(tts.NaturalLen) / float64(maxPlayDuration)
	if minRateBySpace < cfg.RateMin {
		minRateBySpace = cfg.RateMin
	}

	// SmoothDelta constraint with neighbors
	minRateBySmooth := cfg.RateMin
	maxRateBySmooth := cfg.RateMax
	if cfg.SmoothDelta > 0 {
		if idx > 0 {
			prevRate := plans[idx-1].Rate
			lo := prevRate - cfg.SmoothDelta
			hi := prevRate + cfg.SmoothDelta
			if lo > minRateBySmooth {
				minRateBySmooth = lo
			}
			if hi < maxRateBySmooth {
				maxRateBySmooth = hi
			}
		}
		if idx < n-1 {
			nextRate := plans[idx+1].Rate
			lo := nextRate - cfg.SmoothDelta
			hi := nextRate + cfg.SmoothDelta
			if lo > minRateBySmooth {
				minRateBySmooth = lo
			}
			if hi < maxRateBySmooth {
				maxRateBySmooth = hi
			}
		}
	}

	// Target: move rate as close to 1.0 as constraints allow
	var targetRate float64
	if currentRate > 1.0 {
		// Currently accelerated — try to lower
		targetRate = math.Max(1.0, math.Max(minRateBySpace, minRateBySmooth))
	} else {
		// Currently decelerated — try to raise
		targetRate = math.Min(1.0, maxRateBySmooth)
		if targetRate < minRateBySpace {
			targetRate = minRateBySpace
		}
	}

	// Clamp to valid bounds
	if targetRate < cfg.RateMin {
		targetRate = cfg.RateMin
	}
	if targetRate > cfg.RateMax {
		targetRate = cfg.RateMax
	}

	// Only apply if it actually improves (moves closer to 1.0)
	if math.Abs(targetRate-1.0) >= math.Abs(currentRate-1.0)-1e-9 {
		return false
	}

	// Calculate the penalty improvement
	oldLocalPenalty := s.localPenalty(plans, idx, sub, weights)

	// Apply the new rate
	oldRate := plans[idx].Rate
	oldDuration := plans[idx].PlayDuration
	plans[idx].Rate = targetRate
	plans[idx].PlayDuration = timeline.Duration(float64(tts.NaturalLen) / targetRate)

	newLocalPenalty := s.localPenalty(plans, idx, sub, weights)

	if newLocalPenalty < oldLocalPenalty-1e-9 {
		// Improvement — keep the change
		return true
	}

	// No improvement — revert
	plans[idx].Rate = oldRate
	plans[idx].PlayDuration = oldDuration
	return false
}

// fineTuneRates performs a final penalty-aware local search.
// For each segment, it evaluates a set of candidate rates in a small
// neighborhood around the current rate and picks the one minimizing
// the total local penalty contribution (speed + smooth + gap).
func (s *solverImpl) fineTuneRates(
	plans []timeline.SegmentPlan,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
	weights *timeline.PenaltyWeights,
) []timeline.SegmentPlan {
	n := len(plans)
	result := make([]timeline.SegmentPlan, n)
	copy(result, plans)

	// Two iterations: forward then backward, to propagate improvements
	for iter := 0; iter < 2; iter++ {
		for i := 0; i < n; i++ {
			idx := i
			if iter%2 == 1 {
				idx = n - 1 - i
			}

			tts := sub.TTSSegments[idx]
			currentRate := result[idx].Rate

			// Calculate feasible rate bounds
			minRate, maxRate := s.feasibleRateBounds(result, idx, sub, cfg)
			if minRate > maxRate {
				continue
			}

			// Generate candidate rates in the neighborhood
			candidates := s.generateRateCandidates(currentRate, minRate, maxRate, cfg.RateStep)

			bestRate := currentRate
			bestPenalty := s.localPenalty(result, idx, sub, weights)

			for _, candidateRate := range candidates {
				// Temporarily apply candidate
				oldRate := result[idx].Rate
				oldDuration := result[idx].PlayDuration
				result[idx].Rate = candidateRate
				result[idx].PlayDuration = timeline.Duration(float64(tts.NaturalLen) / candidateRate)

				p := s.localPenalty(result, idx, sub, weights)
				if p < bestPenalty-1e-9 {
					bestPenalty = p
					bestRate = candidateRate
				}

				// Revert
				result[idx].Rate = oldRate
				result[idx].PlayDuration = oldDuration
			}

			// Apply best rate
			if math.Abs(bestRate-currentRate) > 1e-9 {
				result[idx].Rate = bestRate
				result[idx].PlayDuration = timeline.Duration(float64(tts.NaturalLen) / bestRate)
			}
		}
	}

	return result
}

// feasibleRateBounds calculates the [minRate, maxRate] range for a segment
// considering spatial constraints and SmoothDelta.
func (s *solverImpl) feasibleRateBounds(
	plans []timeline.SegmentPlan,
	idx int,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) (float64, float64) {
	n := len(plans)
	tts := sub.TTSSegments[idx]

	minRate := cfg.RateMin
	maxRate := cfg.RateMax

	// Space constraint: play duration can't cause overlap with next segment
	if idx < n-1 {
		maxPlayDuration := plans[idx+1].StartTime - cfg.MinGap - plans[idx].StartTime
		if maxPlayDuration > 0 {
			minRateBySpace := float64(tts.NaturalLen) / float64(maxPlayDuration)
			if minRateBySpace > minRate {
				minRate = minRateBySpace
			}
		} else {
			// No space — can only go faster (shouldn't happen with valid greedy result)
			minRate = maxRate
		}
	}

	// SmoothDelta constraint
	if cfg.SmoothDelta > 0 {
		if idx > 0 {
			prevRate := plans[idx-1].Rate
			lo := prevRate - cfg.SmoothDelta
			hi := prevRate + cfg.SmoothDelta
			if lo > minRate {
				minRate = lo
			}
			if hi < maxRate {
				maxRate = hi
			}
		}
		if idx < n-1 {
			nextRate := plans[idx+1].Rate
			lo := nextRate - cfg.SmoothDelta
			hi := nextRate + cfg.SmoothDelta
			if lo > minRate {
				minRate = lo
			}
			if hi < maxRate {
				maxRate = hi
			}
		}
	}

	return minRate, maxRate
}

// generateRateCandidates produces a set of rate values to try around the
// current rate. Includes 1.0, current ± small deltas, and boundary values.
func (s *solverImpl) generateRateCandidates(
	currentRate, minRate, maxRate, rateStep float64,
) []float64 {
	seen := make(map[float64]bool)
	var candidates []float64

	add := func(r float64) {
		// Round to rateStep precision to avoid floating point noise
		r = math.Round(r/rateStep) * rateStep
		if r >= minRate-1e-9 && r <= maxRate+1e-9 {
			r = math.Max(minRate, math.Min(maxRate, r))
			key := math.Round(r*1000) / 1000
			if !seen[key] {
				seen[key] = true
				candidates = append(candidates, r)
			}
		}
	}

	// Always try 1.0 if feasible
	add(1.0)

	// Current rate and small perturbations
	add(currentRate)
	for delta := rateStep; delta <= 5*rateStep; delta += rateStep {
		add(currentRate - delta)
		add(currentRate + delta)
	}

	// Boundary values
	add(minRate)
	add(maxRate)

	// Midpoints toward 1.0
	add((currentRate + 1.0) / 2)

	return candidates
}

// localPenalty computes the penalty contribution of segment idx and its
// interactions with neighbors (smooth penalty, gap penalty).
func (s *solverImpl) localPenalty(
	plans []timeline.SegmentPlan,
	idx int,
	sub *timeline.SubProblem,
	weights *timeline.PenaltyWeights,
) float64 {
	n := len(plans)
	p := 0.0

	// Shift penalty for this segment
	shift := plans[idx].StartTime - sub.SourceSegments[idx].OrigStart()
	p += s.penaltyCalc.ShiftPenalty(shift, weights)

	// Speed penalty for this segment
	p += s.penaltyCalc.SpeedPenalty(plans[idx].Rate, weights)

	// Smooth penalty with left neighbor
	if idx > 0 {
		p += s.penaltyCalc.SmoothPenalty(plans[idx-1].Rate, plans[idx].Rate, weights)
	}

	// Smooth penalty with right neighbor
	if idx < n-1 {
		p += s.penaltyCalc.SmoothPenalty(plans[idx].Rate, plans[idx+1].Rate, weights)
	}

	// Gap penalty with left neighbor
	if idx > 0 {
		origGap := sub.SourceSegments[idx].OrigStart() - sub.SourceSegments[idx-1].OrigEnd()
		actualGap := plans[idx].StartTime - plans[idx-1].EndTime()
		p += s.penaltyCalc.GapPenalty(origGap, actualGap, weights)
	}

	// Gap penalty with right neighbor
	if idx < n-1 {
		origGap := sub.SourceSegments[idx+1].OrigStart() - sub.SourceSegments[idx].OrigEnd()
		actualGap := plans[idx+1].StartTime - plans[idx].EndTime()
		p += s.penaltyCalc.GapPenalty(origGap, actualGap, weights)
	}

	return p
}

// validateConstraints performs a full constraint check on the result.
// Returns false if any hard constraint is violated, meaning the caller should
// fall back to the greedy result.
//
// Checked constraints:
//   - No overlap: S[i] + D[i] + MinGap <= S[i+1]
//   - Rate bounds: RateMin <= R[i] <= RateMax
//   - Window bounds: OrigStart[i] - WindowLeft <= S[i] <= OrigStart[i] + WindowRight
//   - SmoothDelta: |R[i+1] - R[i]| <= SmoothDelta (if configured)
func (s *solverImpl) validateConstraints(
	plans []timeline.SegmentPlan,
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) bool {
	n := len(plans)
	for i := 0; i < n; i++ {
		// Rate bounds
		if plans[i].Rate < cfg.RateMin-1e-6 || plans[i].Rate > cfg.RateMax+1e-6 {
			return false
		}

		// Window bounds
		src := sub.SourceSegments[i]
		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight
		if plans[i].StartTime < windowMin-1*timeline.Duration(1e6) || // 1ms tolerance
			plans[i].StartTime > windowMax+1*timeline.Duration(1e6) {
			return false
		}

		// No overlap + MinGap
		if i > 0 {
			prevEnd := plans[i-1].EndTime()
			gap := plans[i].StartTime - prevEnd
			if gap < cfg.MinGap-1*timeline.Duration(1e6) { // 1ms tolerance
				return false
			}
		}

		// SmoothDelta
		if cfg.SmoothDelta > 0 && i > 0 {
			diff := math.Abs(plans[i].Rate - plans[i-1].Rate)
			if diff > cfg.SmoothDelta+0.01 { // Small tolerance for float rounding
				return false
			}
		}
	}
	return true
}
