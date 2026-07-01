# DeviceEmu

分布式 IoT 终端仿真平台(Go)。单进程并发模拟多个**同时具备 MQTT 控制面 + SIP 信令面**的智能终端,参考某商业 IoT 紧急通信场景,用于平台联调、压测与可观测性验证。

每个被仿真的设备都是一个完整的状态机:连 MQTT broker 接收下行命令、周期上报结构化健康遥测、向 FreeSWITCH 注册 SIP 并发起/挂断呼叫,并能被远程或由 chaos 引擎注入故障。设备层不感知底层协议,transport 层(MQTT / SIP)在它下面屏蔽实现细节。进程自身暴露 Prometheus 指标,配套 Grafana 看板做 fleet 级观测。

```
              ┌──────────────── 单进程 ────────────────┐
   平台 ──────┤   Fleet → Device × N                   │
 (MQTT broker)│     ├─ MQTT Client  (控制面/遥测)       ├──── FreeSWITCH
   FreeSWITCH ┤     └─ SIP  Client  (REGISTER/呼叫)     │     (SIP 信令)
              │   Chaos 引擎 → 周期随机注入故障           │
              │   /metrics (:2112) ──── Prometheus 抓取  │
              └──────────────────┬─────────────────────┘
                                 │ scrape
                          Prometheus ──── Grafana(fleet 看板)
```

## 能力(v3:可观测 + 异常注入 + 百级 soak)

### 设备 fleet(批量)
单进程并发起 N 个虚拟设备,各自独立 SIP 注册保活 + MQTT 会话。`BuildFleetConfigs` 按基础配置展开:递增分机号与本地 UDP 端口,保证互不冲突;`Stop` 时并发优雅停机(WaitGroup)。已在本机验证 100 台规模稳定运行(见 [`docs/soak-report.md`](docs/soak-report.md))。

### 控制面(MQTT)
- 设备生命周期:连接(auto-reconnect + connect-retry)→ 宣告 `online` → 订阅命令 → 周期遥测 → 优雅停机宣告 `offline`
- **LWT 掉线感知**:进程异常退出时 broker 替设备发遗嘱(retained,`reason: lwt`);主动停机则设备自发 `offline`(`reason: shutdown`)覆盖遗嘱
- 命令回执:每条命令(含被拒绝的)都回一条 `ack`,`request_id` 原样回带供平台配对

### 信令面(SIP)
- **REGISTER + Digest 认证**:首发 → 收 401 challenge → 重算 digest 重发 → 200。From 域显式钉死到 FreeSWITCH 认的域,规避 `realm=localhost → 403`
- **注册保活**(`RegisterLoop`):首注册失败按指数退避 + ±20% 抖动重试,单设备失败不拖垮进程;成功后在 `Expires` 过期前自动续订(re-REGISTER)。抖动把大规模并发下的重试/续订时刻打散,避免惊群
- **完整呼叫**:`INVITE → 等 2xx(处理 100/180,遇 407 自动带认证)→ ACK → 保持 → BYE`;`hangup` 经 context 取消提前发 BYE
- **NOTIFY 应答**:对 FreeSWITCH 注册后推送的 NOTIFY 回 200 OK,避免沉默触发 UDP 重传与日志噪音

### 可观测(Prometheus + Grafana)
- 进程级 `/metrics`(默认 `:2112`):暴露 fleet 运维指标(在线设备数、SIP 注册数、遥测发布量、命令处理耗时直方图、故障设备数按类型)+ Go 运行时指标(goroutine 数等)
- Prometheus + Grafana 监控栈随 `docker compose` 一键起,数据源与看板均由 provisioning 声明式配置,无需手动点
- Grafana 看板覆盖:在线/注册爬坡、遥测速率、故障注入与恢复、goroutine 平稳性

### 异常注入(chaos)
- **五类有时限故障**,到点自动恢复:
  | 故障类型 | 效果 |
  |---|---|
  | `cpu_spike` | CPU 遥测值飙到 95~100 |
  | `sip_drop` | `sip_registered` 强制 false |
  | `telemetry_stall` | 暂停遥测上报 |
  | `call_fail` | 呼叫直接失败,不真发起 |
  | `latency` | 命令处理注入 800ms 延迟 |
- **两种注入入口**:平台经 MQTT 下发 `simulate_fault` 命令(走完整命令链路,计入命令处理耗时);或 fleet chaos 模式周期性随机注入(内部直调,专注验证故障对遥测/可观测性的影响)
- 故障状态由 `deviceemu_devices_faulty` 按类型导出,Grafana 上可见"注入 → 起伏 → 到期归零"的完整曲线

### 远程运维命令(MQTT 下发,五选一)
| action | 行为 |
|---|---|
| `call` | 异步发起一次 SIP 呼叫(target 留空用配置默认被叫) |
| `hangup` | 取消当前通话 → 提前 BYE |
| `report_now` | 触发一次立即遥测上报 |
| `set_telemetry_interval` | 动态重置遥测上报间隔 |
| `simulate_fault` | 注入上述五类故障之一,到点自动恢复 |

### 健康遥测
结构化上报:`seq`(自增,可检测丢包/乱序)、`uptime`、`sip_registered`、`call_count`、`last_call_at`、固件版本,以及 CPU / 内存 / 信号三项模拟指标(有界随机游走,供平台做趋势与异常检测)。

## 工程设计要点

这几处是这个项目的核心,也是它和"调几个库串起来"的区别:

- **并发安全的运行时状态**:遥测 goroutine 与命令 goroutine 并发访问的字段(`call_count` / `last_call_at` / `faulty` / SIP 注册态)全部用 `atomic`,正在进行的通话用 `mutex` 保护可取消句柄
- **绝不阻塞 MQTT 回调**:`report_now` / `set_telemetry_interval` 经「缓冲 1 + 非阻塞发送」的 channel 投递给遥测循环——MQTT 回调线程永远不会被业务逻辑卡住(代价:极端并发下同类请求可能被合并,ack 因此是"已受理"而非"已执行"语义,详见契约文档)
- **SIP 三层关闭**:server / client / UA 逐层释放,避免监听端口与 goroutine 泄漏
- **统一 context 生命周期**:`signal.NotifyContext` 接管 Ctrl-C,一处取消、全链路退出
- **可复现环境(IaC)**:FreeSWITCH 分机批量在镜像构建期烤入(`gen-users.sh`,不可变、可复现);Prometheus / Grafana 配置随 compose 声明式加载

## 快速开始(Docker + Go 1.26+)

```bash
# 1. 起依赖(EMQX + FreeSWITCH + Prometheus + Grafana)
make up

# 2. 准备配置并填本地值(broker 地址、SIP 分机/密码等)
cp configs/config.example.yaml configs/config.yaml

# 3. 运行
make run

# 其他:make build / make test / make vet / make down / make clean
```

> FreeSWITCH 首次启动较慢,等 1~2 分钟再发呼叫类命令。
>
> 本项目用 `deploy/freeswitch/Dockerfile` 在 `safarov/freeswitch` 基础上通过 `gen-users.sh` 把分机批量烤到 1000–1119(120 个,可改区间参数)。
>
> **SIP 密码**:该镜像的默认密码每次容器创建时随机生成,不是固定的 `1234`。以容器内 `vars.xml` 实查为准:
> ```bash
> docker exec deviceemu-fs grep default_password /etc/freeswitch/vars.xml
> ```

## 配置

单设备模式只需填 `device` / `mqtt` / `sip` 三段。批量模式追加顶层 `fleet` 段,可选 `chaos` 段:

```yaml
fleet:
  count: 100           # >1 进入批量模式;<=1 或省略 = 单设备
  id_prefix: "device-"
  sip_ext_start: 1000  # 分机号从此递增(须落在 FreeSWITCH 镜像烤入的分机区间内)
  sip_port_start: 5066 # 本地 UDP 端口从此递增

chaos:
  enabled: true        # 开启周期随机故障注入
  interval_second: 8   # 每隔多久注入一次
  fault_seconds: 20    # 单次故障持续时长
```

## MQTT 契约

主题、载荷格式、ack 语义(尤其 `call` 的"受理 ≠ 接通"不对称设计)详见 [`docs/mqtt-contract.md`](docs/mqtt-contract.md)。

核心主题:

| 主题 | 方向 | QoS | retained |
|---|---|---|---|
| `devices/{id}/cmd` | 平台→设备 | 1 | 否 |
| `devices/{id}/cmd/ack` | 设备→平台 | 1 | 否 |
| `devices/{id}/telemetry` | 设备→平台 | 1 | 否 |
| `devices/{id}/status` | 设备→平台 | 1 | 是 |

## 测试

```bash
make test
```

- **单元测试**:命令解析(合法/非法 JSON、缺 action)、fleet 配置展开(ID/端口唯一性)、退避时机与注册续订时机
- **协议联调**:依赖真实 EMQX + FreeSWITCH,目前按手动流程验证(订阅 `devices/+/...`、下发各类命令、断电验证 LWT、模拟注册失败验证退避续订),尚无自动化端到端测试
- **soak 压测**:100 台规模、约 90min、chaos 全程开启,验证注册稳定性、遥测吞吐、goroutine 无泄漏、故障注入与恢复,完整记录见 [`docs/soak-report.md`](docs/soak-report.md)

## 局限 / 扩展边界

- **规模天花板**:MQTT 侧(EMQX)可继续上量;SIP 侧受 FreeSWITCH 分机数、本地 UDP 端口范围、单机软交换容量限制——这是真实 IoT 紧急通信场景里"信令面重、数据面轻"的固有差异,不是实现缺陷。
- `call` 命令的 ack 为"已受理发起"语义,呼叫本身的失败/超时只写日志、不补发第二条 ack(见契约文档"`ok` 的语义")

## Roadmap

- **设备管理 / 遥测平台**:消费本契约,做 fleet 管理、健康看板与异常检测——DeviceEmu 作为模拟设备层,与之配套构成"造设备 + 管设备"的闭环

## 技术栈

Go · [paho.mqtt.golang](https://github.com/eclipse/paho.mqtt.golang) · [emiago/sipgo](https://github.com/emiago/sipgo) · [icholy/digest](https://github.com/icholy/digest) · EMQX · FreeSWITCH · Prometheus · Grafana · Docker