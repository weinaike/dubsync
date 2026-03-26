package solver

import (
	"fmt"
	"testing"
	"time"

	timeline "github.com/weinaike/timeline/src"
	"github.com/weinaike/timeline/src/penalty"
)

// TestLectureScenario_ModeB 重现用户提供的 lecture (Mode B) 案例
// 验证贪心 vs 平滑的输出对比
func TestLectureScenario_ModeB(t *testing.T) {
	s := &solverImpl{
		penaltyCalc: penalty.NewPenaltyCalculator(),
	}
	cfg := timeline.NewDefaultDPConfig(timeline.ModeB)
	weights := timeline.NewDefaultPenaltyWeights(timeline.ModeB)

	// 用户提供的 lecture 数据
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 500 * time.Millisecond, End: 4500 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 4800 * time.Millisecond, End: 9500 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 10000 * time.Millisecond, End: 15000 * time.Millisecond}},
			{Index: 3, Interval: timeline.TimeInterval{Start: 15500 * time.Millisecond, End: 20500 * time.Millisecond}},
			{Index: 4, Interval: timeline.TimeInterval{Start: 21000 * time.Millisecond, End: 26000 * time.Millisecond}},
			{Index: 5, Interval: timeline.TimeInterval{Start: 26500 * time.Millisecond, End: 31500 * time.Millisecond}},
			{Index: 6, Interval: timeline.TimeInterval{Start: 32000 * time.Millisecond, End: 37000 * time.Millisecond}},
			{Index: 7, Interval: timeline.TimeInterval{Start: 37500 * time.Millisecond, End: 42500 * time.Millisecond}},
			{Index: 8, Interval: timeline.TimeInterval{Start: 43000 * time.Millisecond, End: 48000 * time.Millisecond}},
			{Index: 9, Interval: timeline.TimeInterval{Start: 48500 * time.Millisecond, End: 53500 * time.Millisecond}},
			{Index: 10, Interval: timeline.TimeInterval{Start: 54000 * time.Millisecond, End: 59000 * time.Millisecond}},
			{Index: 11, Interval: timeline.TimeInterval{Start: 59500 * time.Millisecond, End: 64500 * time.Millisecond}},
			{Index: 12, Interval: timeline.TimeInterval{Start: 65000 * time.Millisecond, End: 70000 * time.Millisecond}},
			{Index: 13, Interval: timeline.TimeInterval{Start: 70500 * time.Millisecond, End: 75500 * time.Millisecond}},
			{Index: 14, Interval: timeline.TimeInterval{Start: 76000 * time.Millisecond, End: 81000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 4200 * time.Millisecond},
			{Index: 1, NaturalLen: 4700 * time.Millisecond},
			{Index: 2, NaturalLen: 4900 * time.Millisecond},
			{Index: 3, NaturalLen: 4200 * time.Millisecond},
			{Index: 4, NaturalLen: 4700 * time.Millisecond},
			{Index: 5, NaturalLen: 5400 * time.Millisecond},
			{Index: 6, NaturalLen: 4700 * time.Millisecond},
			{Index: 7, NaturalLen: 4900 * time.Millisecond},
			{Index: 8, NaturalLen: 4200 * time.Millisecond},
			{Index: 9, NaturalLen: 4700 * time.Millisecond},
			{Index: 10, NaturalLen: 5400 * time.Millisecond},
			{Index: 11, NaturalLen: 4700 * time.Millisecond},
			{Index: 12, NaturalLen: 4900 * time.Millisecond},
			{Index: 13, NaturalLen: 4200 * time.Millisecond},
			{Index: 14, NaturalLen: 4700 * time.Millisecond},
		},
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Mode B Configuration:")
	fmt.Printf("  RateMin=%.2f, RateMax=%.2f, SmoothDelta=%.2f\n", cfg.RateMin, cfg.RateMax, cfg.SmoothDelta)
	fmt.Printf("  WindowLeft=%v, WindowRight=%v, MinGap=%v\n", cfg.WindowLeft, cfg.WindowRight, cfg.MinGap)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 贪心结果
	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}

	// 平滑结果
	smoothResult, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Smooth error: %v", err)
	}

	// 计算总惩罚
	greedyPenalty := s.penaltyCalc.CalculateTotal(sub.SourceSegments, greedyResult.Segments, &weights)
	smoothPenalty := s.penaltyCalc.CalculateTotal(sub.SourceSegments, smoothResult.Segments, &weights)

	fmt.Println("\n📊 COMPARISON: Greedy vs Smooth")
	fmt.Println("┌───────┬─────────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ Index │         Greedy                           │            Smooth                    │")
	fmt.Println("│       │  Start    End      Rate   Gap    Shift   │  Start    End      Rate   Gap    Shift   │")
	fmt.Println("├───────┼─────────────────────────────────────────────────────────────────────────────────┤")

	for i := 0; i < len(sub.SourceSegments); i++ {
		src := sub.SourceSegments[i]
		gSeg := greedyResult.Segments[i]
		sSeg := smoothResult.Segments[i]

		// 计算实际间隙
		gGap := time.Duration(0)
		sGap := time.Duration(0)
		if i < len(greedyResult.Segments)-1 {
			gGap = greedyResult.Segments[i+1].StartTime - gSeg.EndTime()
			sGap = smoothResult.Segments[i+1].StartTime - sSeg.EndTime()
		}

		// 位移
		gShift := gSeg.StartTime - src.OrigStart()
		sShift := sSeg.StartTime - src.OrigStart()

		fmt.Printf("│  %2d   │ %6.2fs  %6.2fs  %5.2f  %5.0fms %+6.0fms │ %6.2fs  %6.2fs  %5.2f  %5.0fms %+6.0fms │\n",
			i,
			float64(gSeg.StartTime)/float64(time.Second),
			float64(gSeg.EndTime())/float64(time.Second),
			gSeg.Rate,
			float64(gGap)/float64(time.Millisecond),
			float64(gShift)/float64(time.Millisecond),
			float64(sSeg.StartTime)/float64(time.Second),
			float64(sSeg.EndTime())/float64(time.Second),
			sSeg.Rate,
			float64(sGap)/float64(time.Millisecond),
			float64(sShift)/float64(time.Millisecond),
		)

		// 标记差异
		if gSeg.Rate != sSeg.Rate || gSeg.StartTime != sSeg.StartTime {
			t.Logf("⚠️  Segment %d differs: Greedy(Start=%v, Rate=%.3f) vs Smooth(Start=%v, Rate=%.3f)",
				i, gSeg.StartTime, gSeg.Rate, sSeg.StartTime, sSeg.Rate)
		}
	}

	fmt.Println("└───────┴─────────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println("\n📈 SUMMARY:")
	fmt.Printf("  Greedy Total Penalty: %.4f\n", greedyPenalty)
	fmt.Printf("  Smooth Total Penalty: %.4f\n", smoothPenalty)
	fmt.Printf("  Penalty Change:       %.4f (%.1f%%)\n", smoothPenalty-greedyPenalty,
		(smoothPenalty-greedyPenalty)/greedyPenalty*100)

	// 分析速率分布
	fmt.Println("\n📊 Rate Distribution:")
	gRates := make([]float64, len(greedyResult.Segments))
	sRates := make([]float64, len(smoothResult.Segments))
	for i, seg := range greedyResult.Segments {
		gRates[i] = seg.Rate
	}
	for i, seg := range smoothResult.Segments {
		sRates[i] = seg.Rate
	}
	fmt.Printf("  Greedy rates: %v\n", formatRates(gRates))
	fmt.Printf("  Smooth rates: %v\n", formatRates(sRates))

	// 检查约束
	fmt.Println("\n✅ Constraint Verification:")
	for i := 0; i < len(smoothResult.Segments); i++ {
		src := sub.SourceSegments[i]
		seg := smoothResult.Segments[i]

		// 窗口约束
		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		if seg.StartTime < windowMin-1*time.Millisecond || seg.StartTime > windowMax+1*time.Millisecond {
			t.Errorf("Segment %d: StartTime %v outside window [%v, %v]", i, seg.StartTime, windowMin, windowMax)
		}

		// 重叠检查
		if i > 0 {
			prevEnd := smoothResult.Segments[i-1].EndTime()
			gap := seg.StartTime - prevEnd
			if gap < cfg.MinGap-1*time.Millisecond {
				t.Errorf("Segment %d: Gap %v < MinGap %v (prev end=%v, this start=%v)",
					i, gap, cfg.MinGap, prevEnd, seg.StartTime)
			}
		}

		// 速率约束
		if seg.Rate < cfg.RateMin-0.001 || seg.Rate > cfg.RateMax+0.001 {
			t.Errorf("Segment %d: Rate %.3f outside [%.2f, %.2f]", i, seg.Rate, cfg.RateMin, cfg.RateMax)
		}
	}

	// 详细分析 Segment 0
	fmt.Println("\n🔍 Detailed Analysis for Segment 0:")
	src0 := sub.SourceSegments[0]
	tts0 := sub.TTSSegments[0]
	g0 := greedyResult.Segments[0]
	s0 := smoothResult.Segments[0]

	fmt.Printf("  Original slot:     %v ~ %v (duration: %v)\n", src0.OrigStart(), src0.OrigEnd(), src0.OrigDuration())
	fmt.Printf("  TTS NaturalLen:    %v\n", tts0.NaturalLen)
	fmt.Printf("  TTS > Slot?        %v\n", tts0.NaturalLen > src0.OrigDuration())
	fmt.Printf("  Next segment at:   %v\n", sub.SourceSegments[1].OrigStart())
	fmt.Println()
	fmt.Printf("  Greedy: Start=%v, End=%v, Rate=%.3f, PlayDuration=%v\n",
		g0.StartTime, g0.EndTime(), g0.Rate, g0.PlayDuration)
	fmt.Printf("          Gap to next: %v\n", greedyResult.Segments[1].StartTime-g0.EndTime())
	fmt.Println()
	fmt.Printf("  Smooth: Start=%v, End=%v, Rate=%.3f, PlayDuration=%v\n",
		s0.StartTime, s0.EndTime(), s0.Rate, s0.PlayDuration)
	fmt.Printf("          Gap to next: %v\n", smoothResult.Segments[1].StartTime-s0.EndTime())

	// 关键问题：减速是否导致下一个片段可用空间变小？
	fmt.Println("\n⚠️  Impact Analysis:")
	fmt.Printf("  If Greedy Rate=1.0: EndTime = %.3fs, Gap to Segment 1 = %.3fs\n",
		float64(g0.StartTime)/float64(time.Second)+float64(tts0.NaturalLen)/float64(time.Second),
		float64(sub.SourceSegments[1].OrigStart()-g0.StartTime-tts0.NaturalLen)/float64(time.Second))
	fmt.Printf("  Actual Smooth:      EndTime = %.3fs, Gap to Segment 1 = %.3fs\n",
		float64(s0.EndTime())/float64(time.Second),
		float64(smoothResult.Segments[1].StartTime-s0.EndTime())/float64(time.Second))
}

func formatRates(rates []float64) string {
	result := "["
	for i, r := range rates {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%.2f", r)
	}
	return result + "]"
}

// TestNeedSpeedChange_GreedyOnly 测试 need_speed_change 场景的纯贪心效果
func TestNeedSpeedChange_GreedyOnly(t *testing.T) {
	s := &solverImpl{
		penaltyCalc: penalty.NewPenaltyCalculator(),
	}
	cfg := timeline.NewDefaultDPConfig(timeline.ModeA)
	weights := timeline.NewDefaultPenaltyWeights(timeline.ModeA)

	// need_speed_change 数据
	sub := &timeline.SubProblem{
		SourceSegments: []timeline.SourceSegment{
			{Index: 0, Interval: timeline.TimeInterval{Start: 1000 * time.Millisecond, End: 3000 * time.Millisecond}},
			{Index: 1, Interval: timeline.TimeInterval{Start: 3500 * time.Millisecond, End: 5500 * time.Millisecond}},
			{Index: 2, Interval: timeline.TimeInterval{Start: 6000 * time.Millisecond, End: 8000 * time.Millisecond}},
		},
		TTSSegments: []timeline.TTSSegment{
			{Index: 0, NaturalLen: 2200 * time.Millisecond},
			{Index: 1, NaturalLen: 2300 * time.Millisecond},
			{Index: 2, NaturalLen: 2100 * time.Millisecond},
		},
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Scenario: need_speed_change (Mode A) - Greedy Only vs Smooth")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Config: WindowLeft=%v, WindowRight=%v, MinGap=%v\n", cfg.WindowLeft, cfg.WindowRight, cfg.MinGap)

	// 纯贪心结果
	greedyResult, err := s.SolveGreedy(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Greedy error: %v", err)
	}

	// 平滑结果
	smoothResult, err := s.SolveGreedySmooth(sub, &cfg, &weights)
	if err != nil {
		t.Fatalf("Smooth error: %v", err)
	}

	// 计算总惩罚
	greedyPenalty := s.penaltyCalc.CalculateTotal(sub.SourceSegments, greedyResult.Segments, &weights)
	smoothPenalty := s.penaltyCalc.CalculateTotal(sub.SourceSegments, smoothResult.Segments, &weights)

	fmt.Println("\n📊 DETAILED COMPARISON:")
	fmt.Println("┌───────┬───────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│       │              Greedy (No Post-processing)              │     Smooth       │")
	fmt.Println("│ Index │  Window      Start    End      Duration  Rate  Shift │  Start    Rate   │")
	fmt.Println("├───────┼───────────────────────────────────────────────────────────────────────────┤")

	for i := 0; i < len(sub.SourceSegments); i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]
		gSeg := greedyResult.Segments[i]
		sSeg := smoothResult.Segments[i]

		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		gShift := gSeg.StartTime - src.OrigStart()

		fmt.Printf("│  %d    │ [%4.0f,%4.0f]  %6.2fs  %6.2fs  %6.2fs  %4.2f  %+4.0fms │ %6.2fs  %4.2f  │\n",
			i,
			float64(windowMin)/float64(time.Millisecond),
			float64(windowMax)/float64(time.Millisecond),
			float64(gSeg.StartTime)/float64(time.Second),
			float64(gSeg.EndTime())/float64(time.Second),
			float64(gSeg.PlayDuration)/float64(time.Second),
			gSeg.Rate,
			float64(gShift)/float64(time.Millisecond),
			float64(sSeg.StartTime)/float64(time.Second),
			sSeg.Rate,
		)

		// 分析：是否可以 Rate=1.0?
		if i == 0 {
			fmt.Printf("│       │ TTS NaturalLen=%.2fs, Slot=%.2fs, TTS > Slot? %v                    │\n",
				float64(tts.NaturalLen)/float64(time.Second),
				float64(src.OrigDuration())/float64(time.Second),
				tts.NaturalLen > src.OrigDuration())
		}
	}
	fmt.Println("└───────┴───────────────────────────────────────────────────────────────────────────┘")

	fmt.Println("\n📈 PENALTY COMPARISON:")
	fmt.Printf("  Greedy Total Penalty: %.4f\n", greedyPenalty)
	fmt.Printf("  Smooth Total Penalty: %.4f\n", smoothPenalty)

	// 检查 Rate=1.0 是否可行
	fmt.Println("\n🔍 FEASIBILITY CHECK: Can we use Rate=1.0 for all?")
	fmt.Println("┌───────┬────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ Index │  Analysis                                                      │")

	currentTime := timeline.Duration(0)
	allFeasible := true
	for i := 0; i < len(sub.SourceSegments); i++ {
		src := sub.SourceSegments[i]
		tts := sub.TTSSegments[i]

		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		// 最早开始时间
		minStart := currentTime + cfg.MinGap
		if i == 0 {
			minStart = windowMin
		} else {
			if windowMin > minStart {
				minStart = windowMin
			}
		}

		// Rate=1.0 时的结束时间
		endTime := minStart + tts.NaturalLen

		// 检查是否在窗口内
		inWindow := minStart >= windowMin && minStart <= windowMax

		// 下一段的约束
		var nextConstraint string
		if i < len(sub.SourceSegments)-1 {
			nextSrc := sub.SourceSegments[i+1]
			nextWindowMin := nextSrc.OrigStart() - cfg.WindowLeft
			if endTime+cfg.MinGap <= nextWindowMin {
				nextConstraint = "✓ fits before next window"
			} else {
				nextConstraint = fmt.Sprintf("⚠ next window starts at %.2fs", float64(nextWindowMin)/float64(time.Second))
				allFeasible = false
			}
		} else {
			nextConstraint = "(last segment)"
		}

		fmt.Printf("│  %d    │  Start=%.2fs (window [%.1f,%.1f]) %s  End=%.2fs  %s │\n",
			i,
			float64(minStart)/float64(time.Second),
			float64(windowMin)/float64(time.Second),
			float64(windowMax)/float64(time.Second),
			func() string {
				if inWindow {
					return "✓"
				}
				return "✗"
			}(),
			float64(endTime)/float64(time.Second),
			nextConstraint,
		)

		currentTime = endTime
	}
	fmt.Println("└───────┴────────────────────────────────────────────────────────────────┘")

	if allFeasible {
		fmt.Println("\n✅ CONCLUSION: Rate=1.0 IS FEASIBLE for all segments!")
		fmt.Println("   Greedy is SUBOPTIMAL - it applies unnecessary compression.")
	} else {
		fmt.Println("\n❌ CONCLUSION: Rate=1.0 is NOT feasible - compression required.")
	}

	// 验证贪心结果约束
	fmt.Println("\n✅ Verifying Greedy constraints:")
	for i := 0; i < len(greedyResult.Segments); i++ {
		src := sub.SourceSegments[i]
		seg := greedyResult.Segments[i]

		windowMin := src.OrigStart() - cfg.WindowLeft
		if windowMin < 0 {
			windowMin = 0
		}
		windowMax := src.OrigStart() + cfg.WindowRight

		// 窗口约束
		if seg.StartTime < windowMin-1*time.Millisecond || seg.StartTime > windowMax+1*time.Millisecond {
			t.Errorf("Segment %d: StartTime %v outside window [%v, %v]", i, seg.StartTime, windowMin, windowMax)
		}

		// 重叠检查
		if i > 0 {
			prevEnd := greedyResult.Segments[i-1].EndTime()
			gap := seg.StartTime - prevEnd
			if gap < cfg.MinGap-1*time.Millisecond {
				t.Errorf("Segment %d: Gap %v < MinGap %v", i, gap, cfg.MinGap)
			}
		}
	}
}
