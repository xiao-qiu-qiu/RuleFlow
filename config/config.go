package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config 应用程序配置
type Config struct {
	// 服务器配置
	Port string

	// 允许的 CORS 来源（逗号分隔，默认 "*"）
	CORSAllowedOrigins string

	// PostgreSQL 配置
	DatabaseURL string

	// Redis 配置
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// 缓存配置
	CacheTTLSeconds int

	// 日志清理配置
	LogKeepDays      int
	LogMaxRecords    int
	LogCheckInterval int

	// 入口网关：随机入口路径 + 伪装页
	// EntryPath 为空表示关闭该功能，行为与旧版本一致
	EntryPath string
	// EntryCookie 是放行面板内部请求（绝对路径跳转、/api、/assets）的 cookie 值
	// 留空时由 EntryPath 派生
	EntryCookie string
	// SubPath 是订阅/规则集下发的独立入口，只放行 /subscribe、/universal-sub、
	// /rulesets/*、/convert，且不会写入入口 cookie（拿到订阅地址不等于能进面板）。
	// 留空表示订阅与面板共用 EntryPath。
	SubPath string
}

// Load 从环境变量加载配置
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:               getEnv("PORT", "8080"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgresql://ruleflow:password@localhost:5432/ruleflow?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		RedisDB:            getEnvInt("REDIS_DB", 0),
		CacheTTLSeconds:    getEnvInt("CACHE_TTL_SECONDS", 3600),
		LogKeepDays:        getEnvInt("LOG_KEEP_DAYS", 30),
		LogMaxRecords:      getEnvInt("LOG_MAX_RECORDS", 10000),
		LogCheckInterval:   getEnvInt("LOG_CHECK_INTERVAL", 1),
		EntryPath:          normalizeEntryPath(getEnv("RF_ENTRY_PATH", "")),
		EntryCookie:        strings.TrimSpace(getEnv("RF_ENTRY_COOKIE", "")),
		SubPath:            normalizeEntryPath(getEnv("RF_SUB_PATH", "")),
	}
}

// normalizeEntryPath 把入口路径规范成 "/xxx"（无尾斜杠、必须带前导斜杠）
// 空值或 "/" 返回空字符串，表示关闭入口网关
func normalizeEntryPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	if value == "/" {
		return ""
	}
	return value
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
