package orchestration

// ScoreConfig holds the QTPO ranking weights and the hysteresis margin of the
// cost model in Sect. 3 (Selection). The controller and the evaluation
// harnesses use the same typed value. The score of a profile p on edge e is
// Wo*Overhead + Wr*Risk + Wm*MigrationCost + Wh*SwitchPenalty, and the active
// profile is kept unless a competitor scores lower by more than Delta. The
// weights order only candidates that have passed every hard gate. No
// admissibility check reads them, which is the content of Proposition 1
// (weight-independence). The cost-weight guidance is in Sect. 5.
type ScoreConfig struct {
	Wo    float64 // overhead weight
	Wr    float64 // residual-risk weight
	Wm    float64 // migration-cost weight
	Wh    float64 // switch-penalty weight
	Delta float64 // hysteresis margin (absolute score gap)
}

// DefaultScoreConfig is the operating point stated in Sect. 3 of the paper:
// w_o=0.25, w_r=0.40, w_m=0.20, w_h=0.15 and delta=0.05.
var DefaultScoreConfig = ScoreConfig{Wo: 0.25, Wr: 0.40, Wm: 0.20, Wh: 0.15, Delta: 0.05}
