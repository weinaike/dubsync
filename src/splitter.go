package timeline

// Splitter 子问题分解器接口
// 负责按自然断点将大规模问题分解为独立子问题
type Splitter interface {
	// Split 按自然断点分解问题
	// threshold 为断点判定阈值（如 1.5s），间隙 >= threshold 的位置被视为断点
	// 返回分解后的子问题列表
	Split(sources []SourceSegment, tts []TTSSegment, threshold Duration) []SubProblem
}
