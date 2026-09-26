// Package config loads mcp-marstek's YAML configuration: battery connection
// settings and optional read-only mode.
package config

import (
	"fmt"
	"net"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level mcp-marstek configuration.
type Config struct {
	Battery  BatteryConfig `yaml:"battery"`
	ReadOnly bool          `yaml:"read_only,omitempty"`
}

// BatteryConfig configures the connection to a Marstek battery system.
type BatteryConfig struct {
	// Addr is the IP address or hostname of the battery on the LAN.
	Addr string `yaml:"addr"`

	// Port is the UDP API port (default 30000).
	Port int `yaml:"port,omitempty"`

	// Timeout for API requests (default 10s).
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// InstanceID is the battery instance ID (default 0).
	InstanceID int `yaml:"instance_id,omitempty"`
}

// Load reads and parses the config file at path. Values may reference
// environment variables using ${VAR} syntax.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}

	expanded := os.Expand(string(raw), func(key string) string {
		return os.Getenv(key)
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.Battery.Addr == "" {
		return fmt.Errorf("config: battery.addr is required")
	}
	if net.ParseIP(c.Battery.Addr) == nil {
		// Could be a hostname, just do a basic check
		if _, err := net.LookupHost(c.Battery.Addr); err != nil {
			return fmt.Errorf("config: battery.addr %q is not a valid IP or hostname", c.Battery.Addr)
		}
	}
	if c.Battery.Port == 0 {
		c.Battery.Port = 30000
	}
	if c.Battery.Port < 1 || c.Battery.Port > 65535 {
		return fmt.Errorf("config: battery.port must be between 1 and 65535")
	}
	if c.Battery.Timeout == 0 {
		c.Battery.Timeout = 10 * time.Second
	}
	return nil
}
