package module_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	decode := func(body string) (string, error) {
		var got struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(body))
		err := module.Decode(httptest.NewRecorder(), req, &got)

		return got.Name, err
	}

	name, err := decode(`{"name":"ada"}`)
	require.NoError(t, err)
	assert.Equal(t, "ada", name)

	tests := []struct {
		name, body string
		status     int
	}{
		{"an unknown field", `{"name":"ada","admin":true}`, http.StatusBadRequest},
		{"two values", `{"name":"ada"}{"name":"bob"}`, http.StatusBadRequest},
		{"no value", ``, http.StatusBadRequest},
		{"a wrong type", `{"name":7}`, http.StatusBadRequest},
		{"more than the cap", `{"name":"` + strings.Repeat("a", 1<<20) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := decode(tt.body)

			refusal, ok := errors.AsType[*module.Refusal](err)
			require.True(t, ok, err)
			assert.Equal(t, tt.status, refusal.Status)
			assert.NotEmpty(t, refusal.Text)
			assert.NotContains(t, refusal.Text, "\n")
		})
	}
}

func TestReply(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	module.Reply(rec, http.StatusCreated, map[string]int{"id": 7})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"id":7}`, rec.Body.String())
}
