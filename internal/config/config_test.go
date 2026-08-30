package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
				assert.Equal(t, 8083, cfg.Server.Port)
				assert.Equal(t, "localhost", cfg.Server.Host)
				assert.Equal(t, "test-realm", cfg.Keycloak.Realm)
			},
		},
		{
			name:       "port defaults to 8080",
			configFile: "missing_port.yaml",
			wantErr:    false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 8080, cfg.Server.Port)
			},
		},
		{
			name:       "timeout defaults to 10",
			configFile: "missing_timeout.yaml",
			wantErr:    false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 10, cfg.Server.Timeout)
			},
		},
		{
			name:       "invalid port - negative",
			configFile: "invalid_port_negative.yaml",
			wantErr:    true,
		},
		{
			name:       "invalid port - too high",
			configFile: "invalid_port_high.yaml",
			wantErr:    true,
		},
		{
			name:       "missing keycloak url",
			configFile: "missing_keycloak_url.yaml",
			wantErr:    true,
		},
		{
			name:       "invalid keycloak url",
			configFile: "invalid_keycloak_url.yaml",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			originalDir, err := os.Getwd()
			assert.NoError(t, err)
			testdataDir := filepath.Join(originalDir, "testdata")

			err = os.Chdir(testdataDir)
			assert.NoError(t, err)
			defer os.Chdir(originalDir)

			configPath := filepath.Join(testdataDir, "config.yaml")
			testFilePath := filepath.Join(testdataDir, tt.configFile)

			data, err := os.ReadFile(testFilePath)
			assert.NoError(t, err)

			err = os.WriteFile(configPath, data, 0644)
			assert.NoError(t, err)
			defer os.Remove(configPath)

			cfg, err := LoadConfig()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			setup:   validBaselineConfig,
			wantErr: false,
		},
		{
			name: "valid config - http url",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Keycloak.Url = "http://auth.example.com"
				return cfg
			},
			wantErr: false,
		},
		{
			name: "port too low - zero",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Server.Port = 0
				return cfg
			},
			wantErr: true,
		},
		{
			name: "port too low - negative",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Server.Port = -1
				return cfg
			},
			wantErr: true,
		},
		{
			name: "port too high",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Server.Port = 99999
				return cfg
			},
			wantErr: true,
		},
		{
			name: "timeout negative",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Server.Timeout = -5
				return cfg
			},
			wantErr: true,
		},
		{
			name: "missing keycloak url",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Keycloak.Url = ""
				return cfg
			},
			wantErr: true,
		},
		{
			name: "invalid keycloak url - no protocol",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Keycloak.Url = "not-a-url"
				return cfg
			},
			wantErr: true,
		},
		{
			name: "invalid keycloak url - ftp protocol",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Keycloak.Url = "ftp://auth.example.com"
				return cfg
			},
			wantErr: true,
		},
		{
			name: "missing keycloak realm",
			setup: func() *Config {
				cfg := validBaselineConfig()
				cfg.Keycloak.Realm = ""
				return cfg
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setup()
			err := cfg.Validate()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to create a valid baseline config for detailed validation tests
func validBaselineConfig() *Config {
	cfg := &Config{}
	cfg.Server.Name = "cloak-apps"
	cfg.Server.Port = 8080
	cfg.Server.Timeout = 10
	cfg.Server.RefreshIntervalMin = 5
	cfg.Keycloak.Url = "https://keycloak.example.com"
	cfg.Keycloak.Realm = "test-realm"
	cfg.Session.Secret = "this-is-a-very-secure-secret-with-32-chars-minimum"
	cfg.Session.MaxAge = 3600
	cfg.Session.Store = "filesystem"
	cfg.Session.IntrospectionCacheTTL = 15
	cfg.Otlp.Protocol = "grpc"
	return cfg
}

// Server.Name and Otlp.Protocol validation tests

func TestValidate_ServerName_Empty(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Server.Name = ""
	assert.Error(t, cfg.Validate())
}

func TestValidate_OtlpProtocol(t *testing.T) {
	for _, tt := range []struct {
		protocol string
		wantErr  bool
	}{
		{"grpc", false},
		{"http", false},
		{"", true},
		{"HTTP", true},
		{"thrift", true},
	} {
		t.Run(tt.protocol, func(t *testing.T) {
			cfg := validBaselineConfig()
			cfg.Otlp.Protocol = tt.protocol
			if tt.wantErr {
				assert.Error(t, cfg.Validate())
			} else {
				assert.NoError(t, cfg.Validate())
			}
		})
	}
}

// Server.RefreshIntervalMin validation tests

func TestValidate_ServerRefreshIntervalMin_Negative(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Server.RefreshIntervalMin = -1
	err := cfg.Validate()

	assert.ErrorContains(t, err, "server.refresh_interval_min must be positive")
}

func TestValidate_ServerRefreshIntervalMin_Zero(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Server.RefreshIntervalMin = 0
	err := cfg.Validate()
	assert.NoError(t, err, "Zero refresh interval should be valid")
}

func TestValidate_ServerRefreshIntervalMin_Positive(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Server.RefreshIntervalMin = 10
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Session.Secret validation tests

func TestValidate_SessionSecret_Empty(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Secret = ""
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.secret must have min length of 32 chars")
}

func TestValidate_SessionSecret_TooShort(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Secret = strings.Repeat("x", 31)
	assert.Len(t, cfg.Session.Secret, 31, "Test setup: secret should be exactly 31 chars")
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.secret must have min length of 32 chars")
}

func TestValidate_SessionSecret_ExactlyMinLength(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Secret = strings.Repeat("a", 32)
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_SessionSecret_LongSecret(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Secret = strings.Repeat("a", 64)
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Session.MaxAge validation tests

func TestValidate_SessionMaxAge_Negative(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.MaxAge = -1
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.max_age must be positive")
}

func TestValidate_SessionMaxAge_Zero(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.MaxAge = 0
	err := cfg.Validate()
	assert.NoError(t, err, "Zero max_age should be valid (session-only cookie)")
}

func TestValidate_SessionMaxAge_Positive(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.MaxAge = 7200
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Session.Store validation tests

func TestValidate_SessionStore_Invalid(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "invalid-store"
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.store must be one of:")
	assert.ErrorContains(t, err, "invalid-store")
}

func TestValidate_SessionStore_Filesystem(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "filesystem"
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_SessionStore_Cookie(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "cookie"
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_SessionStore_Redis(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "redis"
	cfg.Session.RedisURL = "redis://localhost:6379"
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_SessionStore_CaseSensitive(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "FileSystem" // Wrong case
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.store must be one of:")
}

// Session.RedisURL validation tests

func TestValidate_SessionRedisURL_MissingWhenRedisStore(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "redis"
	cfg.Session.RedisURL = ""
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.redis_url is required when session.store is 'redis'")
}

func TestValidate_SessionRedisURL_ProvidedForRedis(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "redis"
	cfg.Session.RedisURL = "redis://localhost:6379"
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_SessionRedisURL_NotRequiredForFilesystem(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "filesystem"
	cfg.Session.RedisURL = ""
	err := cfg.Validate()
	assert.NoError(t, err, "RedisURL should not be required for filesystem store")
}

func TestValidate_SessionRedisURL_NotRequiredForCookie(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.Store = "cookie"
	cfg.Session.RedisURL = ""
	err := cfg.Validate()
	assert.NoError(t, err, "RedisURL should not be required for cookie store")
}

// Session.IntrospectionCacheTTL validation tests

func TestValidate_IntrospectionCacheTTL_Negative(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.IntrospectionCacheTTL = -1
	err := cfg.Validate()

	assert.ErrorContains(t, err, "session.introspection_cache_ttl must be positive")
}

func TestValidate_IntrospectionCacheTTL_Zero(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.IntrospectionCacheTTL = 0
	err := cfg.Validate()
	assert.NoError(t, err, "Zero cache TTL should be valid (caching disabled)")
}

func TestValidate_IntrospectionCacheTTL_Positive(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Session.IntrospectionCacheTTL = 30
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Boundary value tests

func TestValidate_ServerPort_Boundaries(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"Port at lower boundary", 1, false},
		{"Port at upper boundary", 65535, false},
		{"Port below lower boundary", 0, true},
		{"Port above upper boundary", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaselineConfig()
			cfg.Server.Port = tt.port
			err := cfg.Validate()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_SessionSecret_Boundaries(t *testing.T) {
	tests := []struct {
		name       string
		secretLen  int
		wantErr    bool
		errMessage string
	}{
		{"Secret 31 chars (too short)", 31, true, "session.secret must have min length of 32 chars"},
		{"Secret exactly 32 chars", 32, false, ""},
		{"Secret 33 chars", 33, false, ""},
		{"Secret 64 chars", 64, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaselineConfig()
			cfg.Session.Secret = strings.Repeat("x", tt.secretLen)
			err := cfg.Validate()

			if tt.wantErr {
				assert.ErrorContains(t, err, tt.errMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Multiple validation errors (first error should be returned)

func TestValidate_MultipleErrors_ReturnsFirstError(t *testing.T) {
	cfg := validBaselineConfig()
	cfg.Server.Port = 0          // Invalid port (checked first)
	cfg.Keycloak.Url = ""        // Invalid URL
	cfg.Session.Secret = "short" // Invalid secret

	err := cfg.Validate()

	assert.ErrorContains(t, err, "server.port must be between 1 and 65535")
}

// All store types validation

func TestValidate_AllStoreTypes(t *testing.T) {
	stores := []struct {
		name     string
		redisURL string
		wantErr  bool
	}{
		{"filesystem", "", false},
		{"cookie", "", false},
		{"redis", "redis://localhost:6379", false},
		{"redis", "", true}, // Redis without URL should fail
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBaselineConfig()
			cfg.Session.Store = tc.name
			cfg.Session.RedisURL = tc.redisURL

			err := cfg.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
