package device

import (
	"fmt"
	"testing"

	"github.com/jiayu113/deviceemu/internal/config"
)

func TestBuildFleetConfigs(t *testing.T) {
	base := &config.Config{}
	base.Fleet = config.FleetConfig{Count: 3, IDPrefix: "device-", SIPExtStart: 1001, SIPPortStart: 5066}
	cfgs := BuildFleetConfigs(base)
	if len(cfgs) != 3 {
		t.Fatalf("want 3, got %d", len(cfgs))
	}
	ports, ids := map[int]bool{}, map[string]bool{}
	for i, c := range cfgs {
		if want := fmt.Sprintf("%d", 1001+i); c.SIP.Username != want {
			t.Errorf("dev %d ext=%s want %s", i, c.SIP.Username, want)
		}
		if c.SIP.LocalPort != 5066+i {
			t.Errorf("dev %d port=%d want %d", i, c.SIP.LocalPort, 5066+i)
		}
		if ports[c.SIP.LocalPort] {
			t.Errorf("dup port %d", c.SIP.LocalPort)
		}
		if ids[c.Device.ID] {
			t.Errorf("dup id %s", c.Device.ID)
		}
		ports[c.SIP.LocalPort], ids[c.Device.ID] = true, true
	}
}

func TestBuildFleetConfigsSingle(t *testing.T) {
	base := &config.Config{}
	base.Device.ID = "device-001"
	cfgs := BuildFleetConfigs(base) // 无 fleet 段
	if len(cfgs) != 1 || cfgs[0].Device.ID != "device-001" {
		t.Fatal("no-fleet should yield single base device")
	}
}
