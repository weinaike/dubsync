package testutil

import (
	"testing"
	"time"

	"github.com/weinaike/timeline/src"
)

// AssertBlueprintValid 验证蓝图基本有效性
// 检查片段索引连续、时间非负、结束时间 > 起始时间
func AssertBlueprintValid(t *testing.T, bp *timeline.Blueprint, sources []timeline.SourceSegment) {
	t.Helper()

	if bp == nil {
		t.Fatal("blueprint is nil")
	}

	if len(bp.Segments) != len(sources) {
		t.Fatalf("segment count mismatch: got %d, want %d", len(bp.Segments), len(sources))
	}

	for i, seg := range bp.Segments {
		if seg.Index != i {
			t.Errorf("segment %d: index mismatch, got %d", i, seg.Index)
		}
		if seg.StartTime < 0 {
			t.Errorf("segment %d: negative start time %v", i, seg.StartTime)
		}
		if seg.PlayDuration <= 0 {
			t.Errorf("segment %d: non-positive duration %v", i, seg.PlayDuration)
		}
		if seg.Rate <= 0 {
			t.Errorf("segment %d: non-positive rate %v", i, seg.Rate)
		}
	}
}

// AssertNoOverlap 验证片段不重叠
func AssertNoOverlap(t *testing.T, plans []timeline.SegmentPlan) {
	t.Helper()

	for i := 1; i < len(plans); i++ {
		prevEnd := plans[i-1].StartTime + plans[i-1].PlayDuration
		currStart := plans[i].StartTime
		if currStart < prevEnd {
			t.Errorf("segments %d and %d overlap: prev_end=%v, curr_start=%v, overlap=%v",
				i-1, i, prevEnd, currStart, prevEnd-currStart)
		}
	}
}

// AssertMinGap 验证最小间隙
func AssertMinGap(t *testing.T, plans []timeline.SegmentPlan, minGap time.Duration) {
	t.Helper()

	for i := 1; i < len(plans); i++ {
		prevEnd := plans[i-1].StartTime + plans[i-1].PlayDuration
		currStart := plans[i].StartTime
		gap := currStart - prevEnd
		if gap < minGap {
			t.Errorf("gap between segments %d and %d is %v, want >= %v",
				i-1, i, gap, minGap)
		}
	}
}

// AssertRateInRange 验证变速率在范围内
func AssertRateInRange(t *testing.T, plans []timeline.SegmentPlan, min, max float64) {
	t.Helper()

	for i, seg := range plans {
		if seg.Rate < min || seg.Rate > max {
			t.Errorf("segment %d: rate %v out of range [%v, %v]", i, seg.Rate, min, max)
		}
	}
}

// AssertShiftWithin 验证位移在范围内
func AssertShiftWithin(t *testing.T, plans []timeline.SegmentPlan, sources []timeline.SourceSegment, maxShift time.Duration) {
	t.Helper()

	for i, seg := range plans {
		origStart := sources[i].OrigStart()
		shift := seg.StartTime - origStart
		if shift < 0 {
			shift = -shift
		}
		if shift > maxShift {
			t.Errorf("segment %d: shift %v exceeds max %v", i, shift, maxShift)
		}
	}
}

// AssertPlayDurationCorrect 验证播放时长计算正确
// D[i] = L_trans[i] / R[i]
func AssertPlayDurationCorrect(t *testing.T, plans []timeline.SegmentPlan, tts []timeline.TTSSegment, tolerance time.Duration) {
	t.Helper()

	for i, seg := range plans {
		expected := time.Duration(float64(tts[i].NaturalLen) / seg.Rate)
		diff := seg.PlayDuration - expected
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("segment %d: play duration %v, expected %v (diff %v > tolerance %v)",
				i, seg.PlayDuration, expected, diff, tolerance)
		}
	}
}

// AssertWithinTotalDuration 验证所有片段在总时长内
func AssertWithinTotalDuration(t *testing.T, plans []timeline.SegmentPlan, totalDuration time.Duration) {
	t.Helper()

	if len(plans) == 0 {
		return
	}

	lastSeg := plans[len(plans)-1]
	endTime := lastSeg.StartTime + lastSeg.PlayDuration
	if endTime > totalDuration {
		t.Errorf("last segment ends at %v, exceeds total duration %v", endTime, totalDuration)
	}
}

// AssertStatusEquals 验证蓝图状态
func AssertStatusEquals(t *testing.T, bp *timeline.Blueprint, expected timeline.ScheduleStatus) {
	t.Helper()

	if bp.Status != expected {
		t.Errorf("status = %v, want %v", bp.Status, expected)
	}
}

// AssertDegradationLevel 验证降级等级
func AssertDegradationLevel(t *testing.T, bp *timeline.Blueprint, expectedLevel int) {
	t.Helper()

	if bp.DegradationLevel != expectedLevel {
		t.Errorf("degradation level = %d, want %d", bp.DegradationLevel, expectedLevel)
	}
}

// AssertHasWarnings 验证有警告
func AssertHasWarnings(t *testing.T, bp *timeline.Blueprint) {
	t.Helper()

	hasWarnings := false
	for _, seg := range bp.Segments {
		if seg.Degraded || seg.QualityAlert != nil {
			hasWarnings = true
			break
		}
	}
	if !hasWarnings && len(bp.InfeasibleSegments) == 0 {
		t.Error("expected warnings but found none")
	}
}
