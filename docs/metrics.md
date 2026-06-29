# DeviceEmu 指标设计

## 设计决策:Prometheus 放

- **运维/操作指标 + 运行时**:在线设备数、SIP 注册数、各类故障设备数、遥测发布速率/错误、命令速率(按动作/结果)、呼叫成功率、命令处理延迟、呼叫时长、以及 Go 运行时(goroutine 数 / GC / 内存)。

## 指标清单

| 指标 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `deviceemu_devices_online` | Gauge | — | 当前在线设备数(进程自报)|
| `deviceemu_sip_registered_devices` | Gauge | — | 当前 SIP 注册成功设备数 |
| `deviceemu_devices_faulty` | Gauge | kind | 当前各类故障态设备数 |
| `deviceemu_telemetry_published_total` | Counter | — | 遥测成功发布累计 |
| `deviceemu_telemetry_publish_errors_total` | Counter | — | 遥测发布失败累计 |
| `deviceemu_commands_received_total` | Counter | action,result | 命令累计(ok/err)|
| `deviceemu_calls_total` | Counter | result | 呼叫累计(ok/fail)|
| `deviceemu_sip_register_attempts_total` | Counter | — | 注册尝试(含续订/重试)|
| `deviceemu_sip_register_failures_total` | Counter | — | 注册失败 |
| `deviceemu_mqtt_reconnects_total` | Counter | — | MQTT 重连 |
| `deviceemu_faults_injected_total` | Counter | kind | 故障注入累计 |
| `deviceemu_command_handle_seconds` | Histogram | action | 命令处理耗时 |
| `deviceemu_call_duration_seconds` | Histogram | — | 呼叫时长 |
| `go_goroutines` `process_*` | (运行时) | — | 由 Go/Process 采集器自动提供 |