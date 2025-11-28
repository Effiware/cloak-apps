package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Keycloak struct {
		Url      string `mapstructure:"url"`
		Realm    string `mapstructure:"realm"`
		ClientId string `mapstructure:"client_id"`
	} `mapstructure:"keycloak"`

	Server struct {
		Port    int    `mapstructure:"port"`
		Host    string `mapstructure:"host"`
		Timeout int    `mapstructure:"timeout"`
	} `mapstructure:"server"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.timeout", 10)

	// env overrides
	viper.SetEnvPrefix("CLOAKAPPS")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Server.Timeout < 0 {
		return fmt.Errorf("server.timeout must be positive")
	}

	if c.Keycloak.Url == "" {
		return fmt.Errorf("keycloak.url is required")
	}
	if !strings.HasPrefix(c.Keycloak.Url, "http") {
		return fmt.Errorf("keycloak.url must start with http:// or https://")
	}
	if c.Keycloak.Realm == "" {
		return fmt.Errorf("keycloak.realm is required")
	}

	return nil
}
