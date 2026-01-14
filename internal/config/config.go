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
		Port               int    `mapstructure:"port"`
		Host               string `mapstructure:"host"`
		Timeout            int    `mapstructure:"timeout"`
		LogLevel           string `mapstructure:"log_level"`
		RefreshIntervalMin int    `mapstructure:"refresh_interval_min"`
	} `mapstructure:"server"`

	Session struct {
		Secret                string `mapstructure:"secret"`
		MaxAge                int    `mapstructure:"max_age"`
		Secure                bool   `mapstructure:"secure"`
		Store                 string `mapstructure:"store"`                   // Options: "filesystem", "cookie", "redis"
		CookieIntrospection   bool   `mapstructure:"cookie_introspection"`    // Only for store=cookie
		IntrospectionCacheTTL int    `mapstructure:"introspection_cache_ttl"` // Cache TTL in seconds
		RedisURL              string `mapstructure:"redis_url"`               // Only for store=redis
	} `mapstructure:"session"`

	Organization struct {
		Name              string `mapstructure:"name"`
		HomeUrl           string `mapstructure:"home_url"`
		CustomDescription string `mapstructure:"custom_description"`
	} `mapstructure:"organization"`
}

// LoadConfig reads and validates configuration
func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.timeout", 10)
	viper.SetDefault("server.log_level", "INFO")
	viper.SetDefault("server.refresh_interval_min", 5)
	viper.SetDefault("session.max_age", 3600)
	viper.SetDefault("session.secure", true)
	viper.SetDefault("session.store", "filesystem")
	viper.SetDefault("session.cookie_introspection", false)
	viper.SetDefault("session.introspection_cache_ttl", 15)
	viper.SetDefault("organization.name", "Effiware")
	viper.SetDefault("organization.home_url", "https://effiware.com")

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
	if c.Server.RefreshIntervalMin < 0 {
		return fmt.Errorf("server.refresh_interval_min must be positive")
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
	if c.Session.Secret == "" || len(c.Session.Secret) < 32 {
		return fmt.Errorf("session.secret must have min length of 32 chars to work properly")
	}
	if c.Session.MaxAge < 0 {
		return fmt.Errorf("session.max_age must be positive")
	}

	// Validate session store type
	validStores := []string{"filesystem", "cookie", "redis"}
	isValidStore := false
	for _, store := range validStores {
		if c.Session.Store == store {
			isValidStore = true
			break
		}
	}
	if !isValidStore {
		return fmt.Errorf("session.store must be one of: %v, got '%s'", validStores, c.Session.Store)
	}

	// Validate redis configuration
	if c.Session.Store == "redis" && c.Session.RedisURL == "" {
		return fmt.Errorf("session.redis_url is required when session.store is 'redis'")
	}

	// Validate introspection cache TTL
	if c.Session.IntrospectionCacheTTL < 0 {
		return fmt.Errorf("session.introspection_cache_ttl must be positive")
	}

	return nil
}
