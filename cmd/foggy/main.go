package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/evanmusial/foggy/internal/foggy"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := foggy.NewApp(os.DirFS("web/dist"))
	if err != nil {
		log.Fatalf("create app: %v", err)
	}
	if err := app.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
