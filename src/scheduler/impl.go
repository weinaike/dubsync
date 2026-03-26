package scheduler

import (
	"fmt"

	timeline "github.com/wnk/timeline/src"
	"github.com/wnk/timeline/src/degradation"
	"github.com/wnk/timeline/src/penalty"
	"github.com/wnk/timeline/src/solver"
)

// schedulerImpl implements the Scheduler interface
type schedulerImpl struct {
	solver      timeline.Solver
	degrader    timeline.DegradationHandler
	penaltyCalc timeline.PenaltyCalculator
}

// NewScheduler creates a new Scheduler with default dependencies
func NewScheduler() timeline.Scheduler {
	pc := penalty.NewPenaltyCalculator()
	return &schedulerImpl{
		solver:      solver.NewSolver(pc),
		degrader:    degradation.NewDegradationHandler(pc),
		penaltyCalc: pc,
	}
}

// NewSchedulerWithDeps creates a Scheduler with injected dependencies (for testing)
func NewSchedulerWithDeps(
	solver timeline.Solver,
	degrader timeline.DegradationHandler,
	penaltyCalc timeline.PenaltyCalculator,
) timeline.Scheduler {
	return &schedulerImpl{
		solver:      solver,
		degrader:    degrader,
		penaltyCalc: penaltyCalc,
	}
}

// Schedule executes the full scheduling pipeline
// For chain-constrained problems, Greedy is optimal and doesn't require splitting
func (s *schedulerImpl) Schedule(input *timeline.PipelineInput) (*timeline.Blueprint, error) {
	// Step 1: Validate input
	if err := s.validateInput(input); err != nil {
		return nil, err
	}

	// Step 2: Solve using Greedy (optimal for chain-constrained problems)
	// No splitting needed - Greedy naturally handles long gaps
	sub := &timeline.SubProblem{
		SourceSegments: input.SourceSegments,
		TTSSegments:    input.TTSSegments,
	}

	result, err := s.solver.SolveGreedySmooth(sub, &input.Config.DP, &input.Config.Weights)
	if err != nil {
		return nil, err
	}

	// Step 3: Check if degradation is needed
	var infos []timeline.InfeasibleInfo
	degradationLevel := 0

	for i, seg := range result.Segments {
		if seg.Rate > input.Config.DP.RateMax {
			// Try degradation strategies
			degradationLevel = 1
			result.Segments[i].Degraded = true
			result.Segments[i].Degradation = timeline.DegradationOverspeed
			result.Segments[i].QualityAlert = &timeline.QualityAlert{
				SegmentIndex: seg.Index,
				Reason:       fmt.Sprintf("Rate %.2f exceeds max %.2f", seg.Rate, input.Config.DP.RateMax),
				Impact:       "high",
				Rate:         seg.Rate,
			}
		}
	}

	// Step 4: Build final blueprint
	status := timeline.StatusOK
	if len(infos) > 0 {
		status = timeline.StatusDegraded
	}

	recommendation := ""
	if degradationLevel >= 2 {
		recommendation = "Consider re-translating bottleneck segments with shorter alternatives"
	} else if degradationLevel >= 1 {
		recommendation = "Some segments required rate compression beyond ideal range"
	}

	return &timeline.Blueprint{
		Status:             status,
		Segments:           result.Segments,
		InfeasibleSegments: infos,
		DegradationLevel:   degradationLevel,
		Recommendation:     recommendation,
	}, nil
}

// validateInput validates the pipeline input
func (s *schedulerImpl) validateInput(input *timeline.PipelineInput) error {
	// Check segment count match
	if len(input.SourceSegments) != len(input.TTSSegments) {
		return timeline.NewScheduleError(
			timeline.ErrSegmentsMismatch,
			-1,
			fmt.Sprintf("source: %d, tts: %d", len(input.SourceSegments), len(input.TTSSegments)),
		)
	}

	// Validate configuration
	if err := input.Config.Validate(); err != nil {
		return err
	}

	// Verify segment indices are sequential
	for i, src := range input.SourceSegments {
		if src.Index != i {
			return timeline.NewScheduleError(
				timeline.ErrInvalidInput,
				i,
				fmt.Sprintf("expected index %d, got %d", i, src.Index),
			)
		}
	}

	// Verify TTS segment indices match source indices
	for i, tts := range input.TTSSegments {
		if tts.Index != i {
			return timeline.NewScheduleError(
				timeline.ErrInvalidInput,
				i,
				fmt.Sprintf("TTS segment index mismatch: expected %d, got %d", i, tts.Index),
			)
		}
	}

	return nil
}
