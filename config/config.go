package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Redis   RedisConfig   `yaml:"redis"`
	API     APIConfig     `yaml:"api"`
	Scraper ScraperConfig `yaml:"scraper"`
	Admin   AdminConfig   `yaml:"admin"`
	Logging LoggingConfig `yaml:"logging"`
	Storage StorageConfig `yaml:"storage"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port string `yaml:"port"`
}

// RedisConfig holds Redis-related configuration
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	CacheTTL string `yaml:"cache_ttl"`
}

// APIConfig holds API behavior configuration
type APIConfig struct {
	Timeout string `yaml:"timeout"`
}

// ScraperConfig holds scraper-related configuration
type ScraperConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}

// AdminConfig holds admin endpoint configuration
type AdminConfig struct {
	Password string `yaml:"password"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Debug bool `yaml:"debug"`
}

// StorageConfig holds paths for files that must survive process restarts (news, season resets).
// DataDir defaults to "data" (under the process working directory). Override with storage.data_dir or DATA_DIR
// (e.g. a Docker volume path).
type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

// Load loads configuration from file and environment variables
func Load() *Config {
	loadDotEnv()

	cfg := &Config{
		Server: ServerConfig{
			Port: "8080",
		},
		Redis: RedisConfig{
			Enabled:  false,
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
			CacheTTL: "24h",
		},
		API: APIConfig{
			Timeout: "5s",
		},
		Scraper: ScraperConfig{
			Enabled:  false,
			Interval: "60m",
		},
		Logging: LoggingConfig{
			Debug: false,
		},
	}

	// Try to load from config.yaml
	if data, err := os.ReadFile("config.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			log.Printf("Warning: Failed to parse config.yaml: %v", err)
		}
	}

	// Override with environment variables
	if port := os.Getenv("PORT"); port != "" {
		cfg.Server.Port = port
	}
	if enabled := os.Getenv("REDIS_ENABLED"); enabled != "" {
		cfg.Redis.Enabled = enabled == "true"
	}
	if host := os.Getenv("REDIS_HOST"); host != "" {
		cfg.Redis.Host = host
	}
	if port := os.Getenv("REDIS_PORT"); port != "" {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err == nil {
			cfg.Redis.Port = p
		}
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		cfg.Redis.Password = password
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		var d int
		if _, err := fmt.Sscanf(db, "%d", &d); err == nil {
			cfg.Redis.DB = d
		}
	}
	if ttl := os.Getenv("CACHE_TTL"); ttl != "" {
		cfg.Redis.CacheTTL = ttl
	}
	if timeout := os.Getenv("API_TIMEOUT"); timeout != "" {
		cfg.API.Timeout = timeout
	}
	if enabled := os.Getenv("SCRAPER_ENABLED"); enabled != "" {
		cfg.Scraper.Enabled = enabled == "true"
	}
	if interval := os.Getenv("SCRAPER_INTERVAL"); interval != "" {
		cfg.Scraper.Interval = interval
	}
	if password := os.Getenv("ADMIN_PASSWORD"); password != "" {
		cfg.Admin.Password = password
	}
	if debug := os.Getenv("DEBUG"); debug != "" {
		cfg.Logging.Debug = debug == "true"
	}
	if d := strings.TrimSpace(os.Getenv("DATA_DIR")); d != "" {
		cfg.Storage.DataDir = d
	}

	if strings.TrimSpace(cfg.Storage.DataDir) == "" {
		cfg.Storage.DataDir = "data"
	}

	cfg.Admin.Password = strings.TrimSpace(cfg.Admin.Password)

	return cfg
}

// loadDotEnv loads .env from the working directory, then from the directory of the
// executable if ADMIN_PASSWORD is still empty (common when running a built binary outside the repo root).
func loadDotEnv() {
	_ = godotenv.Load(".env")
	if strings.TrimSpace(os.Getenv("ADMIN_PASSWORD")) != "" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	}
}

// NewsJSONPath returns the path to the persisted news file.
func (c *Config) NewsJSONPath() string {
	return c.resolveDataFile("news.json")
}

// SeasonResetsJSONPath returns the path to the season reset anchors file.
func (c *Config) SeasonResetsJSONPath() string {
	return c.resolveDataFile("season_resets.json")
}

func (c *Config) resolveDataFile(filename string) string {
	return filepath.Join(strings.TrimSpace(c.Storage.DataDir), filename)
}

// GetCacheTTL parses and returns the cache TTL as a duration
func (c *Config) GetCacheTTL() time.Duration {
	ttl, err := time.ParseDuration(c.Redis.CacheTTL)
	if err != nil {
		log.Printf("Warning: Invalid cache TTL '%s', using default 24h", c.Redis.CacheTTL)
		return 24 * time.Hour
	}
	return ttl
}

// GetAPITimeout parses and returns the API timeout as a duration
func (c *Config) GetAPITimeout() time.Duration {
	timeout, err := time.ParseDuration(c.API.Timeout)
	if err != nil {
		log.Printf("Warning: Invalid API timeout '%s', using default 5s", c.API.Timeout)
		return 5 * time.Second
	}
	return timeout
}

// GetScraperInterval parses and returns the scraper interval as a duration
func (c *Config) GetScraperInterval() time.Duration {
	interval, err := time.ParseDuration(c.Scraper.Interval)
	if err != nil {
		log.Printf("Warning: Invalid scraper interval '%s', using default 60m", c.Scraper.Interval)
		return 60 * time.Minute
	}
	return interval
}
