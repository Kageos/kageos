package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// AppStorageConfig app-storage 配置
type AppStorageConfig struct {
	Server struct {
		Port        int    `mapstructure:"port"`
		ListenHost  string `mapstructure:"listen_host"`
		LogLevel    string `mapstructure:"log_level"`
		Debug       bool   `mapstructure:"debug"`
		EnablePprof *bool  `mapstructure:"enable_pprof"`
	} `mapstructure:"server"`

	// 注意：JWT 配置已移至全局配置，不再在此处配置

	Audit struct {
		UploadTracking struct {
			Enabled bool `mapstructure:"enabled"`
		} `mapstructure:"upload_tracking"`
		DownloadTracking struct {
			Enabled       bool `mapstructure:"enabled"`
			RetentionDays int  `mapstructure:"retention_days"`
		} `mapstructure:"download_tracking"`
	} `mapstructure:"audit"`

	Storage struct {
		Type string `mapstructure:"type"` // 当前仅支持 minio

		MinIO struct {
			Endpoint       string `mapstructure:"endpoint"`        // 浏览器上传用的 endpoint（宿主机访问）
			ServerEndpoint string `mapstructure:"server_endpoint"` // ✨ 服务端上传用的 endpoint（容器内访问）
			ConsoleURL     string `mapstructure:"console_url"`     // 可选：仅供系统管理员跳转的 MinIO 控制台地址
			AccessKey      string `mapstructure:"access_key"`
			SecretKey      string `mapstructure:"secret_key"`
			UseSSL         bool   `mapstructure:"use_ssl"`
			Region         string `mapstructure:"region"`
			DefaultBucket  string `mapstructure:"default_bucket"`
			CDNDomain      string `mapstructure:"cdn_domain"` // ✨ CDN 域名（可选，用于加速访问）
		} `mapstructure:"minio"`

		Upload struct {
			MaxSize     int64 `mapstructure:"max_size"`
			TokenExpire int   `mapstructure:"token_expire"`
		} `mapstructure:"upload"`
	} `mapstructure:"storage"`

	DB DBConfig `mapstructure:"db"`
	// 注意：JWT 配置已移至全局配置，不再在此处配置
	// 数据库配置保留在服务配置中，因为微服务后续每个服务一个库
}

var (
	appStorageConfig     *AppStorageConfig
	appStorageConfigOnce sync.Once
)

// GetAppStorageConfig 获取 app-storage 配置（单例）
func GetAppStorageConfig() *AppStorageConfig {
	appStorageConfigOnce.Do(func() {
		appStorageConfig = loadAppStorageConfig()
	})
	return appStorageConfig
}

// loadAppStorageConfig 加载 app-storage 配置
func loadAppStorageConfig() *AppStorageConfig {
	cfg := &AppStorageConfig{}
	if err := loadYAMLConfig("app-storage.yaml", cfg); err != nil {
		// 配置文件不存在或加载失败，返回空配置
		fmt.Printf("Failed to load app-storage config: %v\n", err)
		cfg = &AppStorageConfig{}
	}

	applyCanonicalCDNToAppStorage(cfg)
	return cfg
}

// EnvCanonicalBaseURL 容器/一键部署：与浏览器一致的主站公网地址（含 scheme），用于补全 minio.cdn_domain。
const EnvCanonicalBaseURL = "CANONICAL_BASE_URL"

func applyCanonicalCDNToAppStorage(c *AppStorageConfig) {
	if c == nil || c.GetStorageType() != "minio" {
		return
	}
	if strings.TrimSpace(c.Storage.MinIO.CDNDomain) != "" {
		return
	}
	v := strings.TrimSpace(os.Getenv(EnvCanonicalBaseURL))
	if v == "" {
		return
	}
	c.Storage.MinIO.CDNDomain = v
	fmt.Printf("[config] app-storage: storage.minio.cdn_domain empty, using %s=%s\n", EnvCanonicalBaseURL, v)
}

func (c *AppStorageConfig) GetStorageType() string {
	if c == nil {
		return "minio"
	}
	if v := strings.ToLower(strings.TrimSpace(c.Storage.Type)); v != "" {
		return v
	}
	return "minio"
}

// GetMinIOConsoleURL returns only an explicitly configured operations URL.
// Asset APIs do not expose this address; loopback endpoints never infer a console.
func (c *AppStorageConfig) GetMinIOConsoleURL() string {
	if c == nil || c.GetStorageType() != "minio" {
		return ""
	}
	if value := strings.TrimSpace(c.Storage.MinIO.ConsoleURL); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("MINIO_CONSOLE_URL"))
}

// GetPort 获取端口
func (c *AppStorageConfig) GetPort() int {
	if c.Server.Port == 0 {
		return 8083
	}
	return c.Server.Port
}

// GetListenHost 获取监听地址
func (c *AppStorageConfig) GetListenHost() string {
	if c == nil {
		return normalizeListenHost("")
	}
	return normalizeListenHost(c.Server.ListenHost)
}

// GetLogLevel 获取日志级别
func (c *AppStorageConfig) GetLogLevel() string {
	if c.Server.LogLevel == "" {
		return "info"
	}
	return c.Server.LogLevel
}

// IsDebug 是否调试模式
func (c *AppStorageConfig) IsDebug() bool {
	return c.Server.Debug
}

// IsPprofEnabled 是否启用 pprof。
// 默认为 false，避免缺省配置暴露调试端点。
func (c *AppStorageConfig) IsPprofEnabled() bool {
	if c == nil {
		return false
	}
	return boolConfigValue(c.Server.EnablePprof, false)
}

// GetDB 获取数据库配置
func (c *AppStorageConfig) GetDB() DBConfig {
	return c.DB
}

// GetJWT 获取 JWT 配置（从全局配置获取）
func (c *AppStorageConfig) GetJWT() JWTConfig {
	return GetGlobalSharedConfig().JWT
}
