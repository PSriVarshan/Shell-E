package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	ModelPath    string  `mapstructure:"model_path"`
	LlamaBinPath string  `mapstructure:"llama_bin_path"`
	SystemPrompt string  `mapstructure:"system_prompt"`
	ContextSize  int     `mapstructure:"context_size"`
	Temperature  float64 `mapstructure:"temperature"`
	TopK         int     `mapstructure:"top_k"`
	TopP         float64 `mapstructure:"top_p"`
	Shell        string  `mapstructure:"shell"` // "powershell" or "cmd"
	DataDir      string  `mapstructure:"data_dir"`
	ServerPort   int     `mapstructure:"server_port"` // Port for llama-server
}

// DataDirectory returns the resolved data directory path
func (c *Config) DataDirectory() string {
	if c.DataDir != "" {
		return c.DataDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".shell-e")
}

func LoadConfig() (*Config, error) {
	viper.SetDefault("model_path", "assets/localmodel/qwen2.5-3b-instruct-q4_k_m.gguf")
	viper.SetDefault("llama_bin_path", "assets/bin/llama-server.exe")
	viper.SetDefault("system_prompt", "You are Shell-E, an AI-powered OS command assistant.")
	viper.SetDefault("context_size", 4096)
	viper.SetDefault("temperature", 0.1)
	viper.SetDefault("top_k", 40)
	viper.SetDefault("top_p", 0.9)
	viper.SetDefault("shell", "powershell")
	viper.SetDefault("data_dir", "")
	viper.SetDefault("server_port", 8055)

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.shell-e")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Config file not found, using defaults")
		} else {
			return nil, err
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Ensure data directory exists
	dataDir := config.DataDirectory()
	os.MkdirAll(filepath.Join(dataDir, "memory"), 0755)

	return &config, nil
}
