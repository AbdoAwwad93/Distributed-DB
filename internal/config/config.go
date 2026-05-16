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
	NodeID              string
	Host                string
	Port                string
	PublicURL           string
	MasterURL           string
	MySQLDSN            string
	DBName              string
	SlaveURLs           []string
	HealthCheckInterval time.Duration
}

func LoadMasterConfig() NodeConfig {
	LoadDotEnv(".env")
	dsn := envOrDefault("MYSQL_DSN", "root:@tcp(localhost:3306)/distributed_master?parseTime=true")
	port := envOrDefault("MASTER_PORT", "8080")

	return NodeConfig{
		Role:                "master",
		NodeID:              envOrDefault("MASTER_ID", "master"),
		Host:                envOrDefault("MASTER_HOST", "0.0.0.0"),
		Port:                port,
		PublicURL:           envOrDefault("MASTER_PUBLIC_URL", "http://localhost:"+port),
		MasterURL:           envOrDefault("MASTER_URL", "http://localhost:"+port),
		MySQLDSN:            dsn,
		DBName:              databaseNameFromDSN(dsn),
		SlaveURLs:           csvEnv("SLAVE_URLS"),
		HealthCheckInterval: 5 * time.Second,
	}
}

func LoadSlaveConfig() NodeConfig {
	LoadDotEnv(".env")
	port := envOrDefault("NODE_PORT", "8081")
	dsn := envOrDefault("NODE_MYSQL_DSN", "root:@tcp(localhost:3306)/distributed_slave?parseTime=true")
	nodeID := envOrDefault("NODE_ID", "slave-"+port)

	return NodeConfig{
		Role:      "slave",
		NodeID:    nodeID,
		Host:      envOrDefault("NODE_HOST", "0.0.0.0"),
		Port:      port,
		PublicURL: envOrDefault("NODE_PUBLIC_URL", "http://localhost:"+port),
		MasterURL: envOrDefault("MASTER_URL", "http://localhost:8080"),
		MySQLDSN:  dsn,
		DBName:    databaseNameFromDSN(dsn),
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
