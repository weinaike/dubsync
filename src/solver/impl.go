package solver

import (
	"math"
	"sort"
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

// SolveGreedy implements Phase 1: greedy placement algorithm
// Produces a guaranteed feasible solution (no overlaps)
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
				// Last segment
				// For Mode A (narrow window), compress to fit in slot
				// For Mode B (wide window), allow extending
				if !isWideWindow {
					// Mode A: Fit in original slot via compression
					availableTime = slotDuration
				} else {
					// Mode B: Can extend beyond original slot
					availableTime = maxDuration(slotDuration, tts.NaturalLen)
				}
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

// DPState represents a dynamic programming state
type DPState struct {
	SegmentIndex int               // segment i
	StartTime    timeline.Duration // discrete start time s
	Rate         float64           // playback rate r
	TotalPenalty float64           // accumulated penalty
	PrevState    *DPState          // pointer to previous state for backtracking
}

// SolveDP implements Phase 2: dynamic programming optimization
// upperBound is the greedy solution penalty used for pruning
func (s *solverImpl) SolveDP(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
	weights *timeline.PenaltyWeights,
	upperBound float64,
) (*timeline.Blueprint, error) {
	n := len(sub.SourceSegments)
	if n == 0 {
		return &timeline.Blueprint{
			Status:   timeline.StatusOK,
			Segments: []timeline.SegmentPlan{},
		}, nil
	}

	// Generate rate candidates
	rates := generateRateCandidates(cfg.RateMin, cfg.RateMax, cfg.RateStep)

	// DP table: dp[i] maps timeStep to list of reachable states
	dp := make([]map[int][]*DPState, n)
	for i := range dp {
		dp[i] = make(map[int][]*DPState)
	}

	// Initialize first segment
	src0 := sub.SourceSegments[0]

	windowMin0 := maxDuration(0, src0.OrigStart()-cfg.WindowLeft)
	windowMax0 := src0.OrigStart() + cfg.WindowRight

	for _, rate := range rates {
		// Enumerate start times
		for start := windowMin0; start <= windowMax0; start += cfg.TimeStep {
			shift := start - src0.OrigStart()
			penalty := s.penaltyCalc.ShiftPenalty(shift, weights)
			penalty += s.penaltyCalc.SpeedPenalty(rate, weights)

			// Prune if already exceeds upper bound
			// Use > instead of >= to allow states with equal penalty
			if penalty > upperBound {
				continue
			}

			timeStep := int(start / cfg.TimeStep)
			state := &DPState{
				SegmentIndex: 0,
				StartTime:    start,
				Rate:         rate,
				TotalPenalty: penalty,
				PrevState:    nil,
			}
			dp[0][timeStep] = append(dp[0][timeStep], state)
		}
	}

	// Apply Top-K pruning to first segment
	if cfg.TopK > 0 {
		pruneStatesTopK(dp[0], cfg.TopK)
	}

	// DP transition for segments 1..n-1
	for i := 1; i < n; i++ {
		src := sub.SourceSegments[i]
		prevSrc := sub.SourceSegments[i-1]

		windowMin := maxDuration(0, src.OrigStart()-cfg.WindowLeft)
		windowMax := src.OrigStart() + cfg.WindowRight

		originalGap := src.OrigStart() - prevSrc.OrigEnd()

		// Enumerate current segment's start time
		for start := windowMin; start <= windowMax; start += cfg.TimeStep {
			timeStep := int(start / cfg.TimeStep)

			for _, rate := range rates {
				// Find valid previous states
				for _, prevStates := range dp[i-1] {
					for _, prevState := range prevStates {
						prevEnd := prevState.StartTime + timeline.Duration(
							float64(sub.TTSSegments[i-1].NaturalLen)/prevState.Rate,
						)

						// Check non-overlap and min gap constraint
						if prevEnd+cfg.MinGap > start {
							continue
						}

						// Check smooth constraint
						if cfg.SmoothDelta > 0 {
							rateDiff := math.Abs(rate - prevState.Rate)
							if rateDiff > cfg.SmoothDelta {
								continue
							}
						}

						// Calculate penalty for this transition
						shift := start - src.OrigStart()
						penalty := prevState.TotalPenalty
						penalty += s.penaltyCalc.ShiftPenalty(shift, weights)
						penalty += s.penaltyCalc.SpeedPenalty(rate, weights)
						penalty += s.penaltyCalc.SmoothPenalty(prevState.Rate, rate, weights)

						// Gap penalty
						actualGap := start - prevEnd
						penalty += s.penaltyCalc.GapPenalty(originalGap, actualGap, weights)

						// Prune if exceeds upper bound
						// Use > instead of >= to allow states with equal penalty
						if penalty > upperBound {
							continue
						}

						state := &DPState{
							SegmentIndex: i,
							StartTime:    start,
							Rate:         rate,
							TotalPenalty: penalty,
							PrevState:    prevState,
						}
						dp[i][timeStep] = append(dp[i][timeStep], state)
					}
				}
			}
		}

		// Apply Top-K pruning per time step
		if cfg.TopK > 0 {
			pruneStatesTopK(dp[i], cfg.TopK)
		}
	}

	// Find optimal final state
	var bestState *DPState
	for _, states := range dp[n-1] {
		for _, state := range states {
			if bestState == nil || state.TotalPenalty < bestState.TotalPenalty {
				bestState = state
			}
		}
	}

	if bestState == nil {
		return nil, timeline.NewScheduleError(
			timeline.ErrNoSolution,
			-1,
			"DP found no feasible solution",
		)
	}

	// Backtrack to get solution
	plans := make([]timeline.SegmentPlan, n)
	curr := bestState
	for i := n - 1; i >= 0; i-- {
		plans[i] = timeline.SegmentPlan{
			Index:        sub.SourceSegments[i].Index, // Preserve original global index
			StartTime:    curr.StartTime,
			Rate:         curr.Rate,
			PlayDuration: timeline.Duration(float64(sub.TTSSegments[i].NaturalLen) / curr.Rate),
		}
		curr = curr.PrevState
	}

	return &timeline.Blueprint{
		Status:   timeline.StatusOK,
		Segments: plans,
	}, nil
}

// generateRateCandidates generates rate values from min to max with given step
func generateRateCandidates(min, max, step float64) []float64 {
	var rates []float64
	count := int((max-min)/step) + 1
	rates = make([]float64, 0, count)

	for r := min; r <= max+0.001; r += step {
		rates = append(rates, math.Round(r*100)/100)
	}
	return rates
}

// pruneStatesTopK keeps only the top K states per time step based on penalty
func pruneStatesTopK(statesMap map[int][]*DPState, topK int) {
	for timeStep, states := range statesMap {
		if len(states) > topK {
			sort.Slice(states, func(a, b int) bool {
				return states[a].TotalPenalty < states[b].TotalPenalty
			})
			statesMap[timeStep] = states[:topK]
		}
	}
}

// maxDuration returns the larger of two Durations
func maxDuration(a, b timeline.Duration) timeline.Duration {
	if a > b {
		return a
	}
	return b
}

// max returns the larger of two float64s
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
