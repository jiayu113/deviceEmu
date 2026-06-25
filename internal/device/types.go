package device

// Telemetry 是上报给平台的结构化健康遥测
type Telemetry struct {
	DeviceID      string  `json:"device_id"`
	Ts            int64   `json:"ts"`
	Seq           int64   `json:"seq"`
	UptimeSeconds int64   `json:"uptime_seconds"`
	SIPRegistered bool    `json:"sip_registered"`
	CallCount     int64   `json:"call_count"`
	LastCallAt    int64   `json:"last_call_at"`
	Firmware      string  `json:"firmware_version"`
	Metrics       Metrics `json:"metrics"`
}

// Metrics 是上报给平台的模拟健康指标(有界随机游走)
type Metrics struct {
	CPU    float64 `json:"cpu"`
	Mem    float64 `json:"mem"`
	Signal float64 `json:"signal"`
}

// Status 是上报给平台的在线态
type statusMsg struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Ts       int64  `json:"ts"`
}

// ackMsg 是上报给平台的命令回执
type ackMsg struct {
	RequestID string `json:"request_id,omitempty"`
	DeviceID  string `json:"device_id"`
	Action    string `json:"action"`
	Ok        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Ts        int64  `json:"ts"`
}

// clamp 限定范围
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
