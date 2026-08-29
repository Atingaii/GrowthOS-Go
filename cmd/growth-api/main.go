// Command growth-api runs the GrowthOS modular monolith HTTP process.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpserver"
)

// version can be replaced at build time with:
// go build -ldflags "-X main.version=<build-label>" ./cmd/growth-api
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	router := httpapi.NewRouter(httpapi.RouterOptions{
		Version: version,
		Clock:   httpapi.ClockFunc(time.Now),
	})
	server := httpserver.New(router, httpserver.Config{})

	if err := server.Run(ctx); err != nil {
		log.Printf("growth-api stopped with an error: %v", err)
		os.Exit(1)
	}
}
