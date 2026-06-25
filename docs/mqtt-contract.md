# DeviceEmu MQTT 契约

`{id}` = 设备唯一标识。

| 主题 | 方向 | QoS | retained | 用途 |
|---|---|---|---|---|
| `devices/{id}/cmd`       | 平台→设备 | 1 | 否 | 下发命令 |
| `devices/{id}/cmd/ack`   | 设备→平台 | 1 | 否 | 命令回执 |
| `devices/{id}/telemetry` | 设备→平台 | 1 | 否 | 周期健康遥测 |
| `devices/{id}/status`    | 设备→平台 | 1 | **是** | 在线/离线(配 LWT) |

`status` 用 retained:平台一订阅 `devices/+/status` 就能立刻拿到每个设备最新在线态,不必等下一次心跳。

## 载荷

### status
{ "device_id":"device-1001", "status":"online|offline", "reason":"lwt|shutdown", "ts":1719300000 }
offline 两个来源:① 进程异常退出 → broker 替发遗嘱(LWT);② 主动停机 → 设备自己发 reason:"shutdown"。

### telemetry
{ "device_id":"device-1001","ts":..,"seq":42,"uptime_seconds":210,"sip_registered":true,
  "call_count":3,"last_call_at":..,"firmware_version":"deviceemu-1.0.0",
  "metrics":{"cpu":13.5,"mem":39.2,"signal":-64.0} }
metrics 为模拟健康指标(有界随机游走),供平台做趋势/异常检测。

### cmd
{ "request_id":"req-1","action":"call","target":"sip:service@127.0.0.1:5070" }

| action | 字段 | 说明 |
|---|---|---|
| call | target(可空,空则用配置 callee) | 发起一次 SIP 呼叫 |
| hangup | — | 挂断当前通话 |
| report_now | — | 立即上报一次遥测 |
| set_telemetry_interval | interval_seconds | 动态改上报间隔 |
| simulate_fault | duration_seconds(可空,默认30) | 注入亚健康(cpu 飙高+上报失联) |

### cmd/ack
{ "request_id":"req-1","device_id":"device-1001","action":"call","ok":true,"error":"","ts":.. }