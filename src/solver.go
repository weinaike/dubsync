package timeline

// Solver 求解器接口
// 负责求解单个子问题的最优排期
type Solver interface {
	// SolveGreedy 贪心求解（Phase 1，保底可行解）
	// 从左到右扫描，每段尝试 R=1.0，若重叠则右移或加速
	// 返回的 Blueprint 保证不重叠，但不保证全局最优
	SolveGreedy(sub *SubProblem, cfg *DPConfig, weights *PenaltyWeights) (*Blueprint, error)

	// SolveDP 动态规划求解（Phase 2，全局优化）
	// upperBound 为贪心解的总惩罚，用于剪枝
	// 返回的 Blueprint 在满足约束的前提下最小化总惩罚
	SolveDP(sub *SubProblem, cfg *DPConfig, weights *PenaltyWeights, upperBound float64) (*Blueprint, error)
}
