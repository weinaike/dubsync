package timeline

import "time"

// =====================================================================
// 核心领域类型
// =====================================================================

// Duration 使用毫秒精度的时间长度，避免浮点误差
type Duration = time.Duration

// TimeInterval 表示一个通用的时间区间 [Start, End)
type TimeInterval struct {
	Start Duration // 区间起始时间（含）
	End   Duration // 区间结束时间（不含）
}

// Length 返回区间长度
func (ti TimeInterval) Length() Duration {
	return ti.End - ti.Start
}

// Overlaps 检测两个区间是否重叠
func (ti TimeInterval) Overlaps(other TimeInterval) bool {
	return ti.Start < other.End && other.Start < ti.End
}

// OverlapAmount 计算两个区间的重叠量
func (ti TimeInterval) OverlapAmount(other TimeInterval) Duration {
	overlapStart := ti.Start
	if other.Start > overlapStart {
		overlapStart = other.Start
	}
	overlapEnd := ti.End
	if other.End < overlapEnd {
		overlapEnd = other.End
	}
	if overlapEnd > overlapStart {
		return overlapEnd - overlapStart
	}
	return 0
}

// =====================================================================
// 输入数据结构
// =====================================================================

// SourceSegment 原始音频中的一个语音片段（来自 VAD 检测结果）
// 对应文档变量：T_orig_start[i], T_orig_end[i], L_orig[i]
type SourceSegment struct {
	Index    int          // 片段索引 i
	Interval TimeInterval // 原始起止时间 [T_orig_start[i], T_orig_end[i])
}

// OrigDuration 返回原始片段长度 L_orig[i]
func (s SourceSegment) OrigDuration() Duration {
	return s.Interval.Length()
}

// OrigStart 返回 T_orig_start[i]（理想目标位置）
func (s SourceSegment) OrigStart() Duration {
	return s.Interval.Start
}

// OrigEnd 返回 T_orig_end[i]
func (s SourceSegment) OrigEnd() Duration {
	return s.Interval.End
}

// TTSSegment TTS 生成的翻译音频片段
// 对应文档变量：L_trans[i]
type TTSSegment struct {
	Index        int      // 片段索引 i，与 SourceSegment.Index 对应
	NaturalLen   Duration // TTS 自然语速生成的音频长度 L_trans[i]
	AudioFileRef string   // TTS 音频文件的引用路径（供 Step 3 音频处理使用）
}

// =====================================================================
// 业务模式与配置
// =====================================================================

// Mode 业务场景模式
type Mode int

const (
	// ModeA 视频配音模式——音画同步优先
	// "宁可轻微变速，也不能声画错位"
	ModeA Mode = iota

	// ModeB 播客/纯音频翻译模式——自然听感优先
	// "宁可无限挪移，也绝不破坏音色和语速"
	ModeB
)

// String 返回模式的可读名称
func (m Mode) String() string {
	switch m {
	case ModeA:
		return "ModeA(Video)"
	case ModeB:
		return "ModeB(Podcast)"
	default:
		return "Unknown"
	}
}

// SpeedPenaltyBand 分段线性语速惩罚的一个区间段
// 对应文档中的"非线性语速惩罚设计"表格
type SpeedPenaltyBand struct {
	RateUpperBound float64 // 速率区间上界（含）
	MarginalSlope  float64 // 该区间的边际惩罚斜率
}

// PenaltyWeights 目标函数中的各项惩罚权重
// 对应文档中的 W_shift, W_smooth, W_gap 以及 f_speed 配置
type PenaltyWeights struct {
	WShift  float64 // 位移惩罚权重（单位：每秒）
	WSmooth float64 // 语速平滑惩罚权重
	WGap    float64 // 间隙惩罚权重

	// 分段线性语速惩罚配置（加速方向，R >= 1.0）
	// 必须按 RateUpperBound 升序排列
	SpeedBandsAccel []SpeedPenaltyBand

	// 分段线性语速惩罚配置（减速方向，R < 1.0）
	// 必须按 RateUpperBound 降序排列（即从 1.0 向下）
	SpeedBandsDecel []SpeedPenaltyBand
}

// DPConfig 动态规划求解器的参数配置
type DPConfig struct {
	TimeStep Duration // 时间离散化步长 T_step（推荐 10ms）
	RateMin  float64  // 最小变速倍率（如 0.95 或 1.0）
	RateMax  float64  // 最大变速倍率（如 1.4）
	RateStep float64  // 变速枚举步长（推荐 0.01）

	// 搜索窗口
	WindowLeft  Duration // S[i] 左偏移搜索范围 W_left
	WindowRight Duration // S[i] 右偏移搜索范围 W_right

	// 语速平滑硬约束 |R[i+1] - R[i]| <= Delta
	// 设为 0 表示不启用硬约束（仅靠目标函数中的 WSmooth 软惩罚）
	SmoothDelta float64

	// 最小句间间隙 G_min（推荐 50~100ms）
	MinGap Duration

	// 问题分解阈值 T_break（推荐 1.5s）
	BreakThreshold Duration

	// DP 剪枝：每个 (i, s) 保留的 top-K 个 R 选择
	TopK int
}

// SchedulerConfig 调度器完整配置
type SchedulerConfig struct {
	Mode           Mode           // 业务场景模式
	Weights        PenaltyWeights // 惩罚权重
	DP             DPConfig       // DP 求解器参数
	TotalDuration  Duration       // 总时长上限（视频模式下为视频时长；播客模式可设为 0 表示不限）
	MaxNegotiation int            // 最大协商轮次（推荐 2~3）
}

// =====================================================================
// 求解器输出：编排蓝图
// =====================================================================

// SegmentPlan 单个片段的编排结果
// 对应文档中的求解变量与派生变量
type SegmentPlan struct {
	Index int // 片段索引 i

	// 求解变量
	StartTime Duration // S[i] — 最终决定的起始时间
	Rate      float64  // R[i] — 最终决定的压缩/拉伸倍率（1.0=不变速）

	// 派生变量
	PlayDuration Duration // D[i] = L_trans[i] / R[i] — 实际播放时长

	// 质量标记
	Degraded     bool            // 是否触发了降级策略
	Degradation  DegradationType // 降级类型（如有）
	QualityAlert *QualityAlert   // 质量告警信息（如有）
}

// EndTime 返回该片段的结束时间
func (sp SegmentPlan) EndTime() Duration {
	return sp.StartTime + sp.PlayDuration
}

// DegradationType 降级措施类型
type DegradationType int

const (
	DegradationNone          DegradationType = iota
	DegradationTrimSilence                   // 句尾静音裁剪
	DegradationSmartTruncate                 // 智能截断
	DegradationOverspeed                     // 超范围变速（R > RateMax）
)

// String 返回降级类型的可读名称
func (d DegradationType) String() string {
	switch d {
	case DegradationNone:
		return "none"
	case DegradationTrimSilence:
		return "trim_trailing_silence"
	case DegradationSmartTruncate:
		return "smart_truncate"
	case DegradationOverspeed:
		return "overspeed"
	default:
		return "unknown"
	}
}

// QualityAlert 质量告警（单个片段级别）
type QualityAlert struct {
	SegmentIndex int     // 片段索引
	Reason       string  // 告警原因描述
	Impact       string  // 质量影响等级：low / medium / high
	Rate         float64 // 实际使用的变速倍率
}

// =====================================================================
// 编排蓝图与整体调度结果
// =====================================================================

// ScheduleStatus 调度结果状态
type ScheduleStatus string

const (
	StatusOK       ScheduleStatus = "ok"       // 正常求解
	StatusDegraded ScheduleStatus = "degraded" // 部分片段降级
	StatusFailed   ScheduleStatus = "failed"   // 完全无解
)

// Blueprint 完整的编排蓝图——Step 2 的输出，Step 3 & 4 的输入
type Blueprint struct {
	Status   ScheduleStatus // 整体状态
	Segments []SegmentPlan  // 所有片段的编排结果（按索引升序）

	// 降级信息（仅当 Status == StatusDegraded 时非空）
	InfeasibleSegments []InfeasibleInfo // 无法正常求解的片段列表
	DegradationLevel   int              // 降级严重程度（1~3）
	Recommendation     string           // 建议措施
}

// InfeasibleInfo 无解片段的诊断信息
// 对应文档中的结构化告警输出
type InfeasibleInfo struct {
	Index              int    // 片段索引
	Reason             string // 无解原因（如 "L_trans=3.2s but max_available=2.1s"）
	DegradationApplied string // 实际采用的降级措施
	QualityImpact      string // 质量影响等级
}

// NegotiationRequest TTS 协商请求（第二层兜底）
// 当调度器发现某些片段不可行时，向上游 TTS 管线发送时长约束
type NegotiationRequest struct {
	Round    int             // 当前协商轮次
	Segments []SegmentBudget // 需要重新生成的瓶颈片段
}

// SegmentBudget 单个片段的时长预算约束
type SegmentBudget struct {
	Index         int      // 片段索引
	MaxAllowedLen Duration // D_budget[i] × R_max — TTS 生成长度上限
	CurrentLen    Duration // 当前 L_trans[i]
}

// =====================================================================
// 子问题分解
// =====================================================================

// SubProblem 分解后的独立子问题
// 对应文档中"按自然断点分解问题"的预处理步骤
type SubProblem struct {
	ID             int             // 子问题编号
	SourceSegments []SourceSegment // 该子问题包含的原始片段
	TTSSegments    []TTSSegment    // 对应的 TTS 片段
}

// =====================================================================
// Pipeline 输入/输出汇总
// =====================================================================

// PipelineInput Step 2 全局排期的完整输入
type PipelineInput struct {
	SourceSegments []SourceSegment // 所有原始语音片段（VAD 输出）
	TTSSegments    []TTSSegment    // 所有 TTS 片段（Step 1 输出）
	Config         SchedulerConfig // 调度配置
}

// AudioProcessTask Step 3 单个片段的音频处理任务
type AudioProcessTask struct {
	SegmentIndex int      // 片段索引
	AudioFileRef string   // 输入 TTS 音频文件引用
	TargetRate   float64  // 目标变速倍率 R[i]
	TargetLen    Duration // 目标播放时长 D[i]
}

// MixTask Step 4 最终混音任务
type MixTask struct {
	TotalDuration Duration         // 输出音轨总时长
	Placements    []AudioPlacement // 所有片段的放置信息
	DuckingConfig DuckingConfig    // Ducking 配置
}

// AudioPlacement 单个片段在最终音轨上的放置位置
type AudioPlacement struct {
	SegmentIndex int      // 片段索引
	AudioFileRef string   // 处理后的音频文件引用
	StartTime    Duration // 放置起始时间 S[i]
	Duration     Duration // 播放时长 D[i]
}

// DuckingConfig 背景音自动衰减配置
type DuckingConfig struct {
	AttenuationDB float64  // 衰减量（dB）
	AttackMs      Duration // attack 时间
	ReleaseMs     Duration // release 时间
}
