package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

// Config 是系统总配置。
//
// 参数说明：
// - App：应用名称、运行环境等通用信息。
// - HTTP：Echo HTTP 服务监听地址和跨域配置。
// - Database：SQLite 数据库文件路径。
// - JWT：登录令牌签名、签发方和过期时间。
// - Log：结构化日志级别。
// - Web：Web 管理端静态文件配置。
// - Admin：首次启动时自动创建的管理员账号。
type Config struct {
	App      AppConfig      `koanf:"app"`
	HTTP     HTTPConfig     `koanf:"http"`
	Database DatabaseConfig `koanf:"database"`
	JWT      JWTConfig      `koanf:"jwt"`
	Log      LogConfig      `koanf:"log"`
	Web      WebConfig      `koanf:"web"`
	Admin    AdminConfig    `koanf:"admin"`
}

// AppConfig 描述应用自身信息。
type AppConfig struct {
	// Name 是应用显示名称，例如“博邦 ERP”。
	Name string `koanf:"name"`
	// Environment 是运行环境，例如 development、staging、production。
	Environment string `koanf:"environment"`
}

// HTTPConfig 描述 HTTP 服务配置。
type HTTPConfig struct {
	// Host 是监听地址，默认监听所有网卡以供同一局域网内的客户端访问。
	Host string `koanf:"host"`
	// Port 是监听端口。
	Port int `koanf:"port"`
	// AllowedOrigins 是允许访问 API 的 Web 管理端来源列表。
	AllowedOrigins []string `koanf:"allowed_origins"`
}

// DatabaseConfig 描述数据库连接配置。
type DatabaseConfig struct {
	// Path 是 SQLite 数据库文件路径。
	Path string `koanf:"path"`
}

// JWTConfig 描述 JWT 登录令牌配置。
type JWTConfig struct {
	// Secret 是 JWT HMAC 签名密钥，生产环境必须通过环境变量覆盖。
	Secret string `koanf:"secret"`
	// ExpiresIn 是 access token 有效期，例如 24h。
	ExpiresIn time.Duration `koanf:"expires_in"`
	// Issuer 是 JWT 签发方，用于区分令牌来源。
	Issuer string `koanf:"issuer"`
}

// LogConfig 描述日志配置。
type LogConfig struct {
	// Level 是日志级别，可选 debug、info、warn、error。
	Level string `koanf:"level"`
	// Dir 是日志文件目录，默认 logs。
	Dir string `koanf:"dir"`
	// Console 表示是否同时输出到控制台。
	Console bool `koanf:"console"`
	// RetentionDays 是日志保留天数，超过天数的历史日志会在启动时清理。
	RetentionDays int `koanf:"retention_days"`
}

// WebConfig 描述 Web 管理端静态文件配置。
type WebConfig struct {
	// Enabled 表示是否由 Echo 直接托管 Web 静态文件。
	Enabled bool `koanf:"enabled"`
	// DistDir 是前端构建产物目录，默认 web/dist。
	DistDir string `koanf:"dist_dir"`
}

// AdminConfig 描述系统初始化管理员账号。
type AdminConfig struct {
	// Username 是默认管理员登录账号。
	Username string `koanf:"username"`
	// Password 是默认管理员初始密码，生产环境必须通过环境变量覆盖。
	Password string `koanf:"password"`
	// Name 是默认管理员显示名称。
	Name string `koanf:"name"`
}

// Load 加载系统配置。
//
// 加载顺序：
// 1. 使用内置默认值保证开发环境可直接启动。
// 2. 使用 BB_ERP_ 前缀的环境变量覆盖默认值。
//
// 参数说明：无。
// 返回说明：返回完整 Config；当默认值、环境变量或结构体解析失败时返回错误。
func Load() (*Config, error) {
	k := koanf.New(".")

	defaults := map[string]any{
		"app.name":             "博邦 ERP",
		"app.environment":      "development",
		"http.host":            "0.0.0.0",
		"http.port":            8080,
		"http.allowed_origins": []string{"http://localhost:3000", "http://localhost:5173"},
		"database.path":        "data/erp.db",
		"jwt.secret":           "change-me-in-production",
		"jwt.expires_in":       "24h",
		"jwt.issuer":           "bb-erp-echo",
		"log.level":            "info",
		"log.dir":              "logs",
		"log.console":          true,
		"log.retention_days":   30,
		"web.enabled":          true,
		"web.dist_dir":         "web/dist",
		"admin.username":       "admin",
		"admin.password":       "admin123456",
		"admin.name":           "系统管理员",
	}

	// 默认配置只负责让本地开发可运行，敏感配置需要在部署时由环境变量覆盖。
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
	if cfg.Log.Dir == "" {
		cfg.Log.Dir = "logs"
	}
	if envRetentionDays := k.Int("log.retention.days"); envRetentionDays > 0 {
		cfg.Log.RetentionDays = envRetentionDays
	}
	if cfg.Log.RetentionDays == 0 {
		cfg.Log.RetentionDays = 30
	}
	cfg.Web.Enabled = k.Bool("web.enabled")
	if distDir := k.String("web.dist.dir"); distDir != "" {
		cfg.Web.DistDir = distDir
	}
	if cfg.Web.DistDir == "" {
		cfg.Web.DistDir = "web/dist"
	}

	return &cfg, nil
}

// envKey 将环境变量名称转换成 koanf 的点号路径。
//
// 参数说明：
// - key：原始环境变量名称，例如 BB_ERP_DATABASE_PATH。
//
// 返回说明：
// - 返回 koanf 可识别的配置路径，例如 database.path。
func envKey(key string) string {
	key = strings.TrimPrefix(key, "BB_ERP_")
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "__", ".")
	key = strings.ReplaceAll(key, "_", ".")
	return key
}
