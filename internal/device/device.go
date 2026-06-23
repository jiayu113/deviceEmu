package device

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jiayu113/deviceemu/internal/config"
	"github.com/jiayu113/deviceemu/internal/transport/mqtt"
	"github.com/jiayu113/deviceemu/internal/transport/sip"
)

// Device 是一个被仿真的智能终端,持有底层 transport,统一生命周期
type Device struct {
	id       string
	interval time.Duration
	mqtt     *mqtt.Client
	sip      *sip.Client
	callee   string
}

// New 根据 config 构造 Device
func New(cfg *config.Config) (*Device, error) {
	m := mqtt.New(mqtt.Options{
		Broker:    cfg.MQTT.Broker,
		ClientID:  cfg.Device.ID,
		Username:  cfg.MQTT.Username,
		Password:  cfg.MQTT.Password,
		Keepalive: time.Duration(cfg.MQTT.KeepaliveSeconds) * time.Second,
	})

	s, err := sip.New(sip.Config{
		Server: cfg.SIP.Server, Username: cfg.SIP.Username, Password: cfg.SIP.Password,
		Domain: cfg.SIP.Domain, LocalHost: cfg.SIP.LocalHost, LocalPort: cfg.SIP.LocalPort,
		Expiry: cfg.SIP.RegisterExpirySeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("new sip client: %w", err)
	}
	return &Device{
		id:       cfg.Device.ID,
		interval: time.Duration(cfg.MQTT.TelemetryInterval) * time.Second,
		mqtt:     m,
		sip:      s,
		callee:   cfg.SIP.Callee,
	}, nil
}

// Start 拉起设备:连 MQTT、订阅命令、起遥测循环
func (d *Device) Start(ctx context.Context) error {
	if err := d.mqtt.Connect(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	cmdTopic := fmt.Sprintf("devices/%s/cmd", d.id)
	if err := d.mqtt.Subscribe(cmdTopic, 1, d.handleCommand); err != nil {
		return fmt.Errorf("mqtt subscribe: %w", err)
	}
	if err := d.sip.Register(ctx); err != nil {
		return fmt.Errorf("sip register: %w", err)
	}
	log.Printf("[device %s] subscribed %s", d.id, cmdTopic)
	go d.runTelemetry(ctx)
	return nil
}

// runTelemetry 周期上报心跳/遥测,直到 ctx 取消
func (d *Device) runTelemetry(ctx context.Context) {
	t := time.NewTicker(d.interval)
	defer t.Stop()
	topic := fmt.Sprintf("devices/%s/telemetry", d.id)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[device %s] telemetry loop stopped", d.id)
			return
		case <-t.C:
			payload, _ := json.Marshal(map[string]any{
				"device_id": d.id,
				"ts":        time.Now().Unix(),
				"online":    true,
			})
			if err := d.mqtt.Publish(topic, 1, payload); err != nil {
				log.Printf("[device %s] publish telemetry: %v", d.id, err)
			}
		}
	}
}

// handleCommand 处理平台下发的命令
func (d *Device) handleCommand(_ string, payload []byte) {
	log.Printf("[device %s] cmd: %s", d.id, string(payload))
}

// Stop 优雅停机
func (d *Device) Stop() {
	d.mqtt.Disconnect()
	d.sip.Close()
}
