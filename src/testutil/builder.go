package testutil

import (
	"time"

	"github.com/wnk/timeline/src"
)

// TestDataBuilder 测试数据构建器
// 用于方便地构建 PipelineInput 进行测试
type TestDataBuilder struct {
	segments    []segmentSpec
	mode        timeline.Mode
	totalDur    time.Duration
	config      *timeline.SchedulerConfig
}

type segmentSpec struct {
	origStart time.Duration
	origEnd   time.Duration
	transLen  time.Duration
}

// NewTestDataBuilder 创建新的测试数据构建器
func NewTestDataBuilder() *TestDataBuilder {
	return &TestDataBuilder{
		segments: make([]segmentSpec, 0),
		mode:     timeline.ModeA,
		totalDur: 60 * time.Second,
	}
}

// Mode 设置业务模式
func (b *TestDataBuilder) Mode(mode timeline.Mode) *TestDataBuilder {
	b.mode = mode
	return b
}

// ModeA 设置为视频模式
func (b *TestDataBuilder) ModeA() *TestDataBuilder {
	return b.Mode(timeline.ModeA)
}

// ModeB 设置为播客模式
func (b *TestDataBuilder) ModeB() *TestDataBuilder {
	return b.Mode(timeline.ModeB)
}

// TotalDuration 设置总时长
func (b *TestDataBuilder) TotalDuration(d time.Duration) *TestDataBuilder {
	b.totalDur = d
	return b
}

// AddSegment 添加一个片段
func (b *TestDataBuilder) AddSegment(origStart, origEnd, transLen time.Duration) *TestDataBuilder {
	b.segments = append(b.segments, segmentSpec{
		origStart: origStart,
		origEnd:   origEnd,
		transLen:  transLen,
	})
	return b
}

// Config 设置自定义配置
func (b *TestDataBuilder) Config(cfg *timeline.SchedulerConfig) *TestDataBuilder {
	b.config = cfg
	return b
}

// Build 构建 PipelineInput
func (b *TestDataBuilder) Build() *timeline.PipelineInput {
	sources := make([]timeline.SourceSegment, len(b.segments))
	tts := make([]timeline.TTSSegment, len(b.segments))

	for i, spec := range b.segments {
		sources[i] = timeline.SourceSegment{
			Index: i,
			Interval: timeline.TimeInterval{
				Start: spec.origStart,
				End:   spec.origEnd,
			},
		}
		tts[i] = timeline.TTSSegment{
			Index:        i,
			NaturalLen:   spec.transLen,
			AudioFileRef: "", // 测试中不需要实际文件
		}
	}

	cfg := b.config
	if cfg == nil {
		defaultCfg := timeline.NewDefaultConfig(b.mode)
		defaultCfg.TotalDuration = b.totalDur
		cfg = defaultCfg
	}

	return &timeline.PipelineInput{
		SourceSegments: sources,
		TTSSegments:    tts,
		Config:         *cfg,
	}
}

// ============================================================
// 预设场景快捷方法
// ============================================================

// PerfectMatch T1: 完美匹配场景
// 翻译长度与原始长度几乎相同，无需挪移或变速
func (b *TestDataBuilder) PerfectMatch() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(30 * time.Second).
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 2000*time.Millisecond).
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2000*time.Millisecond).
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 2000*time.Millisecond).
		Build()
}

// NeedSpeedChange T2: 需要轻微变速场景
// 翻译略长于原始，需要压缩约 10%
func (b *TestDataBuilder) NeedSpeedChange() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(30 * time.Second).
		// 原始 2s，翻译 2.2s，需要 R ≈ 1.1
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 2200*time.Millisecond).
		// 原始 2s，翻译 2.3s，需要 R ≈ 1.15
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2300*time.Millisecond).
		// 原始 2s，翻译 2.1s，需要 R ≈ 1.05
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 2100*time.Millisecond).
		Build()
}

// NeedShift T3: 需要显著挪移场景
// 翻译明显更长，需要向右推移
func (b *TestDataBuilder) NeedShift() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(40 * time.Second).
		// 原始 2s，翻译 3s
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 3000*time.Millisecond).
		// 原始 2s，翻译 2.8s
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2800*time.Millisecond).
		// 原始 2s，翻译 3.2s
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 3200*time.Millisecond).
		// 原始 2s，翻译 2.9s
		AddSegment(8500*time.Millisecond, 10500*time.Millisecond, 2900*time.Millisecond).
		// 原始 2s，翻译 3.1s
		AddSegment(11000*time.Millisecond, 13000*time.Millisecond, 3100*time.Millisecond).
		Build()
}

// DominoPush T4: 多米诺推移场景（ModeB）
// 播客模式，长翻译，需多米诺式推移，保持 R=1.0
func (b *TestDataBuilder) DominoPush() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeB().
		TotalDuration(60 * time.Second).
		// 原始 2s，翻译 4s，会被大幅后推
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 4000*time.Millisecond).
		// 原始 2s，翻译 3.5s
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 3500*time.Millisecond).
		// 原始 2s，翻译 4.2s
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 4200*time.Millisecond).
		// 原始 2s，翻译 3.8s
		AddSegment(8500*time.Millisecond, 10500*time.Millisecond, 3800*time.Millisecond).
		// 原始 2s，翻译 4s
		AddSegment(11000*time.Millisecond, 13000*time.Millisecond, 4000*time.Millisecond).
		Build()
}

// MixedScenario T5: 混合场景
// 部分片段需要变速，部分需要挪移
func (b *TestDataBuilder) MixedScenario() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(50 * time.Second).
		// 完美匹配
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 2000*time.Millisecond).
		// 需要轻微变速
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2300*time.Millisecond).
		// 需要显著挪移
		AddSegment(6000*time.Millisecond, 7500*time.Millisecond, 2800*time.Millisecond).
		// 完美匹配
		AddSegment(8000*time.Millisecond, 10000*time.Millisecond, 2000*time.Millisecond).
		// 需要加速
		AddSegment(10500*time.Millisecond, 12000*time.Millisecond, 2400*time.Millisecond).
		// 完美匹配
		AddSegment(12500*time.Millisecond, 14500*time.Millisecond, 2000*time.Millisecond).
		// 需要挪移
		AddSegment(15000*time.Millisecond, 16500*time.Millisecond, 2600*time.Millisecond).
		// 完美匹配
		AddSegment(17000*time.Millisecond, 19000*time.Millisecond, 2000*time.Millisecond).
		Build()
}

// Infeasible T6: 无解场景
// 翻译过长，即使最大压缩也无法放入
func (b *TestDataBuilder) Infeasible() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(15 * time.Second).
		// 原始 2s，翻译 5s（即使 R=1.4 也需要 3.57s）
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 5000*time.Millisecond).
		// 原始 2s，翻译 4.5s
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 4500*time.Millisecond).
		// 原始 2s，翻译 4.8s
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 4800*time.Millisecond).
		Build()
}

// WithBreakPoints T7: 自然断点场景
// 包含多个自然断点，可分解为多个子问题
func (b *TestDataBuilder) WithBreakPoints() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeB().
		TotalDuration(80 * time.Second).
		// 第一组：3 个紧密片段
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 2200*time.Millisecond).
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2300*time.Millisecond).
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 2100*time.Millisecond).
		// 断点：2s 间隙
		// 第二组：3 个片段
		AddSegment(10000*time.Millisecond, 12000*time.Millisecond, 2400*time.Millisecond).
		AddSegment(12500*time.Millisecond, 14500*time.Millisecond, 2300*time.Millisecond).
		AddSegment(15000*time.Millisecond, 17000*time.Millisecond, 2500*time.Millisecond).
		// 断点：2.5s 间隙
		// 第三组：4 个片段
		AddSegment(20000*time.Millisecond, 22000*time.Millisecond, 2100*time.Millisecond).
		AddSegment(22500*time.Millisecond, 24500*time.Millisecond, 2200*time.Millisecond).
		AddSegment(25000*time.Millisecond, 27000*time.Millisecond, 2300*time.Millisecond).
		AddSegment(27500*time.Millisecond, 29500*time.Millisecond, 2400*time.Millisecond).
		Build()
}

// ============================================================
// 边界场景 (Boundary Scenarios)
// ============================================================

// SingleSegment B1: 单片段场景
// 最简单的边界情况，只有一个语音片段
func (b *TestDataBuilder) SingleSegment() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(10 * time.Second).
		AddSegment(2000*time.Millisecond, 5000*time.Millisecond, 3200*time.Millisecond).
		Build()
}

// ContinuousSpeech B2: 连续语音场景
// 片段间隙极小（<50ms），模拟连珠炮式说话
func (b *TestDataBuilder) ContinuousSpeech() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(20 * time.Second).
		// 间隙 20ms
		AddSegment(1000*time.Millisecond, 3500*time.Millisecond, 2800*time.Millisecond).
		// 间隙 30ms
		AddSegment(3520*time.Millisecond, 6030*time.Millisecond, 2700*time.Millisecond).
		// 间隙 40ms
		AddSegment(6060*time.Millisecond, 8600*time.Millisecond, 2600*time.Millisecond).
		// 间隙 10ms
		AddSegment(8640*time.Millisecond, 11150*time.Millisecond, 2700*time.Millisecond).
		AddSegment(11160*time.Millisecond, 13650*time.Millisecond, 2600*time.Millisecond).
		Build()
}

// LongSilence B3: 长时间静默场景
// 片段之间存在大间隙（>3s），模拟有长时间停顿的演讲
func (b *TestDataBuilder) LongSilence() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeB().
		TotalDuration(60 * time.Second).
		AddSegment(1000*time.Millisecond, 4000*time.Millisecond, 3500*time.Millisecond).
		// 5s 静默
		AddSegment(9000*time.Millisecond, 12000*time.Millisecond, 3200*time.Millisecond).
		// 8s 静默
		AddSegment(20000*time.Millisecond, 23000*time.Millisecond, 3400*time.Millisecond).
		// 6s 静默
		AddSegment(29000*time.Millisecond, 32000*time.Millisecond, 3300*time.Millisecond).
		Build()
}

// ShorterTranslation B4: 翻译比原始短
// 翻译音频短于原始，需要减速或产生间隙（R < 1.0）
func (b *TestDataBuilder) ShorterTranslation() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(30 * time.Second).
		AddSegment(1000*time.Millisecond, 4000*time.Millisecond, 2500*time.Millisecond).
		AddSegment(4500*time.Millisecond, 7500*time.Millisecond, 2200*time.Millisecond).
		AddSegment(8000*time.Millisecond, 11000*time.Millisecond, 2400*time.Millisecond).
		AddSegment(11500*time.Millisecond, 14500*time.Millisecond, 2300*time.Millisecond).
		Build()
}

// ZeroWindow B5: 零搜索窗口场景
// S[i] 必须严格等于 T_orig_start[i]，测试极端约束
func (b *TestDataBuilder) ZeroWindow() *timeline.PipelineInput {
	cfg := timeline.NewDefaultConfig(timeline.ModeA)
	cfg.TotalDuration = 20 * time.Second
	cfg.DP.WindowLeft = 0
	cfg.DP.WindowRight = 0

	return NewTestDataBuilder().
		ModeA().
		TotalDuration(20 * time.Second).
		Config(cfg).
		AddSegment(1000*time.Millisecond, 3000*time.Millisecond, 2500*time.Millisecond).
		AddSegment(3500*time.Millisecond, 5500*time.Millisecond, 2400*time.Millisecond).
		AddSegment(6000*time.Millisecond, 8000*time.Millisecond, 2300*time.Millisecond).
		Build()
}

// ============================================================
// 压力测试场景 (Stress Test Scenarios)
// ============================================================

// DensePacking P1: 密集片段场景
// 20个片段，间隙约100ms，测试密集排期能力
func (b *TestDataBuilder) DensePacking() *timeline.PipelineInput {
	builder := NewTestDataBuilder().ModeA().TotalDuration(90 * time.Second)

	// 20个片段，每个原始2s，间隙100ms
	start := 0 * time.Millisecond
	origLen := 2000 * time.Millisecond
	gap := 100 * time.Millisecond

	for i := 0; i < 20; i++ {
		transLen := time.Duration(2300+i%3*100) * time.Millisecond // 2300-2500ms
		builder.AddSegment(start, start+origLen, transLen)
		start = start + origLen + gap
	}

	return builder.Build()
}

// LargeScale P2: 大规模场景
// 50个片段，模拟1小时播客
func (b *TestDataBuilder) LargeScale() *timeline.PipelineInput {
	builder := NewTestDataBuilder().ModeB().TotalDuration(3600 * time.Second)

	// 50个片段，模拟播客
	start := 1000 * time.Millisecond
	for i := 0; i < 50; i++ {
		origLen := time.Duration(3000+i%5*500) * time.Millisecond // 3-5s
		transLen := time.Duration(3200+i%4*300) * time.Millisecond
		gap := time.Duration(500+i%3*200) * time.Millisecond // 500-900ms

		// 每15个片段插入一个自然断点（2s+间隙）
		if i > 0 && i%15 == 0 {
			gap = 2500 * time.Millisecond
		}

		builder.AddSegment(start, start+origLen, transLen)
		start = start + origLen + gap
	}

	return builder.Build()
}

// ExtremeCompression P3: 极端压缩场景
// 10个片段，每个都需要接近极限的压缩（R→1.4）
func (b *TestDataBuilder) ExtremeCompression() *timeline.PipelineInput {
	builder := NewTestDataBuilder().ModeA().TotalDuration(50 * time.Second)

	start := 1000 * time.Millisecond
	for i := 0; i < 10; i++ {
		// 原始2s，翻译约2.7s，需要 R≈1.35
		origLen := 2000 * time.Millisecond
		transLen := time.Duration(2650+i*30) * time.Millisecond
		gap := 300 * time.Millisecond

		builder.AddSegment(start, start+origLen, transLen)
		start = start + origLen + gap
	}

	return builder.Build()
}

// ============================================================
// 真实模拟场景 (Real-world Simulation Scenarios)
// ============================================================

// NewsBroadcast R1: 新闻播报风格
// 12个片段，短句为主，模拟30秒新闻
func (b *TestDataBuilder) NewsBroadcast() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeA().
		TotalDuration(35 * time.Second).
		// 开场白
		AddSegment(500*time.Millisecond, 2500*time.Millisecond, 2400*time.Millisecond).
		// 新闻点1
		AddSegment(2800*time.Millisecond, 5000*time.Millisecond, 2600*time.Millisecond).
		// 新闻点2
		AddSegment(5300*time.Millisecond, 7800*time.Millisecond, 2900*time.Millisecond).
		// 新闻点3
		AddSegment(8100*time.Millisecond, 10500*time.Millisecond, 2800*time.Millisecond).
		// 转场
		AddSegment(10800*time.Millisecond, 12500*time.Millisecond, 2100*time.Millisecond).
		// 详细报道1
		AddSegment(12800*time.Millisecond, 15500*time.Millisecond, 3200*time.Millisecond).
		// 详细报道2
		AddSegment(15800*time.Millisecond, 18200*time.Millisecond, 2900*time.Millisecond).
		// 详细报道3
		AddSegment(18500*time.Millisecond, 21200*time.Millisecond, 3100*time.Millisecond).
		// 采访片段
		AddSegment(21500*time.Millisecond, 24800*time.Millisecond, 3700*time.Millisecond).
		// 总结1
		AddSegment(25100*time.Millisecond, 27500*time.Millisecond, 2800*time.Millisecond).
		// 总结2
		AddSegment(27800*time.Millisecond, 30000*time.Millisecond, 2600*time.Millisecond).
		// 结束语
		AddSegment(30300*time.Millisecond, 32500*time.Millisecond, 2500*time.Millisecond).
		Build()
}

// Dialogue R2: 对话风格
// 8个片段，两人快速交替对话
func (b *TestDataBuilder) Dialogue() *timeline.PipelineInput {
	return NewTestDataBuilder().
		ModeB().
		TotalDuration(30 * time.Second).
		// A: 问
		AddSegment(500*time.Millisecond, 2000*time.Millisecond, 1800*time.Millisecond).
		// B: 答
		AddSegment(2100*time.Millisecond, 4500*time.Millisecond, 2800*time.Millisecond).
		// A: 追问
		AddSegment(4600*time.Millisecond, 6200*time.Millisecond, 2000*time.Millisecond).
		// B: 解释
		AddSegment(6300*time.Millisecond, 9500*time.Millisecond, 3500*time.Millisecond).
		// A: 反问
		AddSegment(9600*time.Millisecond, 11200*time.Millisecond, 1900*time.Millisecond).
		// B: 补充
		AddSegment(11300*time.Millisecond, 14000*time.Millisecond, 3000*time.Millisecond).
		// A: 总结问
		AddSegment(14100*time.Millisecond, 15800*time.Millisecond, 2100*time.Millisecond).
		// B: 结论
		AddSegment(15900*time.Millisecond, 19000*time.Millisecond, 3400*time.Millisecond).
		Build()
}

// Lecture R3: 长演讲风格
// 15个片段，长句为主，模拟5分钟演讲片段
func (b *TestDataBuilder) Lecture() *timeline.PipelineInput {
	builder := NewTestDataBuilder().ModeB().TotalDuration(300 * time.Second)

	start := 500 * time.Millisecond
	for i := 0; i < 15; i++ {
		// 长句：4-7秒
		origLen := time.Duration(4000+i%4*1000) * time.Millisecond
		transLen := time.Duration(4200+i%3*500) * time.Millisecond
		gap := time.Duration(300+i%2*200) * time.Millisecond

		builder.AddSegment(start, start+origLen, transLen)
		start = start + origLen + gap
	}

	return builder.Build()
}

// Interview R4: 访谈风格
// 20个片段，问答交替，问题短回答长
func (b *TestDataBuilder) Interview() *timeline.PipelineInput {
	builder := NewTestDataBuilder().ModeB().TotalDuration(180 * time.Second)

	start := 500 * time.Millisecond
	for i := 0; i < 20; i++ {
		var origLen, transLen time.Duration

		if i%2 == 0 {
			// 主持人提问：短（1.5-2.5s）
			origLen = time.Duration(1500+i%3*500) * time.Millisecond
			transLen = time.Duration(1400+i%2*300) * time.Millisecond
		} else {
			// 嘉宾回答：长（4-8s）
			origLen = time.Duration(4000+i%5*1000) * time.Millisecond
			transLen = time.Duration(4200+i%4*800) * time.Millisecond
		}

		gap := 200 * time.Millisecond
		builder.AddSegment(start, start+origLen, transLen)
		start = start + origLen + gap
	}

	return builder.Build()
}
