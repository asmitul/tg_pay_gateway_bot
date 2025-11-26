package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"tg_pay_gateway_bot/internal/config"
	"tg_pay_gateway_bot/internal/logging"
	"tg_pay_gateway_bot/internal/store"
)

const (
	mongoConnectTimeout    = 10 * time.Second
	mongoIndexTimeout      = 5 * time.Second
	mongoDisconnectTimeout = 5 * time.Second
)

func main() {
	configOnly := flag.Bool("config-only", false, "load and print configuration then exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		logging.Error("configuration error", logging.Fields{"error": err})
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.Setup(cfg)
	if err != nil {
		logging.Error("logger setup error", logging.Fields{"error": err})
		fmt.Fprintf(os.Stderr, "logger setup error: %v\n", err)
		os.Exit(1)
	}

	if *configOnly {
		logging.Info("configuration check", logging.Fields{"event": "config_only"})
		fmt.Println("configuration check: ok")
		fmt.Println(config.FormatRedacted(cfg))
		return
	}

	logger.WithFields(logging.Fields{
		"event":    "startup",
		"mongo_db": cfg.MongoDB,
	}).Info("configuration loaded")

	connectCtx, cancel := context.WithTimeout(context.Background(), mongoConnectTimeout)
	mongoManager, err := store.NewManager(connectCtx, cfg)
	cancel()
	if err != nil {
		logger.WithError(err).Error("mongo connection error")
		fmt.Fprintf(os.Stderr, "mongo connection error: %v\n", err)
		os.Exit(1)
	}

	logger.WithField("event", "mongo_connect").Info("connected to mongo")

	indexCtx, cancelIndexes := context.WithTimeout(context.Background(), mongoIndexTimeout)
	if err := mongoManager.EnsureBaseIndexes(indexCtx); err != nil {
		cancelIndexes()
		logger.WithError(err).Error("mongo index setup error")
		fmt.Fprintf(os.Stderr, "mongo index setup error: %v\n", err)
		os.Exit(1)
	}
	cancelIndexes()

	logger.WithField("event", "mongo_indexes").Info("ensured base mongo indexes")

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), mongoDisconnectTimeout)
		defer cancel()

		if err := mongoManager.Close(shutdownCtx); err != nil {
			logger.WithError(err).Error("mongo disconnect error")
			return
		}

		logger.WithField("event", "mongo_disconnect").Info("mongo client disconnected")
	}()
}
