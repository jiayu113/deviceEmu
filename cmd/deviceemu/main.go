package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jiayu113/deviceemu/internal/config"
	"github.com/jiayu113/deviceemu/internal/device"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGALRM)
	defer stop()

	fleet, err := device.NewFleet(device.BuildFleetConfigs(cfg))
	if err != nil {
		log.Fatal(err)
	}
	if err := fleet.Start(ctx); err != nil {
		fleet.Stop()
		log.Fatal(err)
	}

	log.Printf("fleet running; press Ctrl-C to stop")
	<-ctx.Done()
	log.Printf("shutting down...")
	fleet.Stop()
}
