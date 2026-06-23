package main

import (
	"context"
	"log"

	"github.com/emiago/sipgo/sip"
	"github.com/jiayu113/deviceemu/internal/config"
	mysip "github.com/jiayu113/deviceemu/internal/transport/sip"
)

func main() {

	sip.SIPDebug = true

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	c, err := mysip.New(mysip.Config{
		Server: cfg.SIP.Server, Username: cfg.SIP.Username, Password: cfg.SIP.Password,
		Domain: cfg.SIP.Domain, LocalHost: cfg.SIP.LocalHost, LocalPort: cfg.SIP.LocalPort,
		Expiry: cfg.SIP.RegisterExpirySeconds,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	if err := c.Register(context.Background()); err != nil {
		log.Fatal(err)
	}
	log.Println("register OK, sleeping to keep registration; Ctrl-C to exit")
	select {}
}
