package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimal = "public_url = \"https://recall.example.org\"\nowner = \"ada\"\n"

func writeFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "recallsynapsis.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoadTheWholeFile(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeFile(t, `
listen     = "0.0.0.0:7119"
public_url = "https://recall.example.org"
timezone   = "Europe/Oslo"
owner      = "ada"

[modules.notes]
enabled           = true
capture_max_chars = 4000

[backup]
dir  = "/var/lib/recallsynapsis/backup"
at   = "04:30"
keep = 7
`))
	require.NoError(t, err)

	assert.Equal(t, &Config{
		Listen:    "0.0.0.0:7119",
		PublicURL: "https://recall.example.org",
		Timezone:  "Europe/Oslo",
		Owner:     "ada",
		Backup:    Backup{Dir: "/var/lib/recallsynapsis/backup", At: "04:30", Keep: 7},
		Modules:   map[string]map[string]any{"notes": {"enabled": true, "capture_max_chars": int64(4000)}},
	}, cfg)
}

func TestDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeFile(t, minimal+"[backup]\ndir = \"/backup\"\n"))
	require.NoError(t, err)

	assert.Equal(t, &Config{
		Listen:    "127.0.0.1:7119",
		PublicURL: "https://recall.example.org",
		Timezone:  "UTC",
		Owner:     "ada",
		Backup:    Backup{Dir: "/backup", At: "03:00", Keep: 14},
	}, cfg)
}

func TestRefusalsNameTheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, file, key string
	}{
		{"unknown key at the top", minimal + "ownr = \"ada\"\n", "ownr"},
		{"unknown key in backup", minimal + "[backup]\nkept = 3\n", "backup.kept"},
		{"no public_url", "owner = \"ada\"\n", "public_url"},
		{"public_url with a path", "public_url = \"https://example.org/recall\"\nowner = \"ada\"\n", "public_url"},
		{"public_url with a slash", "public_url = \"https://example.org/\"\nowner = \"ada\"\n", "public_url"},
		{"public_url with a query", "public_url = \"https://example.org?x=1\"\nowner = \"ada\"\n", "public_url"},
		{"public_url with a user", "public_url = \"https://u:p@example.org\"\nowner = \"ada\"\n", "public_url"},
		{"public_url not http", "public_url = \"ftp://example.org\"\nowner = \"ada\"\n", "public_url"},
		{"no owner", "public_url = \"https://recall.example.org\"\n", "owner"},
		{"listen a number", minimal + "listen = 7119\n", "listen"},
		{"listen empty", minimal + "listen = \"\"\n", "listen"},
		{"listen with no port", minimal + "listen = \"127.0.0.1\"\n", "listen"},
		{"timezone the database does not know", minimal + "timezone = \"Mars/Olympus_Mons\"\n", "timezone"},
		{"timezone the host's", minimal + "timezone = \"Local\"\n", "timezone"},
		{"timezone empty", minimal + "timezone = \"\"\n", "timezone"},
		{"at not a time of day", minimal + "[backup]\nat = \"25:00\"\n", "backup.at"},
		{"at with seconds", minimal + "[backup]\nat = \"03:00:00\"\n", "backup.at"},
		{"at a TOML time", minimal + "[backup]\nat = 03:00:00\n", "backup.at"},
		{"keep zero", minimal + "[backup]\nkeep = 0\n", "backup.keep"},
		{"modules not tables", minimal + "[modules]\nnotes = true\n", "modules.notes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, tt.file)
			_, err := Load(path)
			require.Error(t, err)
			// The path holds the subtest's name, so only the rest can name the key.
			assert.Contains(t, strings.ReplaceAll(err.Error(), path, ""), tt.key)
		})
	}
}

func TestAMissingFileNamesThePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "recallsynapsis.toml")
	_, err := Load(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), path)
	assert.NoFileExists(t, path, "no starter file is written")
}

// settings is a module's own struct, with its defaults.
type settings struct {
	Size int    `toml:"size"`
	Mode string `toml:"mode"`
}

func TestAModuleTable(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeFile(t, minimal+`
[modules.plain]
enabled = true

[modules.tuned]
enabled = false
size    = 7

[modules.silent]
`))
	require.NoError(t, err)

	on, err := cfg.Module("plain", nil)
	require.NoError(t, err)
	assert.True(t, on)

	got := settings{Size: 1, Mode: "fast"}
	on, err = cfg.Module("tuned", &got)
	require.NoError(t, err)
	assert.False(t, on)
	assert.Equal(t, settings{Size: 7, Mode: "fast"}, got, "a key left out keeps its default")

	on, err = cfg.Module("silent", nil)
	require.NoError(t, err)
	assert.False(t, on, "a table with no enabled is off")

	on, err = cfg.Module("absent", &got)
	require.NoError(t, err)
	assert.False(t, on, "a module with no table is off")
}

func TestAModuleTableRefusesWhatItsModuleLacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, table, key string
		settings         any
	}{
		{"enabled not a boolean", "enabled = \"yes\"", "modules.x.enabled", nil},
		{"a key of a module with no settings", "enabled = true\ncolour = \"red\"", "modules.x.colour", nil},
		{"a key its settings lack", "enabled = true\nsize = 3\nsise = 4", "modules.x.sise", &settings{}},
		{"a wrong type", "size = \"big\"", "modules.x.size", &settings{}},
		{"a table in it", "[modules.x.inner]\nsize = 3", "modules.x.inner", &settings{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file := minimal + "[modules.x]\n" + tt.table + "\n"
			if strings.HasPrefix(tt.table, "[") {
				file = minimal + tt.table + "\n"
			}

			cfg, err := Load(writeFile(t, file))
			require.NoError(t, err)

			_, err = cfg.Module("x", tt.settings)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}
