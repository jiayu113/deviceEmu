package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var Reg = prometheus.NewRegistry()

func init() {
	// 监控 Go 语言引擎自己的心跳
	Reg.MustRegister(collectors.NewGoCollector())
	// 监控程序占用的物理内存、CPU 使用率、打开了多少个文件句柄
	Reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

var f = promauto.With(Reg)

// ---- Counter(只增)----
var (
	TelemetryPublished = f.NewCounter(prometheus.CounterOpts{
		Name: "deviceemu_telemetry_published_total",
		Help: "遥测成功发布累计",
	})
	TelemetryErrors = f.NewCounter(prometheus.CounterOpts{
		Name: "deviceemu_telemetry_publish_errors_total",
		Help: "遥测发布失败累计",
	})
	CommandsReceived = f.NewCounterVec(prometheus.CounterOpts{
		Name: "deviceemu_commands_received_total",
		Help: "命令累计(按动作/结果)",
	}, []string{"action", "result"}) // result = ok | err
	CallsTotal = f.NewCounterVec(prometheus.CounterOpts{
		Name: "deviceemu_calls_total",
		Help: "呼叫累计(按结果)",
	}, []string{"result"}) // result = ok | fail
	SIPRegisterAttempts = f.NewCounter(prometheus.CounterOpts{
		Name: "deviceemu_sip_register_attempts_total",
		Help: "注册尝试累计(含续订/重试)",
	})
	SIPRegisterFailures = f.NewCounter(prometheus.CounterOpts{
		Name: "deviceemu_sip_register_failures_total",
		Help: "注册失败累计",
	})
	MQTTReconnects = f.NewCounter(prometheus.CounterOpts{
		Name: "deviceemu_mqtt_reconnects_total",
		Help: "MQTT 重连累计",
	})
	FaultsInjected = f.NewCounterVec(prometheus.CounterOpts{
		Name: "deviceemu_faults_injected_total",
		Help: "故障注入累计(按类型)",
	}, []string{"kind"})
)

// ---- Gauge(瞬时,fleet 聚合)----
var (
	DevicesOnline = f.NewGauge(prometheus.GaugeOpts{
		Name: "deviceemu_devices_online",
		Help: "当前在线设备数",
	})
	SIPRegistered = f.NewGauge(prometheus.GaugeOpts{
		Name: "deviceemu_sip_registered_devices",
		Help: "当前 SIP 注册成功设备数",
	})
	DevicesFaulty = f.NewGaugeVec(prometheus.GaugeOpts{
		Name: "deviceemu_devices_faulty",
		Help: "当前各类故障态设备数",
	}, []string{"kind"})
)

// ---- Histogram(分布)----
var (
	CommandLatency = f.NewHistogramVec(prometheus.HistogramOpts{
		Name: "deviceemu_command_handle_seconds",
		Help: "命令处理耗时分布",
	}, []string{"action"})
	CallDuration = f.NewHistogram(prometheus.HistogramOpts{
		Name:    "deviceemu_call_duration_seconds",
		Help:    "呼叫时长分布",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30},
	})
)

// Handler 返回 /metrics 的 http.Handler(只暴露自己的 Reg)
func Handler() http.Handler {
	return promhttp.HandlerFor(Reg, promhttp.HandlerOpts{})
}
