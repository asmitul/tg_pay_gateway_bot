// Package config defines the configuration contract and will handle loading and validating environment configuration.
package config

import "strconv"

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
