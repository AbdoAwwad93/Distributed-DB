package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type NodeConfig struct {
	Role     string
	Host     string
	Port     string
	MySQLDSN string
	DBName   string
}

func LoadMasterConfig() NodeConfig {
	LoadDotEnv(".env")

	dsn := envOrDefault("MYSQL_DSN", "root:@tcp(localhost:3306)/distributed_master?parseTime=true")

	return NodeConfig{
		Role:     "master",
		Host:     envOrDefault("MASTER_HOST", "localhost"),
		Port:     envOrDefault("MASTER_PORT", "8080"),
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

func databaseNameFromDSN(dsn string) string {
	_, afterSlash, found := strings.Cut(dsn, ")/")
	if !found {
		return ""
	}

	dbName, _, _ := strings.Cut(afterSlash, "?")
	return dbName
}
