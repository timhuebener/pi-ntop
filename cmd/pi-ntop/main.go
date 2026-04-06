package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"pi-ntop/internal/app"
	"pi-ntop/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatalf("bootstrap app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			log.Printf("close app: %v", closeErr)
		}
	}()

	log.Printf("%s listening on %s", cfg.AppName, cfg.HTTPAddr)
	if err := application.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
	log.Printf("%s stopped", cfg.AppName)
}
