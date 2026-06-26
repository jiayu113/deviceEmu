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

```json
{ "device_id": "device-1001", "status": "online", "reason": "shutdown", "ts": 1719300000 }
```

- `status`:`online` | `offline`
- `reason`:仅 offline 时有意义。`lwt`(broker 替发遗嘱)/ `shutdown`(主动停机)。空时省略。
- offline 两个来源:① 进程异常退出 → broker 替发遗嘱(LWT,`reason:"lwt"`);② 主动停机 → 设备自己发 `reason:"shutdown"`。
- online 由设备连上后主动发出,**覆盖**之前 retained 的遗嘱。

### telemetry

```json
{
  "device_id": "device-1001",
  "ts": 1719300000,
  "seq": 42,
  "uptime_seconds": 210,
  "sip_registered": true,
  "call_count": 3,
  "last_call_at": 1719299880,
  "firmware_version": "deviceemu-1.0.0",
  "metrics": { "cpu": 13.5, "mem": 39.2, "signal": -64.0 }
}
```

- `seq`:进程内自增,从 1 起,可用来检测丢包/乱序。
- `last_call_at`:最近一次成功呼叫的 unix 秒;**从未呼叫过为 `0`**。
- `metrics`:模拟健康指标(有界随机游走,保留 1 位小数),供平台做趋势/异常检测。
- 注入故障期间(见 `simulate_fault`):`cpu` 飙到 95~100、`sip_registered` 被强制报 `false`——遥测**仍正常上报**,只是内容反映亚健康。

---

## cmd —— 平台 → 设备

设备端用**一个扁平结构**解析所有命令,五种 `action` 共享下面这组字段。平台只需填当前 `action` 相关的字段,其余留空即可(多余字段会被忽略)。

### 公共字段

| 字段 | 类型 | 用于哪个 action | 说明 |
|---|---|---|---|
| `request_id`       | string   | 全部 | 平台生成的请求 ID,设备在 ack 里**原样回带**用于配对。建议每条命令唯一 |
| `action`           | string   | 全部 | 命令类型,五选一 |
| `target`           | string   | `call` | SIP 呼叫目标 |
| `interval_seconds` | int(秒)  | `set_telemetry_interval` | 新的遥测上报间隔 |
| `duration_seconds` | int(秒)  | `simulate_fault` | 故障注入持续时长 |

**解析规则**:

- JSON 非法 → 设备回 `ok:false, error:"invalid json"`。
- `action` 缺失或为空字符串 → 回 `ok:false, error:"missing action"`。
- `action` 不在下表 → 回 `ok:false, error:"unknown action"`。

### 五个命令

| action | 入参字段 | 必填 | 默认 | 设备行为 | ack |
|---|---|---|---|---|---|
| `call`                   | `target`           | 否 | 空则用配置里的 `callee` | **异步**发起一次 SIP 呼叫 | **立即** `ok:true`(仅表示已受理并发起,见下方"ack 语义") |
| `hangup`                 | —                  | — | — | 取消当前通话的 ctx → 提前发 BYE | 有通话 `ok:true`;**无通话** `ok:false, error:"no active call"` |
| `report_now`             | —                  | — | — | 触发一次立即遥测上报(非阻塞) | `ok:true` |
| `set_telemetry_interval` | `interval_seconds` | 是 | — | 动态重置遥测 ticker | `>0` → `ok:true`;**`≤0`** → `ok:false, error:"interval_seconds must be > 0"` |
| `simulate_fault`         | `duration_seconds` | 否 | `30` | 注入亚健康:`sip_registered` 置 false + `cpu` 飙到 95~100,到点自动恢复 | `ok:true`,且 `error` 字段回带提示 `"faulty for {N}s"` |

> `report_now` / `set_telemetry_interval` 经一个**缓冲 1 + 非阻塞**的 channel 投递给遥测 goroutine(绝不卡住 MQTT 回调)。极端并发下若上一条还没被消费,新请求可能被合并/丢弃,但 ack 仍回 `ok:true`——ack 表示"已受理",不保证那一拍一定独立触发。

### 命令示例

```json
// call:呼叫指定目标
{ "request_id": "req-1", "action": "call", "target": "sip:service@127.0.0.1:5070" }

// call:target 留空 → 用配置里的 callee
{ "request_id": "req-2", "action": "call" }

// hangup
{ "request_id": "req-3", "action": "hangup" }

// report_now
{ "request_id": "req-4", "action": "report_now" }

// set_telemetry_interval:把上报间隔改成 10 秒
{ "request_id": "req-5", "action": "set_telemetry_interval", "interval_seconds": 10 }

// simulate_fault:注入 60 秒亚健康(留空则默认 30 秒)
{ "request_id": "req-6", "action": "simulate_fault", "duration_seconds": 60 }
```

---

## cmd/ack —— 设备 → 平台

每条命令(含被拒绝的)都会回一条 ack。

```json
{ "request_id": "req-1", "device_id": "device-1001", "action": "call", "ok": true, "ts": 1719300000 }
```

### 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `request_id` | string | 原样回带命令里的 `request_id`,供平台配对。**为空时省略**(omitempty) |
| `device_id`  | string | 哪个设备回的 |
| `action`     | string | 对应命令的 `action`;若命令 JSON 解析失败,可能为空 |
| `ok`         | bool   | **受理结果**,不等于业务最终结果(见下) |
| `error`      | string | 失败原因。**成功时一般为空被省略**(omitempty)。例外:`simulate_fault` 成功时这里回带 `"faulty for {N}s"` 作为提示信息 |
| `ts`         | int64  | 回执时间,unix 秒 |

### ⚠️ `ok` 的语义:受理 ≠ 业务成功

`ok:true` 表示"命令已被设备受理",**不保证业务动作最终成功**,尤其要注意 `call`:

- ack 在 `go doCall(...)` 启动协程后**立即发出**,此刻 INVITE 还没发,对端是否接通无从得知;真实呼叫结果(失败、超时)**只写本地日志,不会再补发第二条 ack**。
- 所以平台**不能**把 `call` 的 `ok:true` 当成"通话已建立",它只是"呼叫请求已受理并异步发起"。
- 对比:`hangup` / `set_telemetry_interval` 的 ack 更接近**结果回执**——能判断当时有没有通话、间隔是否合法,失败会如实回 `ok:false`。这个"异步命令回执 vs 同步命令回执"的不对称是有意设计。

### 各 action 的 ack 速查

| action | 成功 ack | 失败 ack(`error` 文案) |
|---|---|---|
| `call`                   | `ok:true`(已受理发起) | —(请求层不失败;呼叫本身失败不回 ack,只记日志) |
| `hangup`                 | `ok:true` | `ok:false` · `"no active call"` |
| `report_now`             | `ok:true` | — |
| `set_telemetry_interval` | `ok:true` | `ok:false` · `"interval_seconds must be > 0"` |
| `simulate_fault`         | `ok:true` · `error:"faulty for {N}s"` | — |
| (任意/解析层)            | — | `"invalid json"` / `"missing action"` / `"unknown action"` |

> 解析失败时,设备仍会尽力回带能解析到的 `request_id` 和 `action`;若整段 JSON 都解不开,这两个字段为空,平台只能靠 `ok:false` + `error` 判断。