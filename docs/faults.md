# DeviceEmu 故障注入

故障经 `simulate_fault` 命令注入,带时限,到期自动恢复。一个设备可同时叠加多种。

| kind | duration 默认 | 作用 | 观测 |
|---|---|---|---|
| cpu_spike | 30s | publish 时 cpu=95~100 | telemetry.metrics.cpu;devices_faulty |
| sip_drop | 30s | publish 时 sip_registered=false | telemetry.sip_registered;devices_faulty |
| telemetry_stall | 30s | 跳过本次 publish | 遥测速率为 0;devices_faulty |
| call_fail | 30s | doCall 直接失败不发起 | calls_total{fail};devices_faulty |
| latency | 30s | handleCommand 前 sleep ~800ms | command_handle_seconds p95 |

命令:`{"request_id":"r1","action":"simulate_fault","fault":"sip_drop","duration_seconds":20}`
fault 省略 → 默认 cpu_spike(向后兼容旧命令)。