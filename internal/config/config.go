package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type NodeConfig struct {
	Role                string
	Host                string
	Port                string
	MySQLDSN            string
	DBName              string
	SlaveURLs           []string
	HealthCheckInterval time.Duration
}

func LoadMasterConfig() NodeConfig {
	LoadDotEnv(".env")
	dsn := envOrDefault("MYSQL_DSN", "root:@tcp(localhost:3306)/distributed_master?parseTime=true")

	return NodeConfig{
		Role:                "master",
		Host:                envOrDefault("MASTER_HOST", "localhost"),
		Port:                envOrDefault("MASTER_PORT", "8080"),
		MySQLDSN:            dsn,
		DBName:              databaseNameFromDSN(dsn),
		SlaveURLs:           csvEnv("SLAVE_URLS"),
		HealthCheckInterval: 5*time.Second,
	}
}

func LoadSlaveConfig(role, defaultPort, dsnEnv, defaultDSN string) NodeConfig {
	LoadDotEnv(".env")
	dsn := envOrDefault(dsnEnv, defaultDSN)

	return NodeConfig{
		Role:     role,
		Host:     envOrDefault(strings.ToUpper(role)+"_HOST", "localhost"),
		Port:     envOrDefault(strings.ToUpper(role)+"_PORT", defaultPort),
		MySQLDSN: dsn,
		DBName:   databaseNameFromDSN(dsn),
	}
}

func (c NodeConfig) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func LoadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func csvEnv(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}

	return items
}

func databaseNameFromDSN(dsn string) string {
	_, afterSlash, found := strings.Cut(dsn, ")/")
	if !found {
		return ""
	}

	dbName, _, _ := strings.Cut(afterSlash, "?")
	return dbName
}
