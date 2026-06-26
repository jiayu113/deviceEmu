package device

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/jiayu113/deviceemu/internal/config"
)

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
	for _, d := range f.devices {
		if err := d.Start(ctx); err != nil {
			return fmt.Errorf("start device %s: %w", d.id, err)
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
