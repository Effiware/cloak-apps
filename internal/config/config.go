package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config struct holds file structure of a configuration file
type Config struct {
	Keycloak struct {
		Url          string `mapstructure:"url"`
		Realm        string `mapstructure:"realm"`
		ClientId     string `mapstructure:"client_id"`
		ClientSecret string `mapstructure:"client_secret"`
		RedirectUri  string `mapstructure:"redirect_uri"`
	} `mapstructure:"keycloak"`

	Server struct {
		Port    int    `mapstructure:"port"`
		Host    string `mapstructure:"host"`
		Timeout int    `mapstructure:"timeout"`
	} `mapstructure:"server"`

	Session struct {
		Secret string `mapstructure:"secret"`
		MaxAge string `mapstructure:"max_age"`
	} `mapstructure:"session"`
}

// LoadConfig reads and validates configuration
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

// Validate is used to check configuration values
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
