package solver

import (
	"time"

	timeline "github.com/wnk/timeline/src"
)

// solverImpl implements the Solver interface
type solverImpl struct {
	penaltyCalc timeline.PenaltyCalculator
}

// NewSolver creates a new Solver instance
func NewSolver(penaltyCalc timeline.PenaltyCalculator) timeline.Solver {
	return &solverImpl{
		penaltyCalc: penaltyCalc,
	}
}

// SolveGreedy implements the greedy placement algorithm
// For chain-constrained problems with convex penalties, this produces optimal solutions
func (s *solverImpl) SolveGreedy(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
	weights *timeline.PenaltyWeights,
) (*timeline.Blueprint, error) {
	n := len(sub.SourceSegments)
	if n == 0 {
		return &timeline.Blueprint{
			Status:   timeline.StatusOK,
			Segments: []timeline.SegmentPlan{},
		}, nil
	}

	plans := make([]timeline.SegmentPlan, n)
	currentTime := timeline.Duration(0)
	prevRate := 1.0

	for i := 0; i < n; i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]

		// Calculate search window for this segment's START time
		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		var startTime timeline.Duration
		var rate float64
		var playDuration timeline.Duration

		// Calculate the earliest valid start time (after previous segment + min gap)
		minStartTime := windowMin
		if i > 0 {
			minStartTime = maxDuration(currentTime+cfg.MinGap, windowMin)
		}

		// Original slot duration
		slotDuration := src.OrigDuration()

		// Step 1: Check if ideal placement with R=1.0 is valid
		// - Start time is valid (within window and no overlap with previous)
		// - TTS fits in original slot
		idealStart := src.OrigStart()
		if idealStart >= minStartTime && idealStart <= windowMax && tts.NaturalLen <= slotDuration {
			// Ideal case: original position with no compression
			startTime = idealStart
			rate = 1.0
			playDuration = tts.NaturalLen
		} else {
			// Step 2: TTS doesn't fit in original slot or position invalid
			// Strategy:
			// - Mode A (narrow window): Compress to fit in slot
			// - Mode B (wide window): Shift first, compress only if necessary

			startTime = minStartTime

			// Calculate available time considering next segment's constraint
			availableTime := slotDuration
			isWideWindow := cfg.WindowRight >= 2000*time.Millisecond // Mode B heuristic

			if i < n-1 {
				nextSrc := sub.SourceSegments[i+1]
				// Next segment can shift left by WindowLeft, so our max end is:
				// nextSrc.OrigStart() - WindowLeft - MinGap
				maxEnd := nextSrc.OrigStart() - cfg.WindowLeft - cfg.MinGap
				if maxEnd > startTime {
					availableTime = maxDuration(availableTime, maxEnd-startTime)
				}
				// For Mode B with wide window, next segment can shift right significantly
				if isWideWindow && float64(availableTime) < float64(tts.NaturalLen) {
					potentialMaxEnd := nextSrc.OrigStart() + cfg.WindowRight - cfg.MinGap
					if potentialMaxEnd > startTime {
						potentialAvailable := potentialMaxEnd - startTime
						if float64(potentialAvailable) >= float64(tts.NaturalLen) {
							// Next segment can shift right to accommodate us
							availableTime = potentialAvailable
						}
					}
				}
			} else {
				// Last segment: no downstream constraint
				// Can play at natural length as long as startTime is within window
				// (which is already guaranteed by minStartTime calculation)
				availableTime = tts.NaturalLen
			}

			// Calculate required rate
			if float64(availableTime) >= float64(tts.NaturalLen) {
				rate = 1.0
				playDuration = tts.NaturalLen
			} else {
				requiredRate := float64(tts.NaturalLen) / float64(availableTime)
				if requiredRate <= cfg.RateMax {
					rate = requiredRate
				} else {
					rate = cfg.RateMax
				}
				playDuration = timeline.Duration(float64(tts.NaturalLen) / rate)
			}
		}

		// Apply smooth constraint if configured
		if cfg.SmoothDelta > 0 && i > 0 {
			maxRate := prevRate + cfg.SmoothDelta
			minRate := prevRate - cfg.SmoothDelta
			if minRate < cfg.RateMin {
				minRate = cfg.RateMin
			}
			if maxRate > cfg.RateMax {
				maxRate = cfg.RateMax
			}

			if rate > maxRate {
				rate = maxRate
				playDuration = timeline.Duration(float64(tts.NaturalLen) / rate)
			} else if rate < minRate {
				rate = minRate
				playDuration = timeline.Duration(float64(tts.NaturalLen) / rate)
			}
		}

		plans[i] = timeline.SegmentPlan{
			Index:        sub.SourceSegments[i].Index, // Preserve original global index
			StartTime:    startTime,
			Rate:         rate,
			PlayDuration: playDuration,
		}

		currentTime = startTime + playDuration
		prevRate = rate
	}

	return &timeline.Blueprint{
		Status:   timeline.StatusOK,
		Segments: plans,
	}, nil
}

// maxDuration returns the larger of two Durations
func maxDuration(a, b timeline.Duration) timeline.Duration {
	if a > b {
		return a
	}
	return b
}

// SolveGreedySmooth runs the greedy solver followed by rate smoothing post-processing.
// This reduces unnecessary rate jumps and distributes compression more evenly.
func (s *solverImpl) SolveGreedySmooth(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
	weights *timeline.PenaltyWeights,
) (*timeline.Blueprint, error) {
	// Phase 1: Run standard greedy
	result, err := s.SolveGreedy(sub, cfg, weights)
	if err != nil {
		return nil, err
	}

	if len(result.Segments) <= 1 {
		return result, nil
	}

	// Early exit: if greedy already found perfect solution (all rates = 1.0),
	// skip smoothing to preserve the optimal result and avoid unnecessary rate changes.
	if s.allRatesAreOne(result.Segments) {
		return result, nil
	}

	// Phase 2: Apply rate smoothing post-processing
	result.Segments = s.smoothRates(result.Segments, sub, cfg, weights)

	return result, nil
}

// allRatesAreOne checks if all segments have rate exactly 1.0
func (s *solverImpl) allRatesAreOne(plans []timeline.SegmentPlan) bool {
	for _, p := range plans {
		if p.Rate < 1.0-1e-6 || p.Rate > 1.0+1e-6 {
			return false
		}
	}
	return true
}
