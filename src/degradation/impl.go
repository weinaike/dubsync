package degradation

import (
	"fmt"

	timeline "github.com/wnk/timeline/src"
)

// degradationHandlerImpl implements the DegradationHandler interface
type degradationHandlerImpl struct {
	penaltyCalc timeline.PenaltyCalculator
}

// NewDegradationHandler creates a new DegradationHandler instance
func NewDegradationHandler(penaltyCalc timeline.PenaltyCalculator) timeline.DegradationHandler {
	return &degradationHandlerImpl{
		penaltyCalc: penaltyCalc,
	}
}

// BottleneckAnalysis stores analysis results for infeasibility
type BottleneckAnalysis struct {
	Index         int
	LTrans        timeline.Duration
	AvailableTime timeline.Duration
	Shortfall     timeline.Duration
	RequiredRate  float64
}

// DetectBottleneck identifies segments that cause infeasibility
// Returns indices of bottleneck segments
func (d *degradationHandlerImpl) DetectBottleneck(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) []int {
	bottlenecks := []int{}
	n := len(sub.SourceSegments)

	for i := 0; i < n; i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]

		// Calculate available time window
		windowMin := maxDuration(0, src.OrigStart()-cfg.WindowLeft)
		windowMax := src.OrigStart() + cfg.WindowRight
		availableTime := windowMax - windowMin

		// Calculate minimum duration needed at max rate
		minDurationNeeded := timeline.Duration(float64(tts.NaturalLen) / cfg.RateMax)

		if minDurationNeeded > availableTime {
			bottlenecks = append(bottlenecks, i)
			continue
		}

		// Also check cumulative overflow from tight spacing
		if i > 0 {
			prevSrc := sub.SourceSegments[i-1]
			prevTTS := sub.TTSSegments[i-1]
			gap := src.OrigStart() - prevSrc.OrigEnd()

			// If segments are tightly packed
			if gap < cfg.MinGap {
				// Check if TTS audio significantly exceeds original duration
				if tts.NaturalLen > timeline.Duration(float64(src.OrigDuration())*cfg.RateMax) {
					bottlenecks = append(bottlenecks, i)
				}
			}

			// Check cumulative effect
			prevMinEnd := prevSrc.OrigStart() + timeline.Duration(float64(prevTTS.NaturalLen)/cfg.RateMax)
			if prevMinEnd > src.OrigStart() {
				// Previous segment overflows into this one
				bottlenecks = append(bottlenecks, i)
			}
		}
	}

	return bottlenecks
}

// RelaxConstraints creates a relaxed DPConfig for retry attempts
// Level 1 relaxation: 2x window, increased RateMax
func (d *degradationHandlerImpl) RelaxConstraints(
	cfg *timeline.DPConfig,
	bottleneckIndices []int,
) *timeline.DPConfig {
	relaxed := *cfg // Copy

	// Level 1 relaxation: expand search windows
	relaxed.WindowLeft *= 2
	relaxed.WindowRight *= 2

	// Allow slightly higher rate
	relaxed.RateMax = minFloat(cfg.RateMax*1.1, 1.5)

	// Relax smooth constraint
	relaxed.SmoothDelta = minFloat(cfg.SmoothDelta*1.5, 0.2)

	return &relaxed
}

// RelaxConstraintsLevel2 creates more aggressive relaxation for second retry
func RelaxConstraintsLevel2(cfg *timeline.DPConfig) *timeline.DPConfig {
	relaxed := *cfg

	// Level 2 relaxation: 3x window
	relaxed.WindowLeft *= 3
	relaxed.WindowRight *= 3

	// Allow even higher rate
	relaxed.RateMax = 1.6

	// Further relax smooth constraint
	relaxed.SmoothDelta = 0.25

	// Disable TopK for more exploration
	relaxed.TopK = 0

	return &relaxed
}

// RequestNegotiation generates a negotiation request for TTS pipeline
// Calculates the maximum allowed length for each bottleneck segment
func (d *degradationHandlerImpl) RequestNegotiation(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) *timeline.NegotiationRequest {
	bottlenecks := d.DetectBottleneck(sub, cfg)
	budgets := make([]timeline.SegmentBudget, 0, len(bottlenecks))

	for _, idx := range bottlenecks {
		src := sub.SourceSegments[idx]
		tts := sub.TTSSegments[idx]

		// Calculate the maximum allowed duration for this segment
		// Budget = available_time, and TTS must fit in budget * RateMax
		windowMax := src.OrigStart() + cfg.WindowRight
		windowMin := maxDuration(0, src.OrigStart()-cfg.WindowLeft)

		maxAllowedDuration := windowMax - windowMin

		// Budget = D_budget * R_max
		budget := timeline.Duration(float64(maxAllowedDuration) * cfg.RateMax)

		budgets = append(budgets, timeline.SegmentBudget{
			Index:         idx,
			MaxAllowedLen: budget,
			CurrentLen:    tts.NaturalLen,
		})
	}

	return &timeline.NegotiationRequest{
		Round:    1,
		Segments: budgets,
	}
}

// HardFallback implements Level 3 degradation: force placement with quality alerts
// This should only be used when all other strategies have failed
func (d *degradationHandlerImpl) HardFallback(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) (*timeline.Blueprint, *timeline.QualityAlert) {
	n := len(sub.SourceSegments)
	plans := make([]timeline.SegmentPlan, n)
	var worstAlert *timeline.QualityAlert
	var infeasibleInfos []timeline.InfeasibleInfo

	currentTime := timeline.Duration(0)

	for i := 0; i < n; i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]

		// Use maximum relaxed window
		windowMin := maxDuration(0, src.OrigStart()-cfg.WindowLeft*3)
		windowMax := src.OrigStart() + cfg.WindowRight*3

		// Start at the later of: (currentTime + MinGap) or windowMin
		startTime := maxDuration(currentTime+cfg.MinGap, windowMin)

		// Calculate required rate (may exceed normal limits)
		availableTime := maxDuration(cfg.TimeStep, windowMax-startTime)
		requiredRate := float64(tts.NaturalLen) / float64(availableTime)

		var rate float64
		var degradationType timeline.DegradationType

		// Determine degradation level based on rate
		if requiredRate <= 1.4 {
			rate = requiredRate
			degradationType = timeline.DegradationNone
		} else if requiredRate <= 1.6 {
			rate = requiredRate
			degradationType = timeline.DegradationOverspeed
		} else {
			// Extreme case: use max acceptable rate and mark for truncation
			rate = 1.6
			degradationType = timeline.DegradationSmartTruncate
		}

		playDuration := timeline.Duration(float64(tts.NaturalLen) / rate)

		plans[i] = timeline.SegmentPlan{
			Index:        sub.SourceSegments[i].Index,
			StartTime:    startTime,
			Rate:         rate,
			PlayDuration: playDuration,
			Degraded:     degradationType != timeline.DegradationNone,
			Degradation:  degradationType,
		}

		// Generate quality alert for degraded segments
		if degradationType != timeline.DegradationNone {
			alert := &timeline.QualityAlert{
				SegmentIndex: i,
				Reason:       fmt.Sprintf("Hard fallback: required rate %.2f exceeds limit", rate),
				Impact:       "high",
				Rate:         rate,
			}
			plans[i].QualityAlert = alert

			// Track worst alert
			if worstAlert == nil || rate > worstAlert.Rate {
				worstAlert = alert
			}

			infeasibleInfos = append(infeasibleInfos, timeline.InfeasibleInfo{
				Index:              i,
				Reason:             fmt.Sprintf("L_trans=%v, max_available=%v", tts.NaturalLen, availableTime),
				DegradationApplied: degradationType.String(),
				QualityImpact:      "high",
			})
		}

		currentTime = startTime + playDuration
	}

	return &timeline.Blueprint{
		Status:             timeline.StatusDegraded,
		Segments:           plans,
		InfeasibleSegments: infeasibleInfos,
		DegradationLevel:   3,
		Recommendation:     "Review bottleneck segments for content reduction or re-translation",
	}, worstAlert
}

// AnalyzeBottleneck provides detailed analysis of bottleneck segments
func (d *degradationHandlerImpl) AnalyzeBottleneck(
	sub *timeline.SubProblem,
	cfg *timeline.DPConfig,
) []BottleneckAnalysis {
	var analyses []BottleneckAnalysis

	for i := 0; i < len(sub.SourceSegments); i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]

		windowMin := maxDuration(0, src.OrigStart()-cfg.WindowLeft)
		windowMax := src.OrigStart() + cfg.WindowRight
		availableTime := windowMax - windowMin

		minDurationNeeded := timeline.Duration(float64(tts.NaturalLen) / cfg.RateMax)
		requiredRate := float64(tts.NaturalLen) / float64(maxDuration(1, availableTime))

		if minDurationNeeded > availableTime {
			analyses = append(analyses, BottleneckAnalysis{
				Index:         i,
				LTrans:        tts.NaturalLen,
				AvailableTime: availableTime,
				Shortfall:     minDurationNeeded - availableTime,
				RequiredRate:  requiredRate,
			})
		}
	}

	return analyses
}

// Helper functions

func maxDuration(a, b timeline.Duration) timeline.Duration {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
