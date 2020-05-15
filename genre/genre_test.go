package genre

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenre(t *testing.T) {
	assert.Equal(t, Genre(0), None)
	assert.Equal(t, Genre(21), Other)
}

func TestGenreString(t *testing.T) {
	assert.Equal(t, Genre(0).String(), "None")
	assert.Equal(t, Genre(21).String(), "Other")
}
