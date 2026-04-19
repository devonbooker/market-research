package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignalScore_ZeroDocsYieldsZero(t *testing.T) {
	assert.Equal(t, 0.0, Score(0, 0))
}

func TestSignalScore_NormalizedToUnitInterval(t *testing.T) {
	s := Score(10, 5)
	assert.GreaterOrEqual(t, s, 0.0)
	assert.LessOrEqual(t, s, 1.0)
}

func TestSignalScore_MonotonicInDocs(t *testing.T) {
	a := Score(1, 5)
	b := Score(10, 5)
	assert.Greater(t, b, a)
}
