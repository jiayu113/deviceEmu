package device

import (
	"sync"
	"time"
)

// FaultKind 是可注入的故障类型
type FaultKind string

const (
	// CPU使用率突然飙高
	FaultCPUSpike FaultKind = "cpu_spike"
	// 设备掉线
	FaultSIPDrop FaultKind = "sip_drop"
	// 上报遥测失败
	FaultTelemetryStall FaultKind = "telemetry_stall"
	// 打电话失败
	FaultCallFail FaultKind = "call_fail"
	// 网络延迟
	FaultLatency FaultKind = "latency"
)

func ValidFault(k FaultKind) bool {
	switch k {
	case FaultCPUSpike, FaultSIPDrop, FaultTelemetryStall, FaultCallFail, FaultLatency:
		return true
	}
	return false
}

// faultState 管理单设备当前激活的故障(kind -> 过期时刻),并发安全
type faultState struct {
	mu     sync.Mutex
	active map[FaultKind]time.Time
}

func newFaultState() *faultState {
	return &faultState{active: make(map[FaultKind]time.Time)}
}

// inject 注入/续期一个故障;返回 newly=true 表示这是"从无到有"的激活
func (f *faultState) inject(k FaultKind, d time.Duration) (newly bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	exp, ok := f.active[k]
	wasActive := ok && time.Now().Before(exp)
	f.active[k] = time.Now().Add(d)
	return !wasActive
}

// has 查询故障当前是否激活(只读)
func (f *faultState) has(k FaultKind) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	exp, ok := f.active[k]
	return ok && time.Now().Before(exp)
}

// sweep 删除所有已过期故障,返回刚过期的种类
func (f *faultState) sweep() []FaultKind {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var expired []FaultKind
	for k, exp := range f.active {
		if now.After(exp) {
			delete(f.active, k)
			expired = append(expired, k)
		}
	}
	return expired
}
