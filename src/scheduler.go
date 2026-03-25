package timeline

// Scheduler 调度器接口
// 负责执行全局排期，将翻译音频片段编排到时间轴上
type Scheduler interface {
	// Schedule 执行全局排期
	// 输入 PipelineInput 包含原始片段、TTS 片段和配置
	// 返回 Blueprint 包含所有片段的编排结果
	Schedule(input *PipelineInput) (*Blueprint, error)
}
