package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"runtime/debug"
)

// version is the linker's -X main.version value until init resolves it from the build information.
var version string

// resolveVersion prefers the linker's value, then a revision, then a module version, else "dev".
func resolveVersion(linker string, info *debug.BuildInfo) string {
	if linker != "" || info == nil {
		return cmp.Or(linker, "dev")
	}

	var revision, modified string

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}

	if revision == "" {
		if info.Main.Version == "(devel)" {
			return "dev"
		}

		return cmp.Or(info.Main.Version, "dev")
	}

	revision = revision[:min(len(revision), 7)]
	if modified == "true" {
		revision += "-dirty"
	}

	return revision
}

func init() {
	info, _ := debug.ReadBuildInfo()
	version = resolveVersion(version, info)
	commands["version"] = command{
		summary: "print the recallsynapsis version",
		run: func(_ context.Context, _ []string, _ io.Reader, stdout, _ io.Writer) error {
			_, err := fmt.Fprintf(stdout, "recallsynapsis %s\n", version)

			return err
		},
	}
}
