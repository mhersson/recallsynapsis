package module_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mhersson/recallsynapsis/internal/module"
)

func TestTheCallerTravelsInTheContext(t *testing.T) {
	t.Parallel()

	_, ok := module.CallerFrom(t.Context())
	assert.False(t, ok)

	want := module.Caller{UserID: 7, Source: "phone", Write: true}

	got, ok := module.CallerFrom(module.WithCaller(t.Context(), want))
	assert.True(t, ok)
	assert.Equal(t, want, got)
}
