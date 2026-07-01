package device

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jiayu113/deviceemu/internal/config"
	"github.com/jiayu113/deviceemu/internal/metrics"
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

	startedAt time.Time

	// 运行时状态(遥测 goroutine 与命令 goroutine 并发访问,用原子)
	callCount    atomic.Int64
	lastCallUnix atomic.Int64
	faults       *faultState

	// 与遥测循环通信(缓冲 1 + 非阻塞发送,绝不卡 MQTT 回调)
	reload     chan time.Duration
	publishNow chan struct{}

	// 当前通话的取消句柄(hangup 用)
	mu         sync.Mutex
	cancelCall context.CancelFunc
}

// New 根据 config 构造 Device(此时未连接)
func New(cfg *config.Config) (*Device, error) {
	id := cfg.Device.ID
	interval := time.Duration(cfg.MQTT.TelemetryInterval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}

	willPayload, _ := json.Marshal(statusMsg{DeviceID: id, Status: "offline", Reason: "lwt"})
	m := mqtt.New(mqtt.Options{
		Broker:       cfg.MQTT.Broker,
		ClientID:     id,
		Username:     cfg.MQTT.Username,
		Password:     cfg.MQTT.Password,
		Keepalive:    time.Duration(cfg.MQTT.KeepaliveSeconds) * time.Second,
		WillTopic:    fmt.Sprintf("devices/%s/status", id),
		WillPayload:  willPayload,
		WillQoS:      1,
		WillRetained: true,
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
		id:         id,
		interval:   interval,
		mqtt:       m,
		sip:        s,
		callee:     cfg.SIP.Callee,
		reload:     make(chan time.Duration, 1),
		publishNow: make(chan struct{}, 1),
		faults:     newFaultState(),
	}, nil
}

// Start 拉起设备:连 MQTT、宣告 online、订阅命令、后台注册保活 + 遥测
func (d *Device) Start(ctx context.Context) error {
	d.startedAt = time.Now()
	if err := d.mqtt.Connect(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	d.publishStatus("online", "") // 覆盖遗嘱
	metrics.DevicesOnline.Inc()

	cmdTopic := fmt.Sprintf("devices/%s/cmd", d.id)
	if err := d.mqtt.Subscribe(cmdTopic, 1, d.handleCommand); err != nil {
		return fmt.Errorf("mqtt subscribe: %w", err)
	}

	go d.sip.RegisterLoop(ctx) // 后台注册 + 退避重试 + 续订,不再致命退出
	go d.runTelemetry(ctx)

	log.Printf("[device %s] started, subscribed %s", d.id, cmdTopic)
	return nil
}

// Stop 优雅停机:主动宣告 offline,再断 MQTT / 关 SIP
func (d *Device) Stop() {
	d.publishStatus("offline", "shutdown")
	metrics.DevicesOnline.Dec()
	d.mqtt.Disconnect()
	d.sip.Close()
}

// injectFault 注入有时限故障并维护指标
func (d *Device) injectFault(k FaultKind, dur time.Duration) {
	newly := d.faults.inject(k, dur)
	metrics.FaultsInjected.WithLabelValues(string(k)).Inc() // 每次注入都计
	if newly {
		metrics.DevicesFaulty.WithLabelValues(string(k)).Inc() // 仅"从无到有"时 +1
	}
}

func (d *Device) publishStatus(status, reason string) {
	topic := fmt.Sprintf("devices/%s/status", d.id)
	payload, _ := json.Marshal(statusMsg{DeviceID: d.id, Status: status, Reason: reason, Ts: time.Now().Unix()})
	if err := d.mqtt.Publish(topic, 1, payload, true); err != nil { // retained
		log.Printf("[device %s] publish status: %v", d.id, err)
	}
}

// runTelemetry 周期上报心跳/遥测,直到 ctx 取消
func (d *Device) runTelemetry(ctx context.Context) {
	t := time.NewTicker(d.interval)
	defer t.Stop()
	topic := fmt.Sprintf("devices/%s/telemetry", d.id)

	cpu, mem, signal := 12.0, 38.0, -65.0
	var seq int64
	round1 := func(v float64) float64 { return math.Round(v*10) / 10 }

	publish := func() {
		seq++
		reg := d.sip.Registered()
		c := cpu
		if d.faults.has(FaultCPUSpike) {
			c = 95 + rand.Float64()*5
		}
		if d.faults.has(FaultSIPDrop) {
			reg = false
		}
		tm := Telemetry{
			DeviceID: d.id, Ts: time.Now().Unix(), Seq: seq,
			UptimeSeconds: int64(time.Since(d.startedAt).Seconds()),
			SIPRegistered: reg,
			CallCount:     d.callCount.Load(),
			LastCallAt:    d.lastCallUnix.Load(),
			Firmware:      "deviceemu-1.0.0",
			Metrics:       Metrics{CPU: round1(c), Mem: round1(mem), Signal: round1(signal)},
		}
		payload, _ := json.Marshal(tm)
		if err := d.mqtt.Publish(topic, 1, payload, false); err != nil {
			metrics.TelemetryErrors.Inc()
			log.Printf("[device %s] publish telemetry: %v", d.id, err)
			return
		}
		metrics.TelemetryPublished.Inc()
	}

	walk := func() { // 有界随机游走:在上次值附近小步漂移
		cpu = clamp(cpu+(rand.Float64()*6-3), 1, 100)
		mem = clamp(mem+(rand.Float64()*4-2), 5, 100)
		signal = clamp(signal+(rand.Float64()*4-2), -110, -40)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("[device %s] telemetry loop stopped", d.id)
			return
		case <-t.C:
			for _, k := range d.faults.sweep() {
				metrics.DevicesFaulty.WithLabelValues(string(k)).Dec()
			}
			walk()
			if d.faults.has(FaultTelemetryStall) {
				continue
			}
			publish()
		case <-d.publishNow:
			publish()
		case ni := <-d.reload:
			if ni > 0 {
				t.Reset(ni)
				log.Printf("[device %s] telemetry interval -> %s", d.id, ni)
			}
		}
	}
}

// handleCommand:平台经 MQTT 下发命令,设备据此动作并回 ack
func (d *Device) handleCommand(_ string, payload []byte) {
	start := time.Now()
	cmd, err := parseCommand(payload)

	// 用 defer 统一记录处理耗时
	defer func() {
		action := cmd.Action
		if action == "" {
			action = "unknown" // 保护机制：如果连动作名字都没解出来，不能填空，填 unknown
		}
		// 算从 start 到现在花了几秒，时间分布直方图里
		metrics.CommandLatency.WithLabelValues(action).Observe(time.Since(start).Seconds())
	}()

	if err != nil {
		// JSON 解析失败了，不知道是什么命令
		metrics.CommandsReceived.WithLabelValues("unknown", "err").Inc()
		log.Printf("[device %s] reject cmd: %v: %s", d.id, err, string(payload))
		d.ack(cmd.RequestID, cmd.Action, false, err.Error())
		return
	}

	log.Printf("[device %s] cmd: action=%s", d.id, cmd.Action)

	// Latency (卡顿) 注入
	if d.faults.has(FaultLatency) {
		time.Sleep(800 * time.Millisecond)
	}

	switch cmd.Action {
	case "call":
		target := cmd.Target
		if target == "" {
			target = d.callee
		}
		go d.doCall(target)
		metrics.CommandsReceived.WithLabelValues(cmd.Action, "ok").Inc()
		d.ack(cmd.RequestID, cmd.Action, true, "")

	case "hangup":
		d.mu.Lock()
		cancel := d.cancelCall
		d.mu.Unlock()
		if cancel != nil {
			cancel() // Call 内部 select 命中 ctx.Done() → 提前 BYE
			metrics.CommandsReceived.WithLabelValues(cmd.Action, "ok").Inc()
			d.ack(cmd.RequestID, cmd.Action, true, "")
		} else {
			metrics.CommandsReceived.WithLabelValues(cmd.Action, "err").Inc()
			d.ack(cmd.RequestID, cmd.Action, false, "no active call")
		}

	case "report_now":
		select {
		case d.publishNow <- struct{}{}:
		default:
		}
		metrics.CommandsReceived.WithLabelValues(cmd.Action, "ok").Inc()
		d.ack(cmd.RequestID, cmd.Action, true, "")

	case "set_telemetry_interval":
		if cmd.Interval <= 0 {
			metrics.CommandsReceived.WithLabelValues(cmd.Action, "err").Inc()
			d.ack(cmd.RequestID, cmd.Action, false, "interval_seconds must be > 0")

			return
		}
		select {
		case d.reload <- time.Duration(cmd.Interval) * time.Second:
		default:
		}
		metrics.CommandsReceived.WithLabelValues(cmd.Action, "ok").Inc()
		d.ack(cmd.RequestID, cmd.Action, true, "")

	case "simulate_fault":
		dur := cmd.Duration
		if dur <= 0 {
			dur = 30
		}
		kind := cmd.Fault
		if kind == "" {
			kind = FaultCPUSpike
		}
		if !ValidFault(kind) {
			metrics.CommandsReceived.WithLabelValues("simulate_fault", "err").Inc()
			d.ack(cmd.RequestID, cmd.Action, false, "unknown fault kind: "+string(kind))
			return
		}
		d.injectFault(kind, time.Duration(dur)*time.Second)
		metrics.CommandsReceived.WithLabelValues("simulate_fault", "ok").Inc()
		d.ack(cmd.RequestID, cmd.Action, true, fmt.Sprintf("%s for %ds", kind, dur))

	default:
		log.Printf("[device %s] unknown action: %s", d.id, cmd.Action)
		metrics.CommandsReceived.WithLabelValues(cmd.Action, "err").Inc()
		d.ack(cmd.RequestID, cmd.Action, false, "unknown action")
	}
}

// doCall 发起一次呼叫,登记可取消句柄,完成后更新统计
func (d *Device) doCall(target string) {
	if d.faults.has(FaultCallFail) { // 注入呼叫故障:直接失败,不真发起
		metrics.CallsTotal.WithLabelValues("fail").Inc()
		log.Printf("[device %s] call suppressed by fault injection", d.id)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.cancelCall = cancel
	d.mu.Unlock()
	defer func() {
		cancel()
		d.mu.Lock()
		d.cancelCall = nil
		d.mu.Unlock()
	}()

	start := time.Now()
	if err := d.sip.Call(ctx, target, 5*time.Second); err != nil {
		metrics.CallsTotal.WithLabelValues("fail").Inc()
		log.Printf("[device %s] call failed: %v", d.id, err)
		return
	}
	metrics.CallsTotal.WithLabelValues("ok").Inc()
	metrics.CallDuration.Observe(time.Since(start).Seconds())
	d.callCount.Add(1)
	d.lastCallUnix.Store(time.Now().Unix())
}

func (d *Device) ack(reqID, action string, ok bool, errMsg string) {
	topic := fmt.Sprintf("devices/%s/cmd/ack", d.id)
	payload, _ := json.Marshal(ackMsg{
		RequestID: reqID, DeviceID: d.id, Action: action,
		Ok: ok, Error: errMsg, Ts: time.Now().Unix(),
	})
	if err := d.mqtt.Publish(topic, 1, payload, false); err != nil {
		log.Printf("[device %s] publish ack: %v", d.id, err)
	}
}
