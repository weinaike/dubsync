package timeline

// Solver 求解器接口
// 负责求解单个子问题的最优排期
// 对于链式约束问题，Greedy 已是最优算法
type Solver interface {
	// SolveGreedy 贪心求解
	// 从左到右扫描，每段尝试 R=1.0，若重叠则右移或加速
	// 对于链式约束 + 凸惩罚问题，此算法产生最优解
	SolveGreedy(sub *SubProblem, cfg *DPConfig, weights *PenaltyWeights) (*Blueprint, error)

	// SolveGreedySmooth 贪心求解 + 速率平滑后处理
	// 在 SolveGreedy 的基础上，通过三阶段后处理优化速率平滑性：
	//  1. Rate Equalization: 连续加速段速率均衡
	//  2. Forward-Backward Relaxation: 前向-后向松弛，消除贪心惯性
	//  3. Penalty-Aware Fine-Tuning: 惩罚感知的速率微调
	SolveGreedySmooth(sub *SubProblem, cfg *DPConfig, weights *PenaltyWeights) (*Blueprint, error)
}
