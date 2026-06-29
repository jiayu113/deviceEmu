package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiayu113/deviceemu/internal/config"
	"github.com/jiayu113/deviceemu/internal/device"
	"github.com/jiayu113/deviceemu/internal/metrics"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	metricsAddr := flag.String("metrics-addr", ":2112", "Prometheus /metrics 监听地址")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 起 /metrics(独立 server,失败不致命,只记日志)
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	srv := &http.Server{Addr: *metricsAddr, Handler: mux}
	go func() {
		log.Printf("metrics on http://%s/metrics", *metricsAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics server: %v", err)
		}
	}()

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
