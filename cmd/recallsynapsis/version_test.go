package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveVersion(t *testing.T) {
	info := func(module string, settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: module}, Settings: settings}
	}
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"}
	modified := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	clean := debug.BuildSetting{Key: "vcs.modified", Value: "false"}

	tests := []struct {
		name   string
		linker string
		info   *debug.BuildInfo
		want   string
	}{
		{"linker value beats a revision", "1.2.3", info("v9.9.9", rev), "1.2.3"},
		{"linker value beats a module version", "1.2.3", info("v9.9.9"), "1.2.3"},
		{"tagged module without a revision", "", info("v1.4.0"), "v1.4.0"},
		{"pseudo-version without a revision", "", info("v0.0.0-20260101000000-abcdef123456"), "v0.0.0-20260101000000-abcdef123456"},
		{"clean checkout", "", info("(devel)", rev, clean), "0123456"},
		{"modified checkout", "", info("(devel)", rev, modified), "0123456-dirty"},
		{"revision beats a module version", "", info("v1.4.0", rev, clean), "0123456"},
		{"no build information", "", nil, "dev"},
		{"devel without a revision", "", info("(devel)"), "dev"},
		{"no module version and no revision", "", info(""), "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveVersion(tt.linker, tt.info))
		})
	}
}
