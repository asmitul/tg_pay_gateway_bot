package main

import (
	"flag"
	"fmt"
	"os"

	"tg_pay_gateway_bot/internal/config"
	"tg_pay_gateway_bot/internal/logging"
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
}
