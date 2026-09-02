// internal/config/config.go
package config

import (
	"os"
	"path/filepath"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultModel string           `yaml:"default_model"`
	Providers    []ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	APIKeys []string `yaml:"api_keys"`
}

func GetTermuxHome() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	return "/data/data/com.termux/files/home"
}

func GetTermuxPrefix() string {
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		return prefix
	}
	return "/data/data/com.termux/files/usr"
}

func GetConfigPath() string {
	return filepath.Join(GetTermuxHome(), ".config", "aicli", "config.yaml")
}

func LoadConfig() (*Config, error) {
	configPath := GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultCfg := Config{
			DefaultModel: "gemini",
			Providers: []ProviderConfig{
				{Name: "gemini", Type: "gemini", APIKeys: []string{}},
			},
		}
		_ = SaveConfig(&defaultCfg)
		return &defaultCfg, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	configPath := GetConfigPath()
	_ = os.MkdirAll(filepath.Dir(configPath), 0755)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}
