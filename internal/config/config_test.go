package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		wantErr    bool
		validate   func(*testing.T, *Config)
	}{
		{
			name:       "valid config",
			configFile: "valid.yaml",
			wantErr:    false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 8083 {
					t.Errorf("expected port 8083, got %d", cfg.Server.Port)
				}
				if cfg.Server.Host != "localhost" {
					t.Errorf("expected host localhost, got %s", cfg.Server.Host)
				}
				if cfg.Keycloak.Realm != "test-realm" {
					t.Errorf("expected realm test-realm, got %s", cfg.Keycloak.Realm)
				}
			},
		},
		{
			name:       "port defaults to 8080",
			configFile: "missing_port.yaml",
			wantErr:    false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 8080 {
					t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
				}
			},
		},
		{
			name:       "timeout defaults to 10",
			configFile: "missing_timeout.yaml",
			wantErr:    false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.Timeout != 10 {
					t.Errorf("expected default timeout 10, got %d", cfg.Server.Timeout)
				}
			},
		},
		{
			name:       "invalid port - negative",
			configFile: "invalid_port_negative.yaml",
			wantErr:    true,
			validate:   nil,
		},
		{
			name:       "invalid port - too high",
			configFile: "invalid_port_high.yaml",
			wantErr:    true,
			validate:   nil,
		},
		{
			name:       "missing keycloak url",
			configFile: "missing_keycloak_url.yaml",
			wantErr:    true,
			validate:   nil,
		},
		{
			name:       "invalid keycloak url",
			configFile: "invalid_keycloak_url.yaml",
			wantErr:    true,
			validate:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			// Change to testdata directory temporarily
			originalDir, _ := os.Getwd()
			testdataDir := filepath.Join(originalDir, "testdata")
			os.Chdir(testdataDir)
			defer os.Chdir(originalDir)

			// Rename test file to "config.yaml" temporarily
			configPath := filepath.Join(testdataDir, "config.yaml")
			testFilePath := filepath.Join(testdataDir, tt.configFile)

			// Copy test file to config.yaml
			data, err := os.ReadFile(testFilePath)
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			err = os.WriteFile(configPath, data, 0644)
			if err != nil {
				t.Fatalf("failed to write config.yaml: %v", err)
			}
			defer os.Remove(configPath)

			// Test LoadConfig
			cfg, err := LoadConfig()

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Run custom validation if provided
			if tt.validate != nil && cfg != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port:    8080,
					Host:    "localhost",
					Timeout: 10,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:      "https://auth.example.com",
					Realm:    "test-realm",
					ClientId: "test-client",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config - http url",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port:    8080,
					Host:    "localhost",
					Timeout: 10,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "http://auth.example.com",
					Realm: "test-realm",
				},
			},
			wantErr: false,
		},
		{
			name: "port too low - zero",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: 0,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "https://auth.example.com",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "port too low - negative",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: -1,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "https://auth.example.com",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "port too high",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: 99999,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "https://auth.example.com",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "timeout negative",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port:    8080,
					Timeout: -5,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "https://auth.example.com",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "missing keycloak url",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: 8080,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid keycloak url - no protocol",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: 8080,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "not-a-url",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid keycloak url - ftp protocol",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: 8080,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "ftp://auth.example.com",
					Realm: "test",
				},
			},
			wantErr: true,
		},
		{
			name: "missing keycloak realm",
			config: Config{
				Server: struct {
					Port    int    `mapstructure:"port"`
					Host    string `mapstructure:"host"`
					Timeout int    `mapstructure:"timeout"`
				}{
					Port: 8080,
				},
				Keycloak: struct {
					Url      string `mapstructure:"url"`
					Realm    string `mapstructure:"realm"`
					ClientId string `mapstructure:"client_id"`
				}{
					Url:   "https://auth.example.com",
					Realm: "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
