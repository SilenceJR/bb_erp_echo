package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadDefaultLogConfig 验证默认日志配置能满足文件化历史排查需求。
func TestLoadDefaultLogConfig(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Log.Level != "info" {
		t.Fatalf("log level = %q", cfg.Log.Level)
	}
	if cfg.HTTP.Host != "0.0.0.0" {
		t.Fatalf("http host = %q", cfg.HTTP.Host)
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
	if cfg.Silence.Password != "" {
		t.Fatal("Silence password must not have a source-code default")
	}
	if cfg.JWT.ExpiresIn != 2*time.Hour || cfg.JWT.RefreshExpiresIn != 30*24*time.Hour {
		t.Fatalf("unexpected jwt durations: expires=%s refresh=%s", cfg.JWT.ExpiresIn, cfg.JWT.RefreshExpiresIn)
	}
	if cfg.Update.CheckInterval != 6*time.Hour || cfg.Update.ManifestTimeout != 20*time.Second || cfg.Update.DownloadTimeout != 10*time.Minute {
		t.Fatalf("unexpected update durations: %+v", cfg.Update)
	}
	if !cfg.Discovery.Enabled || cfg.Discovery.ServerName == "" || cfg.Discovery.BindHost != "0.0.0.0" || cfg.Discovery.Port != 39080 {
		t.Fatalf("unexpected discovery defaults: %+v", cfg.Discovery)
	}
	if cfg.Discovery.ScanTimeout != 2500*time.Millisecond || cfg.Discovery.PreflightTimeout != 3*time.Second || cfg.Discovery.HTTPTimeout != 2*time.Second {
		t.Fatalf("unexpected discovery durations: %+v", cfg.Discovery)
	}
}

// TestLoadLogConfigFromEnv 验证日志配置可通过环境变量覆盖。
func TestLoadLogConfigFromEnv(t *testing.T) {
	t.Setenv("BB_ERP_LOG_LEVEL", "debug")
	t.Setenv("BB_ERP_LOG_DIR", "tmp-logs")
	t.Setenv("BB_ERP_LOG_CONSOLE", "false")
	t.Setenv("BB_ERP_LOG_RETENTION_DAYS", "7")
	t.Setenv("BB_ERP_JWT_EXPIRES_IN", "90m")
	t.Setenv("BB_ERP_JWT_REFRESH_EXPIRES_IN", "168h")
	t.Setenv("BB_ERP_WEB_ENABLED", "false")
	t.Setenv("BB_ERP_WEB_DIST_DIR", "tmp-web-dist")
	t.Setenv("BB_ERP_UPDATE_CHECK_INTERVAL", "30m")
	t.Setenv("BB_ERP_UPDATE_MANIFEST_TIMEOUT", "9s")
	t.Setenv("BB_ERP_UPDATE_DOWNLOAD_TIMEOUT", "2m")
	t.Setenv("BB_ERP_UPDATE_SIGNING_PUBLIC_KEY", "direct-test-key")
	t.Setenv("BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE", "test-update-public.key")
	t.Setenv("BB_ERP_DISCOVERY_ENABLED", "false")
	t.Setenv("BB_ERP_DISCOVERY_SERVER_NAME", "测试发现服务")
	t.Setenv("BB_ERP_DISCOVERY_BIND_HOST", "127.0.0.1")
	t.Setenv("BB_ERP_DISCOVERY_PORT", "39081")
	t.Setenv("BB_ERP_DISCOVERY_SCAN_TIMEOUT", "150ms")
	t.Setenv("BB_ERP_DISCOVERY_PREFLIGHT_TIMEOUT", "250ms")
	t.Setenv("BB_ERP_DISCOVERY_HTTP_TIMEOUT", "400ms")
	t.Setenv("BB_ERP_SILENCE_PASSWORD", "configured-silence-password")

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
	if cfg.JWT.ExpiresIn != 90*time.Minute || cfg.JWT.RefreshExpiresIn != 168*time.Hour {
		t.Fatalf("jwt env durations: expires=%s refresh=%s", cfg.JWT.ExpiresIn, cfg.JWT.RefreshExpiresIn)
	}
	if cfg.Update.CheckInterval != 30*time.Minute || cfg.Update.ManifestTimeout != 9*time.Second || cfg.Update.DownloadTimeout != 2*time.Minute {
		t.Fatalf("update env durations: %+v", cfg.Update)
	}
	if cfg.Update.SigningPublicKey != "direct-test-key" || cfg.Update.SigningPublicKeyFile != "test-update-public.key" {
		t.Fatalf("update signing public key config: %+v", cfg.Update)
	}
	if cfg.Discovery.Enabled || cfg.Discovery.ServerName != "测试发现服务" || cfg.Discovery.BindHost != "127.0.0.1" || cfg.Discovery.Port != 39081 {
		t.Fatalf("discovery env config: %+v", cfg.Discovery)
	}
	if cfg.Discovery.ScanTimeout != 150*time.Millisecond || cfg.Discovery.PreflightTimeout != 250*time.Millisecond || cfg.Discovery.HTTPTimeout != 400*time.Millisecond {
		t.Fatalf("discovery env durations: %+v", cfg.Discovery)
	}
	if cfg.Silence.Password != "configured-silence-password" {
		t.Fatal("Silence password was not loaded from BB_ERP_SILENCE_PASSWORD")
	}
}

// TestLoadProductionConfigAllowsDefaultCredentials 验证生产环境允许首次使用默认管理员登录再在系统内修改密码。
func TestLoadProductionConfigAllowsDefaultCredentials(t *testing.T) {
	t.Setenv("BB_ERP_APP_ENVIRONMENT", "production")
	t.Setenv("BB_ERP_UPDATE_ENABLED", "false")

	if _, err := Load(); err != nil {
		t.Fatalf("default production credentials should be allowed for first login: %v", err)
	}
}

// TestLoadProductionConfigUsesInternalCredentials 验证关闭更新时的最小生产配置可以加载。
func TestLoadProductionConfigUsesInternalCredentials(t *testing.T) {
	t.Setenv("BB_ERP_APP_ENVIRONMENT", "production")
	t.Setenv("BB_ERP_UPDATE_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load production config: %v", err)
	}
	if cfg.App.Environment != "production" || cfg.Update.Enabled {
		t.Fatalf("unexpected production config: %+v", cfg)
	}
}

// TestLoadProductionConfigRequiresUpdateVerifier 验证启用更新时必须配置清单和验签公钥。
func TestLoadProductionConfigRequiresUpdateVerifier(t *testing.T) {
	t.Setenv("BB_ERP_APP_ENVIRONMENT", "production")
	t.Setenv("BB_ERP_UPDATE_ENABLED", "true")
	t.Setenv("BB_ERP_UPDATE_MANIFEST_URL", "https://example.com/update-manifest.json")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "signing public key") {
		t.Fatalf("expected update signing key validation error, got %v", err)
	}
}
