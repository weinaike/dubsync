package timeline

import "time"

// NewDefaultConfig 根据模式创建默认调度器配置
func NewDefaultConfig(mode Mode) SchedulerConfig {
	return SchedulerConfig{
		Mode:           mode,
		Weights:        NewDefaultPenaltyWeights(mode),
		DP:             NewDefaultDPConfig(mode),
		TotalDuration:  0, // 由调用方设置
		MaxNegotiation: 2, // 最多 2 轮协商
	}
}

// NewDefaultPenaltyWeights 根据模式创建默认惩罚权重
// 参考文档中的权重推荐初始值
func NewDefaultPenaltyWeights(mode Mode) PenaltyWeights {
	switch mode {
	case ModeA:
		// 视频模式：位移惩罚高，平滑惩罚高
		return PenaltyWeights{
			WShift:  10.0, // /秒
			WSmooth: 5.0,
			WGap:    1.0,
			// 加速方向（R >= 1.0）
			SpeedBandsAccel: []SpeedPenaltyBand{
				{RateUpperBound: 1.05, MarginalSlope: 1.0},   // 几乎无感
				{RateUpperBound: 1.15, MarginalSlope: 3.0},   // 可接受
				{RateUpperBound: 1.25, MarginalSlope: 8.0},   // 显著不自然
				{RateUpperBound: 1.40, MarginalSlope: 20.0},  // 仅紧急情况
			},
			// 减速方向（R < 1.0）
			SpeedBandsDecel: []SpeedPenaltyBand{
				{RateUpperBound: 0.95, MarginalSlope: 2.0},   // 轻微减速
				{RateUpperBound: 1.00, MarginalSlope: 1.0},   // 接近原速
			},
		}
	case ModeB:
		// 播客模式：位移惩罚低，变速惩罚高
		return PenaltyWeights{
			WShift:  0.5, // /秒
			WSmooth: 2.0,
			WGap:    3.0, // 更在意间隙膨胀
			// 加速方向（R >= 1.0）—— 惩罚更高
			SpeedBandsAccel: []SpeedPenaltyBand{
				{RateUpperBound: 1.05, MarginalSlope: 5.0},   // 已经开始影响听感
				{RateUpperBound: 1.15, MarginalSlope: 15.0},  // 明显机械感
				{RateUpperBound: 1.25, MarginalSlope: 50.0},  // 严重破坏听感
				{RateUpperBound: 1.40, MarginalSlope: 200.0}, // 几乎禁止
			},
			// 减速方向（R < 1.0）—— 更能容忍轻微慢速
			SpeedBandsDecel: []SpeedPenaltyBand{
				{RateUpperBound: 0.95, MarginalSlope: 1.0},
				{RateUpperBound: 1.00, MarginalSlope: 0.5},
			},
		}
	default:
		return NewDefaultPenaltyWeights(ModeA)
	}
}

// NewDefaultDPConfig 根据模式创建默认 DP 配置
func NewDefaultDPConfig(mode Mode) DPConfig {
	switch mode {
	case ModeA:
		// 视频模式：窄搜索窗口，严守同步
		return DPConfig{
			TimeStep:      10 * time.Millisecond,
			RateMin:       1.0,  // 不允许减速
			RateMax:       1.4,
			RateStep:      0.01,
			WindowLeft:    500 * time.Millisecond,
			WindowRight:   500 * time.Millisecond,
			SmoothDelta:   0.15, // |R[i+1] - R[i]| <= 0.15
			MinGap:        50 * time.Millisecond,
			BreakThreshold: 1500 * time.Millisecond,
			TopK:          5,
		}
	case ModeB:
		// 播客模式：宽右窗口，允许大幅后推
		return DPConfig{
			TimeStep:      10 * time.Millisecond,
			RateMin:       0.95, // 允许轻微减速
			RateMax:       1.4,
			RateStep:      0.01,
			WindowLeft:    500 * time.Millisecond,
			WindowRight:   5000 * time.Millisecond, // 允许后推 5 秒
			SmoothDelta:   0.15,
			MinGap:        50 * time.Millisecond,
			BreakThreshold: 1500 * time.Millisecond,
			TopK:          5,
		}
	default:
		return NewDefaultDPConfig(ModeA)
	}
}

// Validate 验证 SchedulerConfig 有效性
func (c *SchedulerConfig) Validate() error {
	if c.DP.TimeStep <= 0 {
		return NewScheduleError(ErrInvalidInput, -1, "TimeStep must be positive")
	}
	if c.DP.RateMin <= 0 || c.DP.RateMax < c.DP.RateMin {
		return NewScheduleError(ErrInvalidInput, -1, "invalid RateMin/RateMax")
	}
	if c.DP.RateStep <= 0 {
		return NewScheduleError(ErrInvalidInput, -1, "RateStep must be positive")
	}
	if c.DP.MinGap < 0 {
		return NewScheduleError(ErrInvalidInput, -1, "MinGap cannot be negative")
	}
	return nil
}
