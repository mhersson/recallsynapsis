// Package config reads the configuration file strictly: an unknown key, a
// wrong type or an unusable value is refused with the key named.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // the zone database, so timezone loads in an image without one

	toml "github.com/pelletier/go-toml/v2"
)

// Backup is the [backup] table. With no Dir there are no backups.
type Backup struct {
	Dir  string `toml:"dir"`
	At   string `toml:"at"`
	Keep int    `toml:"keep"`
}

// Config is the whole file with its defaults applied.
type Config struct {
	Listen    string `toml:"listen"`
	PublicURL string `toml:"public_url"`
	Timezone  string `toml:"timezone"`
	Owner     string `toml:"owner"`
	Backup    Backup `toml:"backup"`
	// Modules holds each [modules.NAME] table as read, for its module to decode.
	Modules map[string]map[string]any `toml:"modules"`
}

// Load reads and checks the file at path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}

	cfg := &Config{
		Listen:   "127.0.0.1:7119",
		Timezone: "UTC",
		Backup:   Backup{At: "03:00", Keep: 14},
	}
	if err := toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(cfg); err != nil {
		return nil, decodeError(path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, nil
}

// decodeError names the offending key, which go-toml's own text leaves out
// for unknown keys.
func decodeError(path string, err error) error {
	if strict, ok := errors.AsType[*toml.StrictMissingError](err); ok {
		keys := make([]string, len(strict.Errors))
		for i, e := range strict.Errors {
			keys[i] = strings.Join(e.Key(), ".")
		}

		return fmt.Errorf("%s: unknown key %s", path, strings.Join(keys, ", "))
	}

	if de, ok := errors.AsType[*toml.DecodeError](err); ok {
		row, _ := de.Position()
		if key := de.Key(); len(key) > 0 {
			return fmt.Errorf("%s:%d: key %s: %w", path, row, strings.Join(key, "."), err)
		}

		return fmt.Errorf("%s:%d: %w", path, row, err)
	}

	return fmt.Errorf("%s: %w", path, err)
}

func (c *Config) validate() error {
	if c.PublicURL == "" {
		return errors.New("public_url is required, such as \"https://recall.example.org\"")
	}

	// Only a bare origin, so an address built on it never carries a path, query or user.
	if u, err := url.Parse(c.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || c.PublicURL != u.Scheme+"://"+u.Host {
		return fmt.Errorf("public_url %q must be an http or https origin, with no path, query or user", c.PublicURL)
	}

	// An empty listen would bind every interface.
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("listen %q is not a host and port such as \"127.0.0.1:7119\"", c.Listen)
	}

	if c.Owner == "" {
		return errors.New("owner is required: the name the owner signs in with")
	}

	// LoadLocation takes "" as UTC and "Local" as the host's zone, neither from the database.
	if _, err := time.LoadLocation(c.Timezone); err != nil || c.Timezone == "" || c.Timezone == "Local" {
		return fmt.Errorf("timezone %q is not a time zone such as \"Europe/Oslo\"", c.Timezone)
	}

	if _, err := time.Parse("15:04", c.Backup.At); err != nil {
		return fmt.Errorf("backup.at %q is not a time of day such as \"03:00\"", c.Backup.At)
	}

	if c.Backup.Keep < 1 {
		return fmt.Errorf("backup.keep is %d; keep at least one snapshot", c.Backup.Keep)
	}

	return nil
}
