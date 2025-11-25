package main

import (
	"fmt"
	"os"

	"tg_pay_gateway_bot/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("configuration loaded (env=%s, mongo_db=%s)\n", cfg.AppEnv, cfg.MongoDB)
}
