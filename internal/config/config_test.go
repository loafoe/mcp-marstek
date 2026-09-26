package config

import (
	"os"
	"testing"
	"time"
)

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := &Config{
		Battery: BatteryConfig{
			Addr: "192.168.1.100",
			Port: 30000,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Battery.Port != 30000 {
		t.Errorf("expected port 30000, got %d", cfg.Battery.Port)
	}
}

func TestConfig_Validate_MissingAddr(t *testing.T) {
	cfg := &Config{
		Battery: BatteryConfig{
			Port: 30000,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing addr, got nil")
	}
}

func TestConfig_Validate_Defaults(t *testing.T) {
	cfg := &Config{
		Battery: BatteryConfig{
			Addr: "192.168.1.100",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Battery.Port != 30000 {
		t.Errorf("expected default port 30000, got %d", cfg.Battery.Port)
	}
	if cfg.Battery.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", cfg.Battery.Timeout)
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Battery: BatteryConfig{
			Addr: "192.168.1.100",
			Port: 99999,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestConfig_Load(t *testing.T) {
	content := []byte(`
battery:
  addr: 192.168.1.100
  port: 30000
read_only: false
`)
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Battery.Addr != "192.168.1.100" {
		t.Errorf("expected addr 192.168.1.100, got %s", cfg.Battery.Addr)
	}
	if cfg.Battery.Port != 30000 {
		t.Errorf("expected port 30000, got %d", cfg.Battery.Port)
	}
	if cfg.ReadOnly {
		t.Error("expected read_only false")
	}
}
