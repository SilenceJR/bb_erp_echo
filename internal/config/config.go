package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"bb_erp_echo/internal/buildinfo"

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
	Update   UpdateConfig   `koanf:"update"`
	Files    FilesConfig    `koanf:"files"`
}

// FilesConfig 描述受保护上传文件的存储位置。
type FilesConfig struct {
	RootDir string `koanf:"root_dir"`
}

// AppConfig 描述应用自身信息。
type AppConfig struct {
	// Name 是应用显示名称，例如“博邦 ERP”。
	Name string `koanf:"name"`
	// Version 是当前服务端版本号，用于升级检查。
	Version string `koanf:"version"`
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
	// Secret 是 JWT HMAC 签名密钥，按当前部署策略使用系统内部配置。
	Secret string `koanf:"secret"`
	// ExpiresIn 是 access token 有效期，例如 2h。
	ExpiresIn time.Duration `koanf:"expires_in"`
	// RefreshExpiresIn 是 refresh token 在每次成功轮换后的滚动有效期，例如 720h。
	RefreshExpiresIn time.Duration `koanf:"refresh_expires_in"`
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
	// Password 是默认管理员初始密码；管理员首次登录后应在系统内修改。
	Password string `koanf:"password"`
	// Name 是默认管理员显示名称。
	Name string `koanf:"name"`
}

// UpdateConfig 描述本机发布目录或兼容 HTTP 更新源配置。
type UpdateConfig struct {
	// Enabled 表示是否允许服务端主动检查更新清单。
	Enabled bool `koanf:"enabled"`
	// ManifestURL 是兼容 HTTP(S) 更新清单地址。当 ManifestFile 非空时不使用该字段。
	ManifestURL string `koanf:"manifest_url"`
	// ManifestFile 是本机 stable manifest 文件路径；它优先于 ManifestURL。
	// 默认值 updates/stable/update-manifest.json 适用于服务端安装目录。
	ManifestFile string `koanf:"manifest_file"`
	// CacheDir 是服务端缓存客户端升级包的目录。
	CacheDir string `koanf:"cache_dir"`
	// ClientVersion 是当前随服务端发布的客户端版本，用于判断是否需要缓存新客户端包。
	ClientVersion string `koanf:"client_version"`
	// CheckInterval 是自动检查更新的周期。
	CheckInterval time.Duration `koanf:"check_interval"`
	// ManifestTimeout 是读取兼容 HTTP 清单的超时时间；本机 stable manifest 不使用网络。
	ManifestTimeout time.Duration `koanf:"manifest_timeout"`
	// DownloadTimeout 是下载客户端升级包的超时时间。
	DownloadTimeout time.Duration `koanf:"download_timeout"`
	// SigningPublicKey 是 Minisign 公钥内容，用于验证 stable manifest 的 v3 客户端更新签名。
	SigningPublicKey string `koanf:"signing_public_key"`
	// SigningPublicKeyFile 是 Minisign 公钥文件路径；SigningPublicKey 非空时优先使用直接值。
	SigningPublicKeyFile string `koanf:"signing_public_key_file"`
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
		"app.name":                       "博邦 ERP",
		"app.version":                    buildinfo.Version,
		"app.environment":                "development",
		"http.host":                      "0.0.0.0",
		"http.port":                      8080,
		"http.allowed_origins":           []string{"http://localhost:3000", "http://localhost:5173"},
		"database.path":                  "data/erp.db",
		"jwt.secret":                     "change-me-in-production",
		"jwt.expires_in":                 "2h",
		"jwt.refresh_expires_in":         "720h",
		"jwt.issuer":                     "bb-erp-echo",
		"log.level":                      "info",
		"log.dir":                        "logs",
		"log.console":                    true,
		"log.retention_days":             30,
		"web.enabled":                    true,
		"web.dist_dir":                   "web/dist",
		"admin.username":                 "admin",
		"admin.password":                 "admin123456",
		"admin.name":                     "系统管理员",
		"update.enabled":                 false,
		"update.manifest_url":            "",
		"update.manifest_file":           "updates/stable/update-manifest.json",
		"update.cache_dir":               "updates",
		"update.client_version":          buildinfo.Version,
		"update.check_interval":          "6h",
		"update.manifest_timeout":        "20s",
		"update.download_timeout":        "10m",
		"update.signing_public_key":      "",
		"update.signing_public_key_file": "",
		"files.root_dir":                 "static/uploads",
	}

	// 默认配置保证首次部署可启动；JWT 和管理员初始密码按系统内部默认策略运行，
	// 更新源等部署相关参数仍可通过环境变量覆盖。
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
	// JWT duration names contain a semantic underscore; read the public environment
	// variable names directly so BB_ERP_JWT_EXPIRES_IN and its refresh counterpart
	// are not split into a different koanf path.
	if expiresIn, ok := os.LookupEnv("BB_ERP_JWT_EXPIRES_IN"); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(expiresIn))
		if err != nil {
			return nil, fmt.Errorf("parse BB_ERP_JWT_EXPIRES_IN: %w", err)
		}
		cfg.JWT.ExpiresIn = parsed
	}
	if refreshExpiresIn, ok := os.LookupEnv("BB_ERP_JWT_REFRESH_EXPIRES_IN"); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(refreshExpiresIn))
		if err != nil {
			return nil, fmt.Errorf("parse BB_ERP_JWT_REFRESH_EXPIRES_IN: %w", err)
		}
		cfg.JWT.RefreshExpiresIn = parsed
	}

	if cfg.JWT.ExpiresIn == 0 {
		cfg.JWT.ExpiresIn = 2 * time.Hour
	}
	if cfg.JWT.RefreshExpiresIn == 0 {
		cfg.JWT.RefreshExpiresIn = 30 * 24 * time.Hour
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
	cfg.Update.Enabled = k.Bool("update.enabled")
	if manifestURL := k.String("update.manifest.url"); manifestURL != "" {
		cfg.Update.ManifestURL = manifestURL
	}
	// manifest_file 与 manifest_url 一样包含语义下划线，直接读取原始环境变量，
	// 避免 envKey 将其拆成 update.manifest.file 后无法覆盖默认路径。
	if manifestFile, ok := os.LookupEnv("BB_ERP_UPDATE_MANIFEST_FILE"); ok {
		cfg.Update.ManifestFile = strings.TrimSpace(manifestFile)
	}
	if cacheDir := k.String("update.cache.dir"); cacheDir != "" {
		cfg.Update.CacheDir = cacheDir
	}
	if cfg.Update.CacheDir == "" {
		cfg.Update.CacheDir = "updates"
	}
	if clientVersion := k.String("update.client.version"); clientVersion != "" {
		cfg.Update.ClientVersion = clientVersion
	}
	if cfg.Update.ClientVersion == "" {
		cfg.Update.ClientVersion = buildinfo.Version
	}
	if checkInterval := k.Duration("update.check.interval"); checkInterval > 0 {
		cfg.Update.CheckInterval = checkInterval
	}
	if cfg.Update.CheckInterval <= 0 {
		cfg.Update.CheckInterval = 6 * time.Hour
	}
	if manifestTimeout := k.Duration("update.manifest.timeout"); manifestTimeout > 0 {
		cfg.Update.ManifestTimeout = manifestTimeout
	}
	if cfg.Update.ManifestTimeout <= 0 {
		cfg.Update.ManifestTimeout = 20 * time.Second
	}
	if downloadTimeout := k.Duration("update.download.timeout"); downloadTimeout > 0 {
		cfg.Update.DownloadTimeout = downloadTimeout
	}
	if cfg.Update.DownloadTimeout <= 0 {
		cfg.Update.DownloadTimeout = 10 * time.Minute
	}
	if signingPublicKey := k.String("update.signing.public.key"); signingPublicKey != "" {
		cfg.Update.SigningPublicKey = signingPublicKey
	}
	if signingPublicKeyFile := k.String("update.signing.public.key.file"); signingPublicKeyFile != "" {
		cfg.Update.SigningPublicKeyFile = signingPublicKeyFile
	}
	// 这两个名称含有连续语义词，直接读取环境变量避免 koanf 将 key/file 继续拆分后丢失优先级。
	if signingPublicKey, ok := os.LookupEnv("BB_ERP_UPDATE_SIGNING_PUBLIC_KEY"); ok {
		cfg.Update.SigningPublicKey = signingPublicKey
	}
	if signingPublicKeyFile, ok := os.LookupEnv("BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE"); ok {
		cfg.Update.SigningPublicKeyFile = signingPublicKeyFile
	}
	if cfg.Files.RootDir == "" {
		cfg.Files.RootDir = "static/uploads"
	}
	if rootDir := k.String("files.root.dir"); rootDir != "" {
		cfg.Files.RootDir = rootDir
	}
	if err := validateProductionConfig(cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateProductionConfig 防止生产环境使用不完整的更新配置。
//
// 参数说明：
// - cfg：已完成默认值和环境变量覆盖的完整配置。
//
// 返回说明：
// - development/staging 等环境不执行生产强校验；production 配置不完整时返回错误。
func validateProductionConfig(cfg Config) error {
	if !strings.EqualFold(strings.TrimSpace(cfg.App.Environment), "production") {
		return nil
	}

	for name, value := range map[string]string{
		"database path":          cfg.Database.Path,
		"log directory":          cfg.Log.Dir,
		"upload directory":       cfg.Files.RootDir,
		"update cache directory": cfg.Update.CacheDir,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("production %s must be configured", name)
		}
	}

	if !cfg.Update.Enabled {
		return nil
	}

	manifestFile := strings.TrimSpace(cfg.Update.ManifestFile)
	if manifestFile == "" {
		manifestURL := strings.TrimSpace(cfg.Update.ManifestURL)
		parsedURL, err := url.Parse(manifestURL)
		if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") {
			return fmt.Errorf("production update manifest URL must be an HTTP(S) URL when local manifest file is not configured")
		}
	}

	if strings.TrimSpace(cfg.Update.SigningPublicKey) == "" {
		keyFile := strings.TrimSpace(cfg.Update.SigningPublicKeyFile)
		if keyFile == "" {
			return fmt.Errorf("production update signing public key or key file must be configured")
		}
		if _, err := os.Stat(keyFile); err != nil {
			return fmt.Errorf("production update signing public key file %q is not readable: %w", keyFile, err)
		}
	}

	return nil
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
