package config

import "fmt"

type NodeConfig struct {
	Role string
	Host string
	Port string
}

func LoadMasterConfig() NodeConfig {
	return NodeConfig{
		Role: "master",
		Host: "localhost",
		Port: "8080",
	}
}

func (c NodeConfig) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}
