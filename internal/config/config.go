package config

import (
	"os"
	"path/filepath"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultModel      string           `yaml:"default_model"`
	SystemInstruction string           `yaml:"system_instruction"`
	Providers         []ProviderConfig `yaml:"providers"`
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

func GetConfigPath() string {
	return filepath.Join(GetTermuxHome(), ".config", "aicli", "config.yaml")
}

func GetRulesPath() string {
	return filepath.Join(GetTermuxHome(), ".config", "aicli", "rules.yaml")
}

func LoadConfig() (*Config, error) {
	configPath := GetConfigPath()
	rulesPath := GetRulesPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultCfg := Config{
			DefaultModel:      "gemini",
			SystemInstruction: "Aktifkan mode JARGO Lead Software Architect & Developer. Senior System Architect & Full-Stack Engineer mindset. Zero-Alloc, Clean Code, SOLID, DRY, KISS.",
			Providers: []ProviderConfig{
				{Name: "gemini", Type: "gemini", APIKeys: []string{}},
			},
		}
		_ = SaveConfig(&defaultCfg)
		_ = os.WriteFile(rulesPath, []byte("system_instruction: \""+defaultCfg.SystemInstruction+"\"\n"), 0644)
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

	// Muat aturan tambahan dari rules.yaml jika tersedia
	if data, err := os.ReadFile(rulesPath); err == nil {
		var ruleStruct struct {
			SystemInstruction string `yaml:"system_instruction"`
		}
		if yaml.Unmarshal(data, &ruleStruct) == nil && ruleStruct.SystemInstruction != "" {
			cfg.SystemInstruction = ruleStruct.SystemInstruction
		}
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
