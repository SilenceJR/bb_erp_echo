package config

import "testing"

// TestLoadDefaultLogConfig 验证默认日志配置能满足文件化历史排查需求。
func TestLoadDefaultLogConfig(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Log.Level != "info" {
		t.Fatalf("log level = %q", cfg.Log.Level)
	}
	if cfg.Log.Dir != "logs" {
		t.Fatalf("log dir = %q", cfg.Log.Dir)
	}
	if !cfg.Log.Console {
		t.Fatalf("log console should default to true")
	}
	if cfg.Log.RetentionDays != 30 {
		t.Fatalf("log retention days = %d", cfg.Log.RetentionDays)
	}
	if !cfg.Web.Enabled {
		t.Fatalf("web static should default to enabled")
	}
	if cfg.Web.DistDir != "web/dist" {
		t.Fatalf("web dist dir = %q", cfg.Web.DistDir)
	}
}

// TestLoadLogConfigFromEnv 验证日志配置可通过环境变量覆盖。
func TestLoadLogConfigFromEnv(t *testing.T) {
	t.Setenv("BB_ERP_LOG_LEVEL", "debug")
	t.Setenv("BB_ERP_LOG_DIR", "tmp-logs")
	t.Setenv("BB_ERP_LOG_CONSOLE", "false")
	t.Setenv("BB_ERP_LOG_RETENTION_DAYS", "7")
	t.Setenv("BB_ERP_WEB_ENABLED", "false")
	t.Setenv("BB_ERP_WEB_DIST_DIR", "tmp-web-dist")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Log.Level != "debug" {
		t.Fatalf("log level = %q", cfg.Log.Level)
	}
	if cfg.Log.Dir != "tmp-logs" {
		t.Fatalf("log dir = %q", cfg.Log.Dir)
	}
	if cfg.Log.Console {
		t.Fatalf("log console should be false")
	}
	if cfg.Log.RetentionDays != 7 {
		t.Fatalf("log retention days = %d", cfg.Log.RetentionDays)
	}
	if cfg.Web.Enabled {
		t.Fatalf("web static should be false")
	}
	if cfg.Web.DistDir != "tmp-web-dist" {
		t.Fatalf("web dist dir = %q", cfg.Web.DistDir)
	}
}
