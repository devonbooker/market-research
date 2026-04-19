package sources

import "math"

// Score computes a signal score in [0.0, 1.0] from docs/week and avg doc score.
// Uses a saturating function so a single hot source does not dominate.
// Formula: tanh(docsPerWeek * max(avgScore, 1) / 100).
func Score(docsPerWeek int, avgScore float64) float64 {
	if docsPerWeek == 0 {
		return 0
	}
	effScore := avgScore
	if effScore < 1 {
		effScore = 1
	}
	return math.Tanh(float64(docsPerWeek) * effScore / 100.0)
}
