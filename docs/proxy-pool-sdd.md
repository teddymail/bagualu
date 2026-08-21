# 代理池服务 SDD

状态：Draft

## 1. 文档目的

本文档定义一个代理池服务的第一版技术方案。服务负责：

1. 收集和管理代理订阅。
2. 定时拉取订阅并解析为统一的代理节点。
3. 通过分组管理节点资源。
4. 将客户端 API Key 绑定到分组。
5. 为客户端提供稳定、可鉴权、可过滤、可转换格式的代理资源 API。

本文档用于评审产品边界、核心数据模型和实现顺序。实现前需要根据评审结论补充部署参数和最终接口契约。

## 2. 目标与非目标

### 2.1 目标

- 支持以订阅 URL 作为主要上游来源，也允许管理员录入原始订阅内容用于测试或离线导入。
- 支持常见订阅格式：Base64 编码节点列表、Clash YAML；解析后统一存储。
- 支持节点去重、订阅刷新、解析错误记录和节点状态管理。
- 支持一个节点属于多个分组。
- 支持客户端 API Key 绑定一个默认分组，调用时获取该分组的可用节点。
- 支持按协议、地区、标签、健康状态和延迟筛选节点。
- 支持至少两种输出：统一 JSON 和 Base64 节点订阅；Clash YAML 可作为第二阶段能力。
- 支持管理员查看上游、分组、节点、密钥和刷新任务状态。
- 所有对外资源 API 都具备鉴权、限流和基础审计能力。

### 2.2 非目标

- 第一版不做代理转发，不承担 TCP/UDP 流量转发。
- 第一版不自动购买、探测或生成代理，只处理用户提供的订阅。
- 第一版不承诺绕过上游订阅的访问限制或版权限制。
- 第一版不实现复杂的智能选路、自动测速编排和多级故障转移。
- 第一版不允许客户端修改节点、上游或分组配置。

## 3. 核心概念

| 概念 | 说明 |
| --- | --- |
| 上游订阅 | 管理员配置的来源，可以是 URL 或原始内容。一次刷新会产生一批节点快照。 |
| 节点 | 解析后得到的标准化代理配置，例如 VMess、VLESS、Trojan、SS、HTTP、SOCKS5。 |
| 分组 | 节点的逻辑集合，例如 `hk`、`streaming`、`stable`。一个节点可以属于多个分组。 |
| API Key | 客户端访问资源 API 的凭证。MVP 中一个 Key 绑定一个默认分组。 |
| 节点快照 | 某次上游刷新得到的完整解析结果，用于计算新增、删除和恢复。 |
| 节点状态 | 节点在池内的可用状态，包括 active、disabled、stale、invalid。 |
| 输出订阅 | 将分组中的节点转换成客户端需要的协议或格式。 |

## 4. 用户与权限

### 4.1 管理员

管理员可以：

- 创建、修改、暂停、删除上游订阅。
- 手动触发刷新，查看最近刷新结果和错误。
- 创建、修改、删除分组并维护节点归属。
- 创建、吊销、轮换客户端 API Key。
- 查看节点明细、解析状态、最近健康检查结果。

### 4.2 API 客户端

API 客户端只能：

- 使用 API Key 获取授权分组中的节点。
- 查询自己的 Key 状态和资源摘要。

API Key 不直接暴露数据库中的明文；数据库只保存哈希值，创建或轮换时仅返回一次完整 Key。

## 5. 设计概览

```text
                 +-------------------+
                 | 管理 API / Admin UI|
                 +---------+---------+
                           |
                           v
+----------------+   +-----+------+   +-------------------+
| 上游订阅 URL    |-->| Refresh    |-->| Parser / Normalize |
| 原始订阅内容     |   | Worker     |   +---------+---------+
+----------------+   +------------+             |
                                                  v
                                         +--------+--------+
                                         | Node Store       |
                                         | Group Membership |
                                         +--------+--------+
                                                  |
                                                  v
                                         +--------+--------+
                                         | Resource API     |
                                         | Auth / Filter    |
                                         | Format Converter |
                                         +------------------+
                                                  |
                                                  v
                                      API Key -> Group -> Nodes
```

建议的初始部署形态是一个 API 进程、一个 Worker 进程和一个关系型数据库。API 与 Worker 可以共享同一套代码和数据库，但进程职责分离，避免刷新任务阻塞资源 API。

## 6. 关键业务流程

### 6.1 创建并刷新上游

1. 管理员创建上游，填写名称、订阅 URL、格式提示、刷新周期和请求超时。
2. Worker 根据 `next_refresh_at` 拉取上游，检查 HTTP 状态、响应大小和内容类型。
3. Parser 识别或使用管理员指定的格式，解析为标准节点。
4. Normalize 规范化节点名称、协议字段、地址、端口、TLS 和传输配置。
5. 以标准化指纹去重。
6. 在事务中写入本次快照及节点关联关系。
7. 刷新成功时更新 `last_success_at` 和节点状态；刷新失败时保留上一次成功快照，并记录错误。
8. 新节点只有在被加入分组后，才会出现在对应资源 API 中。

### 6.2 节点归组

MVP 支持两种方式：

- 手动归组：管理员在节点列表中选择节点加入或移出分组。
- 规则归组：分组配置匹配规则，例如协议、地区标签、名称正则。每次刷新后自动重新计算。

手动归组和规则归组需要明确优先级。建议规则归组产生“计算归属”，手动归组产生“强制覆盖”；未配置规则的分组完全由手动维护。

### 6.3 客户端获取资源

1. 客户端通过 `Authorization: Bearer <api-key>` 或 URL 中的 token 调用资源 API。
2. API 校验 Key 哈希、状态、过期时间和调用限额。
3. 解析 Key 绑定的默认分组；如果请求显式指定其他分组，必须验证该 Key 是否拥有该分组权限。
4. 过滤 disabled、invalid、stale 节点，并应用协议、标签、地区和数量限制。
5. 按稳定排序或轮换策略输出结果。
6. 返回 `ETag`、`Last-Modified` 和节点统计，便于客户端缓存和减少重复下载。

### 6.4 健康检查

健康检查是可插拔能力，默认只检查节点最近一次刷新是否有效，不在 MVP 中强制对所有代理发起真实代理连接。

第二阶段可以增加：

- 按分组配置的探测 URL 和超时。
- 受控并发的 TCP/TLS/HTTP 探测。
- 延迟、连续失败次数和冷却时间。
- 按健康状态自动排除节点。

探测必须限制目标地址、并发和请求体，避免服务成为 SSRF 或扫描器。

## 7. 数据模型

以下为逻辑模型，实际字段名可按选定 ORM 调整。

### 7.1 upstreams

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | UUID | 主键 |
| name | varchar | 管理员可读名称 |
| source_type | enum | `url` / `content` |
| source_url | text nullable | 订阅地址，需加密存储或脱敏展示 |
| content_ciphertext | text nullable | 原始内容，可选加密存储 |
| format_hint | enum nullable | `auto` / `base64` / `clash_yaml` / `sing_box_json` |
| refresh_interval_seconds | int | 刷新周期，设置上下限 |
| request_timeout_seconds | int | 拉取超时 |
| enabled | boolean | 是否参与自动刷新 |
| last_success_at | timestamp nullable | 最近成功时间 |
| last_failure_at | timestamp nullable | 最近失败时间 |
| last_error | text nullable | 脱敏后的错误 |
| created_at / updated_at | timestamp | 审计时间 |

### 7.2 upstream_snapshots

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | UUID | 主键 |
| upstream_id | UUID | 上游 |
| fetch_started_at / completed_at | timestamp | 拉取时间 |
| http_status | int nullable | 上游响应码 |
| content_hash | varchar | 原始响应摘要 |
| node_count | int | 解析节点数 |
| error_count | int | 解析错误数 |
| status | enum | `running` / `success` / `failed` |
| error_summary | text nullable | 错误摘要 |

### 7.3 nodes

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | UUID | 主键 |
| fingerprint | varchar unique | 规范化配置的 SHA-256 |
| protocol | enum | `ss` / `vmess` / `vless` / `trojan` / `http` / `socks5` |
| name | varchar | 展示名称 |
| host | varchar | 地址或域名 |
| port | int | 端口 |
| config_ciphertext | json/text | 脱敏后或加密后的完整配置 |
| tags | json | 协议、地区、来源等标签 |
| state | enum | `active` / `disabled` / `stale` / `invalid` |
| last_seen_at | timestamp | 最近在成功快照中出现的时间 |
| last_check_at | timestamp nullable | 最近探测时间 |
| latency_ms | int nullable | 最近延迟 |
| failure_count | int | 连续失败数 |
| created_at / updated_at | timestamp | 时间 |

### 7.4 upstream_node_snapshots

记录节点在哪些上游快照中出现，支持来源追踪和过期判断。

- `snapshot_id`
- `node_id`
- `raw_name`
- `parse_warnings`

主键为 `(snapshot_id, node_id)`。

### 7.5 groups

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | UUID | 主键 |
| name | varchar unique | 分组名称 |
| description | text | 说明 |
| selection_mode | enum | `manual` / `rule` |
| selection_rule | json nullable | 规则归组配置 |
| output_policy | json | 排序、数量、状态过滤策略 |
| enabled | boolean | 是否可被 API 访问 |
| created_at / updated_at | timestamp | 时间 |

### 7.6 group_nodes

- `group_id`
- `node_id`
- `membership_type`: `manual` / `rule` / `override`
- `created_at`

主键为 `(group_id, node_id)`，并为 `group_id`、`node_id` 建索引。

### 7.7 api_keys

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | UUID | 主键 |
| name | varchar | 客户端名称 |
| key_prefix | varchar | 用于管理界面识别，不是完整密钥 |
| key_hash | varchar unique | Argon2id 或 HMAC-SHA-256 摘要 |
| default_group_id | UUID | 默认分组 |
| scopes | json | `resource:read` 等权限 |
| rate_limit | int | 每分钟请求数 |
| expires_at | timestamp nullable | 过期时间 |
| last_used_at | timestamp nullable | 最近使用时间 |
| revoked_at | timestamp nullable | 吊销时间 |
| created_at | timestamp | 创建时间 |

若未来需要一个 Key 访问多个分组，新增 `api_key_groups` 关联表；MVP 先保持一个 Key 一个默认分组，减少权限歧义。

### 7.8 refresh_jobs / audit_logs

`refresh_jobs` 保存调度、重试、耗时、结果和错误；`audit_logs` 保存管理员配置变更和资源 API 的摘要访问记录。日志中不得写入完整 API Key、订阅 URL 查询参数中的敏感 token 或节点密码。

## 8. API 设计

API 前缀统一为 `/api/v1`。管理 API 需要管理员认证，资源 API 使用客户端 API Key。

### 8.1 管理 API

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| POST | `/admin/upstreams` | 创建上游 |
| GET | `/admin/upstreams` | 查询上游及最近刷新状态 |
| PATCH | `/admin/upstreams/{id}` | 修改上游 |
| POST | `/admin/upstreams/{id}/refresh` | 手动触发刷新，返回 job id |
| GET | `/admin/refresh-jobs/{id}` | 查询刷新任务 |
| POST | `/admin/groups` | 创建分组 |
| PATCH | `/admin/groups/{id}` | 修改分组规则和输出策略 |
| GET | `/admin/groups/{id}/nodes` | 查看分组节点 |
| PUT | `/admin/groups/{id}/nodes` | 覆盖或更新手动归组 |
| POST | `/admin/api-keys` | 创建 API Key，仅返回一次明文 |
| POST | `/admin/api-keys/{id}/rotate` | 轮换 Key |
| POST | `/admin/api-keys/{id}/revoke` | 吊销 Key |
| GET | `/admin/nodes` | 查询、筛选和禁用节点 |

### 8.2 资源 API

建议同时提供路径 token 和 Header 两种方式，但文档和默认客户端优先使用 Header，避免 token 出现在代理日志中。

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/resources` | 获取 API Key 默认分组资源 |
| GET | `/resources/{group_name}` | 获取指定分组资源，需拥有该分组权限 |
| GET | `/resources/{group_name}/nodes` | 获取统一 JSON 节点列表 |
| GET | `/resources/{group_name}/subscription` | 获取订阅格式内容 |
| GET | `/resources/{group_name}/stats` | 获取节点数量和健康摘要 |

资源 API 支持查询参数：

- `format`: `json`、`base64`、`clash`。
- `protocol`: 逗号分隔的协议白名单。
- `tag`: 标签过滤，可重复传入。
- `limit`: 返回数量，服务端设置最大值。
- `healthy_only`: 是否只返回健康节点；默认 `true`。
- `sort`: `stable`、`latency`、`random`。

示例：

```http
GET /api/v1/resources/hk/subscription?format=base64&limit=50
Authorization: Bearer pp_live_xxx
```

成功响应应包含：

```json
{
  "group": "hk",
  "format": "base64",
  "node_count": 12,
  "generated_at": "2026-08-21T03:00:00Z",
  "expires_at": "2026-08-21T04:00:00Z",
  "content": "..."
}
```

对订阅格式接口，实际响应可以直接返回对应格式的 body，并通过响应头返回 `ETag`、`X-Node-Count` 和 `X-Resource-Generated-At`。JSON 形式适合管理端和自动化调用；订阅形式适合直接填入客户端。

### 8.3 错误码

| HTTP 状态 | 错误码 | 说明 |
| --- | --- | --- |
| 401 | `invalid_api_key` | Key 不存在、已吊销或已过期 |
| 403 | `group_forbidden` | Key 无权访问指定分组 |
| 404 | `group_not_found` | 分组不存在或未启用 |
| 409 | `refresh_in_progress` | 同一上游已有刷新任务 |
| 422 | `invalid_upstream` | 上游配置无法校验 |
| 429 | `rate_limited` | 超过 API Key 或 IP 限流 |
| 503 | `resource_unavailable` | 分组没有可用节点 |

## 9. 解析与标准化

### 9.1 解析器接口

解析器应使用统一接口，便于新增格式：

```text
Parser.parse(input, format_hint) -> ParseResult
ParseResult.nodes: []RawNode
ParseResult.warnings: []ParseWarning
```

处理顺序：

1. 判断响应是否为 YAML、JSON、纯文本或 Base64。
2. 解码外层内容，限制最大响应体积。
3. 解析节点协议和必要字段。
4. 对非法节点逐条记录错误，不因单个坏节点丢弃整个快照。
5. 标准化字段并生成指纹。

### 9.2 节点指纹

指纹输入应包含影响连接行为的字段，例如：

```text
protocol + host + port + user identity + tls + transport + security + protocol options
```

名称、来源订阅 ID 和展示标签不应影响指纹，否则同一个节点改名后会产生重复记录。指纹使用 SHA-256，数据库建立唯一约束。

### 9.3 过期策略

- 节点在最近一次成功快照中出现：保持 `active`，除非被管理员禁用。
- 节点连续一个刷新周期未出现：标记 `stale`，默认不输出。
- 节点超过可配置保留时间未出现：从分组中移除，可异步归档。
- 节点解析失败：不覆盖旧节点配置；记录警告或错误。

## 10. 刷新任务与并发控制

- 每个上游同一时间最多一个运行中的刷新任务。
- 调度器扫描启用中的上游，按 `next_refresh_at` 创建任务。
- 网络失败和 5xx 使用指数退避，最多重试 3 次；4xx 默认不重试。
- 单次任务设置连接超时、读取超时、最大响应体积和最大节点数。
- 手动刷新与自动刷新使用同一任务队列，避免两套逻辑产生不同结果。
- 快照写入采用事务；只有解析和校验完成后才更新当前有效快照。
- Worker 重启后，超过租约时间的任务可被其他 Worker 接管。

## 11. 安全设计

- 管理 API 使用独立管理员认证，不复用客户端 API Key。
- API Key 使用高熵随机值，响应只返回一次明文；日志、数据库和错误信息全部脱敏。
- 上游 URL 可能包含 token，展示和日志中只保留域名及脱敏后的路径。
- 订阅拉取必须做 SSRF 防护：禁止或限制 loopback、私网、链路本地、云元数据地址；DNS 解析后再次校验目标 IP，并限制重定向。
- 限制上游响应大小、节点数量、单节点字段长度和 YAML/JSON 解析深度。
- 资源 API 按 Key 和 IP 限流，防止订阅被公开转发导致成本和资源泄露。
- 管理员的删除、吊销、修改和手动刷新都写入审计日志。
- 节点密码、UUID、私钥等敏感配置应加密存储；只有在生成授权资源时解密。
- 默认不允许将完整节点配置写入普通应用日志。

## 12. 一致性与缓存

资源 API 读取的是最近一次成功快照及当前分组归属。刷新失败时继续提供旧资源，并在统计接口中暴露数据更新时间和过期状态。

建议按以下维度缓存生成结果：

```text
(group_id, format, filter, sort policy, node state version)
```

分组、节点状态、API 策略或快照变更时递增版本号，使缓存自然失效。短期可使用进程内缓存；多实例部署时改用 Redis 或基于数据库版本的缓存校验。

## 13. 可观测性

至少提供以下指标：

- `upstream_refresh_total{upstream,status}`
- `upstream_refresh_duration_seconds{upstream}`
- `parsed_nodes_total{protocol}`
- `active_nodes_total{group}`
- `resource_requests_total{group,format,status}`
- `resource_request_duration_seconds{group,format}`
- `api_key_rate_limited_total`
- `node_health_check_total{group,status}`

日志使用结构化格式，包含 request ID、job ID、upstream ID、group ID 和耗时，不包含密钥和节点敏感字段。

健康检查接口建议：

- `GET /health/live`：进程存活。
- `GET /health/ready`：数据库和必要队列可用。

## 14. 部署建议

### MVP

- API 服务。
- Worker 服务。
- PostgreSQL 或兼容关系型数据库。
- Redis 可选：用于队列、限流和多实例缓存。
- 反向代理负责 TLS、基础请求体限制和访问日志脱敏。

初始部署可以使用 Docker Compose。生产环境再根据刷新任务规模拆分 Worker、数据库和 Redis。

## 15. 测试策略

### 单元测试

- Base64、Clash YAML、JSON 等解析器的正常和异常输入。
- 节点标准化和指纹稳定性。
- 分组规则匹配、手动覆盖和 stale 逻辑。
- API Key 哈希校验、过期、吊销和分组授权。
- 输出格式转换和过滤参数校验。

### 集成测试

- 上游拉取到快照提交的完整流程。
- 刷新失败不覆盖上一次有效快照。
- 同一上游并发刷新只执行一次。
- Key 只能读取授权分组。
- `ETag` 命中时返回 `304 Not Modified`。

### 安全与压力测试

- SSRF 地址、恶意重定向和 DNS 重绑定场景。
- 超大响应、超多节点、畸形 YAML/JSON。
- API Key 暴力尝试和限流。
- 多 Worker 租约接管和重复任务。
- 资源 API 在大量客户端重复拉取下的缓存命中率。

## 16. 分阶段实施计划

### Phase 0：确认契约

- 确认第一版支持的协议和订阅格式。
- 确认管理员认证方式。
- 确认 API Key 是否只绑定一个分组。
- 确认是否允许原始内容上游，以及敏感配置加密密钥的部署方式。

### Phase 1：核心数据与管理能力

- 建立数据库迁移和基础模型。
- 实现上游、分组、节点和 API Key 管理 API。
- 实现管理员鉴权、审计日志和脱敏。
- 实现手动节点归组。

交付标准：管理员可以创建上游、创建分组、创建 Key，并能查看节点和归属。

### Phase 2：订阅刷新与解析

- 实现刷新任务、租约、重试和调度。
- 实现 Base64 和 Clash YAML 解析器。
- 实现标准化、指纹去重、快照和 stale 策略。
- 增加刷新结果和错误查询。

交付标准：上游刷新成功后，节点可被稳定归档、去重和追踪；失败不会清空旧资源。

### Phase 3：资源 API

- 实现 Key 鉴权和分组授权。
- 实现 JSON、Base64 输出。
- 实现筛选、排序、数量限制、ETag 和限流。
- 增加资源 API 指标和调用审计摘要。

交付标准：客户端仅凭 API Key 即可获取绑定分组的可用订阅资源。

### Phase 4：健康检查与规则分组

- 增加可配置健康检查。
- 增加延迟和失败熔断策略。
- 增加规则归组、手动覆盖和规则预览。
- 增加 Clash YAML 和其他输出格式。

交付标准：分组可以按规则动态维护，资源 API 默认排除连续失败节点。

### Phase 5：生产化

- 多 Worker、Redis 队列和分布式限流。
- 数据库备份、密钥轮换、恢复演练。
- 管理界面、告警和运营报表。
- 灰度发布和回滚方案。

## 17. 关键取舍

### 一个 Key 一个分组，还是一个 Key 多个分组

MVP 采用一个 Key 一个默认分组。它最容易解释、审计和撤销，也避免客户端通过参数猜测其他分组。确认需要跨组订阅后，再增加 `api_key_groups`，不会破坏资源 API 的基本结构。

### 实时拉取，还是预生成资源

采用“定时刷新 + 请求时生成/缓存”。实时拉取上游会让客户端请求受上游可用性影响，也会放大请求量；预生成快照能保证刷新失败时仍有最后可用资源。

### 节点是否跨上游合并

默认按指纹全局合并节点，并通过来源关系追踪多个上游。这样可以减少重复资源；如果未来需要严格隔离来源，可增加按上游维度的分组规则或租户字段。

## 18. 待确认问题

1. 第一版必须支持哪些代理协议和输出格式？是否只需要 Clash 订阅？
2. API Key 是按单个分组绑定，还是一个 Key 需要访问多个分组？
3. 分组主要依赖手动维护、订阅来源，还是需要地区/协议/名称规则？
4. 是否需要真实代理测速？测速目标 URL、频率和最大并发是什么？
5. 是否需要多租户？不同用户之间的上游、节点和 Key 是否完全隔离？
6. 上游订阅是否包含敏感 token，部署环境是否提供 KMS 或固定加密密钥？
7. 资源 API 是否允许通过 URL token 访问？如果允许，应接受其容易进入访问日志的风险。
8. 预期上游数量、节点总量、客户端数量和资源 API QPS 是多少？

## 19. 推荐的首个可验收版本

首个版本建议只实现以下闭环：

```text
创建订阅 URL
  -> 手动/定时刷新
  -> Base64 或 Clash YAML 解析
  -> 标准化去重
  -> 手动加入分组
  -> 创建绑定分组的 API Key
  -> GET /api/v1/resources/{group}/subscription
  -> 返回 Base64 订阅
```

这个闭环先验证核心价值和数据模型。规则分组、真实测速、多租户、更多输出格式和高可用队列都应在闭环稳定后加入，避免第一版同时承担过多变化点。
