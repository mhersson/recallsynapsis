package module_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestAHitIsWrittenAsTheShapeSays(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(module.Hit{
		Ref: module.NewRef("notes", 42), Kind: "note", Title: "Beds", Snippet: "the beds",
		AreaID: 7, Area: "garden", State: "filed", ChangedAt: time.Date(2026, 11, 22, 9, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ref":"notes:42","kind":"note","title":"Beds","snippet":"the beds","area":"garden","state":"filed","changed_at":"2026-11-22T09:30:00Z"}`, string(b))
}

func TestAnItemIsWrittenAsALinkIs(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(module.Item{Ref: module.NewRef("notes", 42), Kind: "idea", State: "inbox", Title: "Beds", Available: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ref":"notes:42","kind":"idea","state":"inbox","title":"Beds","available":true}`, string(b))
}

func TestARefNamesAModuleAndAnID(t *testing.T) {
	t.Parallel()

	ref := module.NewRef("notes", 42)
	assert.Equal(t, module.Ref("notes:42"), ref)

	name, id, ok := ref.Split()
	assert.True(t, ok)
	assert.Equal(t, "notes", name)
	assert.Equal(t, int64(42), id)

	for _, bad := range []module.Ref{"", "notes", "notes:", ":42", "notes:x", "notes:4:2"} {
		_, _, ok := bad.Split()
		assert.False(t, ok, bad)
	}
}
