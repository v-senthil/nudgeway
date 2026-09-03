package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_Defaults_MissingFileOK(t *testing.T) {
	t.Parallel()
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("expected defaults to load when file missing, got %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ShutdownTimeout != 20*time.Second {
		t.Errorf("HTTP.ShutdownTimeout = %v, want 20s", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Env != "local" {
		t.Errorf("Env = %q, want local", cfg.Env)
	}
}

func TestLoad_ParsesExample(t *testing.T) {
	t.Parallel()
	cfg, err := Load("../../../config/example.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q", cfg.HTTP.Addr)
	}
	if cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Errorf("Redis.Addr = %q", cfg.Redis.Addr)
	}
	if cfg.HBase.Namespace != "fullwa" {
		t.Errorf("HBase.Namespace = %q", cfg.HBase.Namespace)
	}
	if len(cfg.HBase.ZookeeperQuorum) != 1 || cfg.HBase.ZookeeperQuorum[0] != "127.0.0.1:2181" {
		t.Errorf("HBase.ZookeeperQuorum = %v", cfg.HBase.ZookeeperQuorum)
	}
}

func TestApplyEnv_Overrides(t *testing.T) {
	t.Setenv("FULLWA_HTTP_ADDR", ":9999")
	t.Setenv("FULLWA_LOG_LEVEL", "debug")
	cfg, err := Load("../../../config/example.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":9999" {
		t.Errorf("HTTP.Addr = %q, want :9999", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
	}
}
