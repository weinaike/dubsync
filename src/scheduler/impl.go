package scheduler

import (
	"fmt"
	"time"

	timeline "github.com/wnk/timeline/src"
	"github.com/wnk/timeline/src/degradation"
	"github.com/wnk/timeline/src/penalty"
	"github.com/wnk/timeline/src/solver"
	"github.com/wnk/timeline/src/splitter"
)

// schedulerImpl implements the Scheduler interface
type schedulerImpl struct {
	splitter    timeline.Splitter
	solver      timeline.Solver
	degrader    timeline.DegradationHandler
	penaltyCalc timeline.PenaltyCalculator
}

// NewScheduler creates a new Scheduler with default dependencies
func NewScheduler() timeline.Scheduler {
	penaltyCalc := penalty.NewPenaltyCalculator()
	return &schedulerImpl{
		splitter:    splitter.NewSplitter(),
		solver:      solver.NewSolver(penaltyCalc),
		degrader:    degradation.NewDegradationHandler(penaltyCalc),
		penaltyCalc: penaltyCalc,
	}
}

// NewSchedulerWithDeps creates a Scheduler with injected dependencies (for testing)
func NewSchedulerWithDeps(
	splitter timeline.Splitter,
	solver timeline.Solver,
	degrader timeline.DegradationHandler,
	penaltyCalc timeline.PenaltyCalculator,
) timeline.Scheduler {
	return &schedulerImpl{
		splitter:    splitter,
		solver:      solver,
		degrader:    degrader,
		penaltyCalc: penaltyCalc,
	}
}

// Schedule executes the full scheduling pipeline
func (s *schedulerImpl) Schedule(input *timeline.PipelineInput) (*timeline.Blueprint, error) {
	// Step 1: Validate input
	if err := s.validateInput(input); err != nil {
		return nil, err
	}

	// Step 2: Split into sub-problems
	subProblems := s.splitter.Split(
		input.SourceSegments,
		input.TTSSegments,
		input.Config.DP.BreakThreshold,
	)

	// Step 3: Solve each sub-problem
	allPlans := make([]timeline.SegmentPlan, 0, len(input.SourceSegments))
	var allInfeasibleInfos []timeline.InfeasibleInfo
	maxDegradationLevel := 0

	for _, sub := range subProblems {
		plans, infos, degradationLevel, err := s.solveSubProblem(&sub, &input.Config)
		if err != nil {
			return nil, err
		}
		allPlans = append(allPlans, plans...)
		allInfeasibleInfos = append(allInfeasibleInfos, infos...)
		if degradationLevel > maxDegradationLevel {
			maxDegradationLevel = degradationLevel
		}
	}

	// Step 4: Build final blueprint
	status := timeline.StatusOK
	if len(allInfeasibleInfos) > 0 {
		status = timeline.StatusDegraded
	}

	recommendation := ""
	if maxDegradationLevel >= 2 {
		recommendation = "Consider re-translating bottleneck segments with shorter alternatives"
	} else if maxDegradationLevel >= 1 {
		recommendation = "Some segments required constraint relaxation"
	}

	return &timeline.Blueprint{
		Status:             status,
		Segments:           allPlans,
		InfeasibleSegments: allInfeasibleInfos,
		DegradationLevel:   maxDegradationLevel,
		Recommendation:     recommendation,
	}, nil
}

// solveSubProblem solves a single sub-problem with degradation fallback
func (s *schedulerImpl) solveSubProblem(
	sub *timeline.SubProblem,
	cfg *timeline.SchedulerConfig,
) ([]timeline.SegmentPlan, []timeline.InfeasibleInfo, int, error) {
	var infos []timeline.InfeasibleInfo
	degradationLevel := 0

	// Phase 1: Greedy solution (guaranteed feasible or explicit failure)
	greedyResult, err := s.solver.SolveGreedy(sub, &cfg.DP, &cfg.Weights)
	if err != nil {
		return nil, nil, 0, err
	}

	// Calculate upper bound from greedy solution
	upperBound := s.penaltyCalc.CalculateTotal(
		sub.SourceSegments,
		greedyResult.Segments,
		&cfg.Weights,
	)

	// Phase 2: DP optimization
	// Skip DP when greedy already achieves all rate=1.0 (no compression needed)
	// In this case DP cannot significantly improve the solution
	allRatesPerfect := true
	for _, seg := range greedyResult.Segments {
		if seg.Rate < 0.999 || seg.Rate > 1.001 {
			allRatesPerfect = false
			break
		}
	}

	// Calculate max rate from greedy result
	maxGreedyRate := 0.0
	for _, seg := range greedyResult.Segments {
		if seg.Rate > maxGreedyRate {
			maxGreedyRate = seg.Rate
		}
	}

	// Heuristic: Skip DP if:
	// 1. All rates are perfect (1.0), OR
	// 2. Upper bound is 0 (no penalty), OR
	// 3. Greedy is "good enough":
	//    - For Mode B (wide window): max rate < 1.2 and problem >= 5 segments
	//    - For large problems (>= 10 segments): max rate <= 1.25
	// 4. Extreme cases where DP won't help:
	//    - Max greedy rate > 1.35 (infeasible or near-infeasible)
	//    - Problem size >= 8 and max rate >= 1.3 (computationally expensive)
	problemSize := len(sub.SourceSegments)
	skipDP := allRatesPerfect || upperBound == 0 ||
		(maxGreedyRate < 1.2 && problemSize >= 5 && cfg.DP.WindowRight > 2000*time.Millisecond) ||
		(maxGreedyRate <= 1.25 && problemSize >= 10) ||
		maxGreedyRate > 1.35 ||
		(problemSize >= 8 && maxGreedyRate >= 1.3)

	var dpResult *timeline.Blueprint
	if skipDP {
		// Greedy is already optimal or near-optimal
		dpResult = greedyResult
	} else {
		dpResult, err = s.solver.SolveDP(sub, &cfg.DP, &cfg.Weights, upperBound)
	}

	if err != nil {
		// DP failed, try degradation strategies

		// Level 1: Constraint relaxation
		degradationLevel = 1
		bottlenecks := s.degrader.DetectBottleneck(sub, &cfg.DP)

		if len(bottlenecks) > 0 {
			relaxedCfg := s.degrader.RelaxConstraints(&cfg.DP, bottlenecks)
			dpResult, err = s.solver.SolveDP(sub, relaxedCfg, &cfg.Weights, upperBound*2)
		}

		if err != nil {
			// Level 2: More aggressive relaxation
			degradationLevel = 2
			relaxedCfg2 := degradation.RelaxConstraintsLevel2(&cfg.DP)
			dpResult, err = s.solver.SolveDP(sub, relaxedCfg2, &cfg.Weights, upperBound*3)

			if err != nil && cfg.MaxNegotiation > 0 {
				// Note: Actual negotiation would require callback mechanism
				// For now, proceed to Level 3
			}

			if err != nil {
				// Level 3: Hard fallback
				degradationLevel = 3
				var alert *timeline.QualityAlert
				dpResult, alert = s.degrader.HardFallback(sub, &cfg.DP)

				if alert != nil {
					infos = append(infos, timeline.InfeasibleInfo{
						Index:              alert.SegmentIndex,
						Reason:             alert.Reason,
						DegradationApplied: "hard_fallback",
						QualityImpact:      alert.Impact,
					})
				}
			}
		}
	}

	// Mark degraded segments based on actual rates
	for i := range dpResult.Segments {
		if dpResult.Segments[i].Rate > cfg.DP.RateMax && !dpResult.Segments[i].Degraded {
			dpResult.Segments[i].Degraded = true
			dpResult.Segments[i].Degradation = timeline.DegradationOverspeed
			dpResult.Segments[i].QualityAlert = &timeline.QualityAlert{
				SegmentIndex: dpResult.Segments[i].Index,
				Reason:       fmt.Sprintf("Rate %.2f exceeds max %.2f", dpResult.Segments[i].Rate, cfg.DP.RateMax),
				Impact:       "high",
				Rate:         dpResult.Segments[i].Rate,
			}
		}
	}

	return dpResult.Segments, infos, degradationLevel, nil
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
