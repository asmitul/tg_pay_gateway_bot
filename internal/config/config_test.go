package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsAndRequired(t *testing.T) {
	unsetEnv(t, KeyAppEnv)
	unsetEnv(t, KeyHTTPPort)
	unsetEnv(t, KeyLogLevel)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "12345")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "tg_bot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.AppEnv != DefaultAppEnv {
		t.Fatalf("expected app env %s, got %s", DefaultAppEnv, cfg.AppEnv)
	}

	if cfg.BotOwnerID != 12345 {
		t.Fatalf("expected bot owner id to be parsed, got %d", cfg.BotOwnerID)
	}

	if cfg.HTTPPort != DefaultHTTPPort {
		t.Fatalf("expected default http port %d, got %d", DefaultHTTPPort, cfg.HTTPPort)
	}

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("expected default log level %s, got %s", DefaultLogLevel, cfg.LogLevel)
	}
}

func TestLoadFailsOnMissingRequired(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	unsetEnv(t, KeyTelegramToken)
	t.Setenv(KeyBotOwner, "999")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "tg_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected missing required env to error")
	}

	if !strings.Contains(err.Error(), KeyTelegramToken) {
		t.Fatalf("expected error to mention missing %s, got %v", KeyTelegramToken, err)
	}
}

func TestLoadValidatesOwnerID(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "abc")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "tg_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid %s", KeyBotOwner)
	}

	if !strings.Contains(err.Error(), KeyBotOwner) {
		t.Fatalf("expected error to mention %s, got %v", KeyBotOwner, err)
	}
}

func TestLoadValidatesHTTPPort(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "123")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "tg_bot")
	t.Setenv(KeyHTTPPort, "-1")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid %s", KeyHTTPPort)
	}

	if !strings.Contains(err.Error(), KeyHTTPPort) {
		t.Fatalf("expected error to mention %s, got %v", KeyHTTPPort, err)
	}
}

func TestLoadUsesDotEnvInDevelopment(t *testing.T) {
	tmpDir := t.TempDir()
	dotenvContent := []byte(`
APP_ENV=development
TELEGRAM_TOKEN=dotenv-token
BOT_OWNER=77
MONGO_URI=mongodb://from-dotenv
MONGO_DB=tg_bot_dev
HTTP_PORT=9091
LOG_LEVEL=debug
`)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), dotenvContent, 0o644); err != nil {
		t.Fatalf("failed to write dotenv: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	unsetEnv(t, KeyAppEnv)
	unsetEnv(t, KeyTelegramToken)
	unsetEnv(t, KeyBotOwner)
	unsetEnv(t, KeyMongoURI)
	unsetEnv(t, KeyMongoDB)
	unsetEnv(t, KeyHTTPPort)
	unsetEnv(t, KeyLogLevel)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected dotenv-backed config to load, got error: %v", err)
	}

	if cfg.AppEnv != EnvDevelopment {
		t.Fatalf("expected development env from dotenv, got %s", cfg.AppEnv)
	}

	if cfg.TelegramToken != "dotenv-token" {
		t.Fatalf("expected token from dotenv, got %s", cfg.TelegramToken)
	}

	if cfg.BotOwnerID != 77 {
		t.Fatalf("expected owner id 77 from dotenv, got %d", cfg.BotOwnerID)
	}

	if cfg.MongoURI != "mongodb://from-dotenv" {
		t.Fatalf("expected mongo uri from dotenv, got %s", cfg.MongoURI)
	}

	if cfg.MongoDB != "tg_bot_dev" {
		t.Fatalf("expected mongo db from dotenv, got %s", cfg.MongoDB)
	}

	if cfg.HTTPPort != 9091 {
		t.Fatalf("expected http port from dotenv, got %d", cfg.HTTPPort)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf("expected log level from dotenv, got %s", cfg.LogLevel)
	}
}

func TestLoadValidatesMongoURIFormat(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "123")
	t.Setenv(KeyMongoURI, "http://localhost:27017")
	t.Setenv(KeyMongoDB, "tg_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid mongo uri to error")
	}

	if !strings.Contains(err.Error(), KeyMongoURI) {
		t.Fatalf("expected error to mention %s, got %v", KeyMongoURI, err)
	}
}

func TestFormatRedactedMasksSecrets(t *testing.T) {
	cfg := Config{
		TelegramToken: "abcd1234secret",
		BotOwnerID:    42,
		MongoURI:      "mongodb://user:pass@localhost:27017/tg_bot",
		MongoDB:       "tg_bot",
		AppEnv:        EnvDevelopment,
		LogLevel:      "debug",
		HTTPPort:      9000,
	}

	summary := FormatRedacted(cfg)

	if strings.Contains(summary, "user:pass@") {
		t.Fatalf("expected mongo uri credentials to be redacted, got %s", summary)
	}

	if !strings.Contains(summary, "mongodb://localhost:27017/tg_bot") {
		t.Fatalf("expected mongo uri host to remain after redaction, got %s", summary)
	}

	if strings.Contains(summary, "1234secret") {
		t.Fatalf("expected telegram token to be redacted, got %s", summary)
	}

	if !strings.Contains(summary, "telegram_token: abcd...redacted") {
		t.Fatalf("expected telegram token to show masked prefix, got %s", summary)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	_ = os.Unsetenv(key)
}
