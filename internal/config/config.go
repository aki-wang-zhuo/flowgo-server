package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config 服务端运行配置。
type Config struct {
	// Addr HTTP 监听地址。
	Addr string
	// DataDir 数据根目录（含 CloverDB）。
	DataDir string
	// DBPath Clover 数据库目录（默认 DataDir/flowgo.db）。
	DBPath string
	// JWTSecret JWT 签名密钥。
	JWTSecret string
	// TokenTTL 访问令牌有效期（编辑器登录）。
	TokenTTL time.Duration
	// PublicBaseURL 对外基址（OAuth issuer / resource）；空则按请求 Host 推断。
	// 本机示例：http://127.0.0.1:8090
	PublicBaseURL string
	// OAuthAccessTTL MCP OAuth access_token 有效期。
	OAuthAccessTTL time.Duration
	// OAuthRefreshTTL MCP OAuth refresh_token 有效期。
	OAuthRefreshTTL time.Duration
	// MCPEnabled 是否启用 MCP。
	MCPEnabled bool
	// CORSOrigin 允许的前端源（开发期可 *）。
	CORSOrigin string
}

// Load 从环境变量加载配置，缺省给出合理默认值。
func Load() Config {
	dataDir := envOr("FLOWGO_DATA_DIR", filepath.Join(".", "data"))
	return Config{
		Addr:            envOr("FLOWGO_ADDR", ":8090"),
		DataDir:         dataDir,
		DBPath:          envOr("FLOWGO_DB_PATH", filepath.Join(dataDir, "flowgo.db")),
		JWTSecret:       envOr("FLOWGO_JWT_SECRET", "flowgo-dev-secret-change-me"),
		TokenTTL:        time.Duration(envInt("FLOWGO_TOKEN_TTL_HOURS", 24)) * time.Hour,
		PublicBaseURL:   envOr("FLOWGO_PUBLIC_BASE_URL", "http://127.0.0.1:8090"),
		OAuthAccessTTL:  time.Duration(envInt("FLOWGO_OAUTH_ACCESS_TTL_MIN", 60)) * time.Minute,
		OAuthRefreshTTL: time.Duration(envInt("FLOWGO_OAUTH_REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,
		MCPEnabled:      envOr("FLOWGO_MCP_ENABLED", "true") == "true",
		CORSOrigin:      envOr("FLOWGO_CORS_ORIGIN", "*"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
