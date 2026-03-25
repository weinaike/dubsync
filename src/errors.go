package timeline

import (
	"errors"
	"fmt"
)

// 核心错误类型
var (
	// ErrNoSolution 无可行解
	// 所有降级策略都无法找到满足约束的排期
	ErrNoSolution = errors.New("no feasible solution found")

	// ErrInvalidInput 输入无效
	// 输入数据不满足基本要求（如片段索引不连续、时间为负等）
	ErrInvalidInput = errors.New("invalid input")

	// ErrSegmentsMismatch 片段数量不匹配
	// SourceSegments 和 TTSSegments 数量不一致
	ErrSegmentsMismatch = errors.New("source and TTS segments count mismatch")

	// ErrTotalDurationExceeded 超出总时长
	// 即使最大压缩也无法在 TotalDuration 内排完
	ErrTotalDurationExceeded = errors.New("total duration exceeded")
)

// ScheduleError 调度错误（包含详细信息）
type ScheduleError struct {
	Err       error     // 底层错误
	Segment   int       // 相关片段索引（-1 表示全局）
	Reason    string    // 详细原因
	Bottlenecks []int   // 瓶颈片段列表
}

func (e *ScheduleError) Error() string {
	if e.Segment >= 0 {
		return fmt.Sprintf("segment %d: %s: %v", e.Segment, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *ScheduleError) Unwrap() error {
	return e.Err
}

// NewScheduleError 创建调度错误
func NewScheduleError(err error, segment int, reason string) *ScheduleError {
	return &ScheduleError{
		Err:     err,
		Segment: segment,
		Reason:  reason,
	}
}

// NewScheduleErrorWithBottlenecks 创建带瓶颈信息的调度错误
func NewScheduleErrorWithBottlenecks(err error, reason string, bottlenecks []int) *ScheduleError {
	return &ScheduleError{
		Err:         err,
		Segment:     -1,
		Reason:      reason,
		Bottlenecks: bottlenecks,
	}
}
