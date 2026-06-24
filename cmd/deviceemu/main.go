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

	dev, err := device.New(cfg)
	if err != nil {
		log.Fatalf("new device: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := dev.Start(ctx); err != nil {
		log.Fatalf("start device: %v", err)
	}
	log.Printf("device %s started", cfg.Device.ID)

	// 等待 SIGINT / SIGTERM,优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down...")
	cancel()
	dev.Stop()
	log.Println("bye")
}
