package main

import (
	"flag"
	"fmt"
	"os"

	"tg_pay_gateway_bot/internal/config"
)

func main() {
	configOnly := flag.Bool("config-only", false, "load and print configuration then exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	if *configOnly {
		fmt.Println("configuration check: ok")
		fmt.Println(config.FormatRedacted(cfg))
		return
	}

	fmt.Printf("configuration loaded (env=%s, mongo_db=%s)\n", cfg.AppEnv, cfg.MongoDB)
}
