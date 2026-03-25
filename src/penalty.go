package timeline

// PenaltyCalculator 惩罚计算器接口
// 负责计算目标函数中的各项惩罚值
type PenaltyCalculator interface {
	// CalculateTotal 计算排期总惩罚
	// 包括位移惩罚、语速惩罚、平滑惩罚、间隙惩罚
	CalculateTotal(sources []SourceSegment, plans []SegmentPlan, weights *PenaltyWeights) float64

	// ShiftPenalty 位移惩罚
	// shift = |S[i] - T_orig_start[i]|
	// 惩罚 = W_shift * shift（秒）
	ShiftPenalty(shift Duration, weights *PenaltyWeights) float64

	// SpeedPenalty 语速惩罚（分段线性）
	// 根据 weights 中的 SpeedBandsAccel/SpeedBandsDecel 计算
	SpeedPenalty(rate float64, weights *PenaltyWeights) float64

	// SmoothPenalty 语速平滑惩罚
	// 惩罚 = W_smooth * |rate1 - rate2|
	SmoothPenalty(rate1, rate2 float64, weights *PenaltyWeights) float64

	// GapPenalty 间隙惩罚（双向）
	// 惩罚 = W_gap * |actualGap - originalGap|
	// 双向惩罚：间隙变大或变小都会被惩罚
	GapPenalty(originalGap, actualGap Duration, weights *PenaltyWeights) float64
}
