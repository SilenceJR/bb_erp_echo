package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	App      AppConfig      `koanf:"app"`
	HTTP     HTTPConfig     `koanf:"http"`
	Database DatabaseConfig `koanf:"database"`
	JWT      JWTConfig      `koanf:"jwt"`
	Log      LogConfig      `koanf:"log"`
	Admin    AdminConfig    `koanf:"admin"`
}

type AppConfig struct {
	Name        string `koanf:"name"`
	Environment string `koanf:"environment"`
}

type HTTPConfig struct {
	Host           string   `koanf:"host"`
	Port           int      `koanf:"port"`
	AllowedOrigins []string `koanf:"allowed_origins"`
}

type DatabaseConfig struct {
	Path string `koanf:"path"`
}

type JWTConfig struct {
	Secret    string        `koanf:"secret"`
	ExpiresIn time.Duration `koanf:"expires_in"`
	Issuer    string        `koanf:"issuer"`
}

type LogConfig struct {
	Level string `koanf:"level"`
}

type AdminConfig struct {
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	Name     string `koanf:"name"`
}

func Load() (*Config, error) {
	k := koanf.New(".")

	defaults := map[string]any{
		"app.name":             "博邦 ERP",
		"app.environment":      "development",
		"http.host":            "127.0.0.1",
		"http.port":            8080,
		"http.allowed_origins": []string{"http://localhost:3000", "http://localhost:5173"},
		"database.path":        "bb_erp.sqlite3",
		"jwt.secret":           "change-me-in-production",
		"jwt.expires_in":       "24h",
		"jwt.issuer":           "bb-erp-echo",
		"log.level":            "info",
		"admin.username":       "admin",
		"admin.password":       "admin123456",
		"admin.name":           "系统管理员",
	}

	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("load default config: %w", err)
	}

	if err := k.Load(env.Provider("BB_ERP_", ".", envKey), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.JWT.ExpiresIn == 0 {
		cfg.JWT.ExpiresIn = 24 * time.Hour
	}

	return &cfg, nil
}

func envKey(key string) string {
	key = strings.TrimPrefix(key, "BB_ERP_")
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "__", ".")
	key = strings.ReplaceAll(key, "_", ".")
	return key
}
