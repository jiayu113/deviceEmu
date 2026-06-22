package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 对应 config.yaml 全量结构
type Config struct {
	Device DeviceConfig `yaml:"device"`
	MQTT   MQTTConfig   `yaml:"mqtt"`
	SIP    SIPConfig    `yaml:"sip"`
}

type DeviceConfig struct {
	ID string `yaml:"id"`
}

type MQTTConfig struct {
	Broker            string `yaml:"broker"`
	Username          string `yaml:"username"`
	Password          string `yaml:"password"`
	KeepaliveSeconds  int    `yaml:"keepalive_seconds"`
	TelemetryInterval int    `yaml:"telemetry_interval_seconds"`
}

type SIPConfig struct {
	Server                string `yaml:"server"`
	Username              string `yaml:"username"`
	Password              string `yaml:"password"`
	Domain                string `yaml:"domain"`
	RegisterExpirySeconds int    `yaml:"register_expiry_seconds"`
	LocalHost             string `yaml:"local_host"`
	LocalPort             int    `yaml:"local_port"`
	Callee                string `yaml:"callee"`
}

// Load 读取并解析 yaml 配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}
