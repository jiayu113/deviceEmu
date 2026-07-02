package device

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jiayu113/deviceemu/internal/config"
)

var allFaults = []FaultKind{FaultCPUSpike, FaultSIPDrop, FaultTelemetryStall, FaultCallFail, FaultLatency}

// BuildFleetConfigs 把 base 配置按 fleet 段展开成 N 份
func BuildFleetConfigs(base *config.Config) []*config.Config {
	n := base.Fleet.Count
	if n <= 1 {
		return []*config.Config{base}
	}
	out := make([]*config.Config, 0, n)
	for i := 0; i < n; i++ {
		cp := *base
		ext := base.Fleet.SIPExtStart + i
		cp.Device.ID = fmt.Sprintf("%s%d", base.Fleet.IDPrefix, ext)
		cp.SIP.Username = fmt.Sprintf("%d", ext)
		cp.SIP.LocalPort = base.Fleet.SIPPortStart + i
		cp.SIP.RTPPort = base.Fleet.RTPPortStart + i*2
		out = append(out, &cp)
	}
	return out
}

// Fleet 持有 N 个设备,统一生命周期
type Fleet struct {
	devices []*Device
}

func NewFleet(cfgs []*config.Config) (*Fleet, error) {
	f := &Fleet{}
	for _, c := range cfgs {
		dev, err := New(c)
		if err != nil {
			return nil, fmt.Errorf("build device %s: %w", c.Device.ID, err)
		}
		f.devices = append(f.devices, dev)
	}
	return f, nil
}

func (f *Fleet) Start(ctx context.Context) error {
	for i, d := range f.devices {
		if err := d.Start(ctx); err != nil {
			return fmt.Errorf("start device %s: %w", d.id, err)
		}
		if i > 0 {
			time.Sleep(20 * time.Millisecond) // 简单错峰,避免全部挤在同一瞬间
		}
	}
	log.Printf("[fleet] %d devices started", len(f.devices))
	return nil
}

func (f *Fleet) Stop() {
	var wg sync.WaitGroup
	for _, d := range f.devices {
		wg.Add(1)
		go func(dev *Device) {
			defer wg.Done()
			dev.Stop()
		}(d)
	}
	wg.Wait()
}

// StartChaos 周期随机注入,直到 ctx 取消。Enabled=false 时直接返回。
func (f *Fleet) StartChaos(ctx context.Context, cfg config.ChaosConfig) {
	if !cfg.Enabled || len(f.devices) == 0 {
		return
	}
	iv := time.Duration(cfg.IntervalSecond) * time.Second
	if iv <= 0 {
		iv = 10 * time.Second
	}
	fd := time.Duration(cfg.FaultSeconds) * time.Second
	if fd <= 0 {
		fd = 20 * time.Second
	}
	go func() {
		t := time.NewTicker(iv)
		defer t.Stop()
		log.Printf("[chaos] enabled: every %s inject random fault for %s", iv, fd)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				dev := f.devices[rand.IntN(len(f.devices))]
				k := allFaults[rand.IntN(len(allFaults))]
				dev.injectFault(k, fd)
				log.Printf("[chaos] injected %s into %s", k, dev.id)
			}
		}
	}()
}
