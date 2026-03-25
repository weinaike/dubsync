package timeline

// DegradationHandler 降级处理器接口
// 负责处理无解场景，提供三层兜底策略
type DegradationHandler interface {
	// DetectBottleneck 检测瓶颈片段
	// 返回导致无解的片段索引列表
	DetectBottleneck(sub *SubProblem, cfg *DPConfig) []int

	// RelaxConstraints 约束松弛（第一层）
	// 针对瓶颈片段放宽搜索窗口、最大变速、平滑约束
	RelaxConstraints(cfg *DPConfig, bottleneckIndices []int) *DPConfig

	// RequestNegotiation 请求协商（第二层）
	// 计算每个瓶颈片段的时长预算，生成 TTS 协商请求
	RequestNegotiation(sub *SubProblem, cfg *DPConfig) *NegotiationRequest

	// HardFallback 硬降级（第三层）
	// 执行句尾静音裁剪、智能截断或超范围变速
	// 返回强行编排的 Blueprint 和质量告警
	HardFallback(sub *SubProblem, cfg *DPConfig) (*Blueprint, *QualityAlert)
}
