package config

import (
	"os"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	SSH      SSHConfig
	JWT      JWTConfig
}

type ServerConfig struct {
	Port        int
	Environment string
}

type DatabaseConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
}

type SSHConfig struct {
	Host           string
	Username       string
	Password       string
	RemoteBindHost string
	RemoteBindPort int
}

type JWTConfig struct {
	Secret    string
	ExpiresIn string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:        getIntEnv("PORT", 4174),
			Environment: getEnv("GIN_MODE", "debug"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "127.0.0.1"),
			Port:     getIntEnv("DB_PORT", 5432),
			Name:     getEnv("DB_NAME", "sensorsdb"),
			User:     getEnv("DB_USER", "sa"),
			Password: getEnv("DB_PASSWORD", "s3rv3r5mxdb"),
		},
		SSH: SSHConfig{
			Host:           getEnv("SSH_HOST", "41.191.232.15"),
			Username:       getEnv("SSH_USERNAME", "sa"),
			Password:       getEnv("SSH_PASSWORD", "s3rv3r5mx$"),
			RemoteBindHost: getEnv("REMOTE_BIND_HOST", "127.0.0.1"),
			RemoteBindPort: getIntEnv("REMOTE_BIND_PORT", 5437),
		},
		JWT: JWTConfig{
			Secret:    getEnv("JWT_SECRET", "fuel-monitor-secret-key-2024"),
			ExpiresIn: getEnv("JWT_EXPIRES_IN", "24h"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}