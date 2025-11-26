// Package config defines the configuration contract and will handle loading and validating environment configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	// Canonical environment variable keys.
	KeyTelegramToken = "TELEGRAM_TOKEN"
	KeyBotOwner      = "BOT_OWNER"
	KeyMongoURI      = "MONGO_URI"
	KeyMongoDB       = "MONGO_DB"
	KeyAppEnv        = "APP_ENV"
	KeyLogLevel      = "LOG_LEVEL"
	KeyHTTPPort      = "HTTP_PORT"

	// Allowed environment values.
	EnvDevelopment = "development"
	EnvProduction  = "production"

	// Defaults for optional settings.
	DefaultAppEnv   = EnvProduction
	DefaultLogLevel = "info"
	DefaultHTTPPort = 8080

	// Recommended database names by environment.
	DefaultMongoDBProd = "tg_bot"
	DefaultMongoDBDev  = "tg_bot_dev"
)

// VarSpec describes a single configuration key.
type VarSpec struct {
	Key         string // environment variable name
	Example     string // human-friendly sample value
	Required    bool   // whether the bot must refuse to start without this value
	Default     string // default when unset (empty when required)
	Description string // what the variable controls
	Notes       string // extra guidance or policies
}

// Contract enumerates the authoritative configuration keys for the bot.
// .env loading is only permitted when APP_ENV=development; production must rely
// on environment variables supplied by the runtime.
var Contract = []VarSpec{
	{
		Key:         KeyTelegramToken,
		Example:     "123:ABC",
		Required:    true,
		Description: "Telegram Bot Token issued by BotFather.",
	},
	{
		Key:         KeyBotOwner,
		Example:     "123456789",
		Required:    true,
		Description: "Super admin Telegram user_id with owner privileges.",
	},
	{
		Key:         KeyMongoURI,
		Example:     "mongodb://localhost:27017",
		Required:    true,
		Description: "MongoDB connection string.",
	},
	{
		Key:         KeyMongoDB,
		Example:     DefaultMongoDBProd + " / " + DefaultMongoDBDev,
		Required:    true,
		Description: "MongoDB database name.",
		Notes:       "Recommended: production=" + DefaultMongoDBProd + ", development=" + DefaultMongoDBDev + ".",
	},
	{
		Key:         KeyAppEnv,
		Example:     EnvDevelopment + " / " + EnvProduction,
		Default:     DefaultAppEnv,
		Description: "Runtime environment; controls log format and dotenv usage.",
		Notes:       "Load .env files only when APP_ENV=" + EnvDevelopment + ".",
	},
	{
		Key:         KeyLogLevel,
		Example:     DefaultLogLevel,
		Default:     DefaultLogLevel,
		Description: "Overrides default log level.",
	},
	{
		Key:         KeyHTTPPort,
		Example:     strconv.Itoa(DefaultHTTPPort),
		Default:     strconv.Itoa(DefaultHTTPPort),
		Description: "HTTP health/diagnostics port.",
	},
}

// Config mirrors resolved configuration values after loading.
type Config struct {
	TelegramToken string
	BotOwnerID    int64
	MongoURI      string
	MongoDB       string
	AppEnv        string
	LogLevel      string
	HTTPPort      int
}

// Load resolves configuration from the environment (with optional dotenv in development).
func Load() (Config, error) {
	appEnv, err := resolveAppEnv()
	if err != nil {
		return Config{}, err
	}

	if err := loadDotEnv(appEnv); err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:        firstNonEmpty(normalizeEnv(os.Getenv(KeyAppEnv)), appEnv),
		TelegramToken: strings.TrimSpace(os.Getenv(KeyTelegramToken)),
		MongoURI:      strings.TrimSpace(os.Getenv(KeyMongoURI)),
		MongoDB:       strings.TrimSpace(os.Getenv(KeyMongoDB)),
		LogLevel:      firstNonEmpty(strings.TrimSpace(os.Getenv(KeyLogLevel)), DefaultLogLevel),
		HTTPPort:      DefaultHTTPPort,
	}

	if err := validateAppEnv(cfg.AppEnv); err != nil {
		return Config{}, err
	}

	missing := make([]string, 0)

	if cfg.TelegramToken == "" {
		missing = append(missing, KeyTelegramToken)
	}

	ownerRaw := strings.TrimSpace(os.Getenv(KeyBotOwner))
	if ownerRaw == "" {
		missing = append(missing, KeyBotOwner)
	} else {
		ownerID, parseErr := strconv.ParseInt(ownerRaw, 10, 64)
		if parseErr != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", KeyBotOwner, parseErr)
		}
		cfg.BotOwnerID = ownerID
	}

	if cfg.MongoURI == "" {
		missing = append(missing, KeyMongoURI)
	} else if err := validateMongoURI(cfg.MongoURI); err != nil {
		return Config{}, err
	}

	if cfg.MongoDB == "" {
		missing = append(missing, KeyMongoDB)
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}

	httpPortRaw := strings.TrimSpace(os.Getenv(KeyHTTPPort))
	if httpPortRaw != "" {
		port, parseErr := strconv.Atoi(httpPortRaw)
		if parseErr != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", KeyHTTPPort, parseErr)
		}
		if port <= 0 {
			return Config{}, fmt.Errorf("%s must be greater than 0", KeyHTTPPort)
		}
		cfg.HTTPPort = port
	}

	return cfg, nil
}

// IsDevelopment reports if APP_ENV is development.
func (c Config) IsDevelopment() bool {
	return c.AppEnv == EnvDevelopment
}

// FormatRedacted returns a human-readable, secret-safe summary of the resolved configuration.
// Secrets such as TELEGRAM_TOKEN and MongoDB credentials are redacted.
func FormatRedacted(cfg Config) string {
	lines := []string{
		"app_env: " + cfg.AppEnv,
		fmt.Sprintf("bot_owner: %d", cfg.BotOwnerID),
		"telegram_token: " + maskSecret(cfg.TelegramToken),
		"mongo_uri: " + redactMongoURI(cfg.MongoURI),
		"mongo_db: " + cfg.MongoDB,
		"log_level: " + cfg.LogLevel,
		fmt.Sprintf("http_port: %d", cfg.HTTPPort),
	}

	return strings.Join(lines, "\n")
}

func resolveAppEnv() (string, error) {
	if explicit := normalizeEnv(os.Getenv(KeyAppEnv)); explicit != "" {
		return explicit, nil
	}

	dotEnvValues, err := godotenv.Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultAppEnv, nil
		}
		return "", fmt.Errorf("read .env: %w", err)
	}

	if envFromFile := normalizeEnv(dotEnvValues[KeyAppEnv]); envFromFile != "" {
		return envFromFile, nil
	}

	return DefaultAppEnv, nil
}

func loadDotEnv(appEnv string) error {
	if appEnv != EnvDevelopment {
		return nil
	}

	if err := godotenv.Load(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load .env: %w", err)
	}

	return nil
}

func validateAppEnv(appEnv string) error {
	if appEnv == EnvDevelopment || appEnv == EnvProduction {
		return nil
	}

	return fmt.Errorf("invalid %s: must be %q or %q", KeyAppEnv, EnvDevelopment, EnvProduction)
}

func validateMongoURI(uri string) error {
	parsed, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", KeyMongoURI, err)
	}

	if parsed.Scheme != "mongodb" && parsed.Scheme != "mongodb+srv" {
		return fmt.Errorf("invalid %s: unsupported scheme %q", KeyMongoURI, parsed.Scheme)
	}

	if parsed.Host == "" {
		return fmt.Errorf("invalid %s: missing host", KeyMongoURI)
	}

	return nil
}

func normalizeEnv(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "<empty>"
	}

	if len(value) <= 4 {
		return "***"
	}

	return value[:4] + "...redacted"
}

func redactMongoURI(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "<invalid>"
	}

	parsed.User = nil

	return parsed.String()
}
