# DeviceEmu

分布式 IoT 终端仿真平台(Go)。模拟具备 MQTT 控制面 + SIP 信令面的智能终端,
用于联调与压测,参考某商业 IoT 紧急通信场景。

## 当前能力(v1 地基)
- 单设备完整生命周期:MQTT 连接 / 订阅命令 / 周期遥测 / 优雅退出
- SIP REGISTER(digest 认证)/ INVITE→ACK→BYE 完整呼叫
- MQTT 命令触发 SIP 呼叫(控制面 → 信令面协同)

## 架构
transport 层(mqtt/sip)对 device 层暴露接口,device 不感知底层协议——
便于后续多终端并发与异常注入扩展。

## 快速开始(需 Docker + Go 1.23+)
1. `cp configs/config.example.yaml configs/config.yaml` 并填本地值
2. `docker compose -f deploy/docker-compose.yaml up -d`
3. `go run ./cmd/deviceemu`

## Roadmap
- v2:多终端并发(worker pool 模拟 1000+ 设备)
- v3:Prometheus 指标 + 异常注入
- v4:soak / 压测报告