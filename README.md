# 八卦炉

八卦炉是一个运行在 OpenWrt 上的节点资源管理、可用性检测和统一订阅服务。

它解决的不是“再启动一个代理内核”，而是机场节点管理中的几个长期痛点：

- 机场提供的节点数量很多，但节点经常过期、失联、认证失败或速度骤降。
- 多个机场的节点分散在不同订阅里，无法统一筛选、测速、评分和调度。
- 订阅刷新后节点会变化，手动维护的节点却不应该被订阅刷新覆盖。
- 仅看一次 Ping 不能代表真实下载能力；节点需要持续、按计划进行下载测速。
- 不同客户端需要不同订阅格式，用户不应该为 Clash、sing-box、v2rayNG 等客户端分别维护节点列表。
- OpenClash、Mihomo、PassWall 等软件可能已经占用常见端口，新服务不应影响现有代理服务。

八卦炉将多个上游订阅和手动节点汇集到一个受控资源池，按节点状态、协议、地区、评分和出口 IP 进行筛选，并生成适配不同客户端的订阅地址。

## 能做什么

### 统一管理多机场资源

在“上游订阅”中添加多个机场订阅，八卦炉会分别记录来源并刷新节点。订阅节点的生命周期由订阅内容决定：

- 订阅刷新后新增节点会自动进入资源池。
- 订阅中消失的节点会从订阅资源中移除。
- 消失节点对应的测速日志会同时清理，避免历史数据长期堆积。
- 订阅节点的测速历史最多保留 7 天。
- 某一个订阅刷新失败时，保留该订阅上一次成功的节点，不因临时网络错误清空资源。

### 订阅节点和手动节点分离

手动添加的节点与订阅节点使用不同的来源标识：

- 手动节点可以单独新增、修改、删除和测速。
- 手动节点不会因为任何订阅刷新而消失或被覆盖。
- 订阅节点只由对应订阅的刷新结果维护。
- 同一个地址可以同时存在于不同来源中，便于区分机场资源和自有节点。

手动节点支持完整分享链接导入，例如：

```text
vless://uuid@example.com:443?type=tcp&security=reality&sni=example.com&pbk=public-key&sid=short-id#节点名称
```

也可以直接粘贴多条 URI 或 Base64 订阅内容，由八卦炉自动解析。

### 三类测速

每个节点都可以从“节点”页面查看测速历史，也可以从“测试中心”批量执行：

1. **连通性测试**：确认协议配置、认证信息和 Mihomo 链路是否可用。
2. **Ping 测试**：记录整数毫秒延迟，用于观察线路响应速度。
3. **下载测速**：通过节点实际下载数据，记录整数吞吐速度和完整测速证据。

下载测速按本地自然日调度，每个活跃节点每天执行一次，使用稳定的测速源和受控下载数据量。测速结果会记录测试地址、出口 IP、WAN 背景流量、负载状态、有效下载耗时和 Mihomo 连接证据，避免把路由器自身拥塞误判成节点速度。

基础设施错误单独标记，不会直接污染节点评分；节点评分根据可用性、延迟和吞吐结果综合计算。

### 统一输出给不同客户端

八卦炉为同一组节点提供统一订阅链接，并根据请求参数或客户端 User-Agent 自动选择格式，支持：

- Clash / Mihomo YAML
- sing-box JSON
- Base64 分享链接格式
- dae / daed 配置
- JSON 节点列表
- 原始分享链接输出

Clash Verge、Clash for Android、OpenClash、Nikki 等客户端通常自动获得 Clash/Mihomo 格式；sing-box、dae、v2rayNG、v2rayN、PassWall、Shadowrocket 等客户端会根据 User-Agent 选择合适格式。也可以显式指定格式，例如：

```text
http://路由器地址:18787/api/v1/subscribe/订阅令牌?format=clash
http://路由器地址:18787/api/v1/subscribe/订阅令牌?format=sing-box
http://路由器地址:18787/api/v1/subscribe/订阅令牌?format=base64
http://路由器地址:18787/api/v1/subscribe/订阅令牌?format=dae
```

实际订阅地址以控制台“订阅链接”页面生成的地址为准，不要把管理密码或 API Key 写进订阅 URL。

### 与现有 Mihomo/OpenClash 共存

八卦炉使用独立的受管 Mihomo 实例，不使用系统常见代理端口：

| 用途 | 默认监听 |
| --- | --- |
| 八卦炉管理后台 | `0.0.0.0:18787` |
| 八卦炉受管 Mihomo 控制 API | `127.0.0.1:19090` |
| 八卦炉受管 Mihomo 代理 | `127.0.0.1:17890` |

`19090` 和 `17890` 由八卦炉内部固定管理，不在 LuCI 设置页开放修改，避免误改导致测速和受管内核失效。系统已有的 `9090`、`7890` 等端口不会被八卦炉占用。

## 安装前准备

- OpenWrt 或 iStoreOS，建议使用 24.10 或更新版本。
- 可通过 SSH 登录路由器的 `root` 权限。
- 路由器有足够的磁盘空间保存数据库、节点配置和 7 天测速历史。
- 路由器可以访问 GitHub，或准备在管理后台上传 Mihomo 内核文件。

当前公开 workflow 编译：

- OpenWrt x86_64
- OpenWrt ARM64，安装包架构为 `aarch64_cortex-a53`

MIPS 包暂未列入公开构建矩阵。安装前先确认设备架构：

```sh
ubus call system board
opkg print-architecture
```

## OpenWrt 安装

### 1. 获取 IPK

只有推送形如 `v0.1.5` 的版本 tag 才会触发公开构建 workflow。进入 GitHub 项目的 Releases 页面，下载与设备架构对应的 `bagualu_*.ipk`。

不要下载错误架构的安装包，例如 x86_64 包不能安装到 ARM64 路由器。

### 2. 上传安装包

在电脑终端执行：

```sh
scp bagualu_*.ipk root@192.168.1.1:/tmp/
```

将 `192.168.1.1` 替换为你的路由器地址。

### 3. 安装并启动

```sh
ssh root@192.168.1.1
opkg update
opkg install /tmp/bagualu_*.ipk
/etc/init.d/bagualu enable
/etc/init.d/bagualu start
/etc/init.d/bagualu status
```

安装成功后，LuCI 菜单进入：

```text
服务 → 八卦炉
```

状态页可以查看 Bagualu 和 Mihomo 的运行状态、PID、端口、最近错误，并执行启动、停止、重启、启用和禁用。

### 4. 设置八卦炉后台密码

八卦炉后台密码不是路由器 LuCI 密码。请在 LuCI 的：

```text
服务 → 八卦炉 → Configuration
```

找到“重置八卦炉后台密码”，填写至少 8 位的新密码并保存。该操作会直接覆盖八卦炉数据库中的密码，不需要输入旧密码，也不会跳转到路由器系统密码页面。

首次安装后建议立即重置密码，不要长期使用默认初始化密码。

### 5. 打开管理后台

浏览器访问：

```text
http://路由器地址:18787/
```

例如：

```text
http://192.168.1.1:18787/
```

管理后台采用 Ant Design 明亮模式，主要页面包括：

- 仪表盘：节点总量、可用性和运行概览。
- 上游订阅：添加、刷新和管理多个机场订阅。
- 节点：搜索、筛选、手动导入、编辑、删除和查看测速历史。
- 分组：按用途建立节点集合和选择策略。
- 订阅链接：生成面向不同客户端的统一订阅地址。
- 运行后台：查看实时任务和受管 Mihomo 状态。
- 测试中心：提交连通性、Ping 和下载测速任务。
- 流量报表：查看八卦炉管理的吞吐和测速数据。
- 系统设置：查看只读运行配置和 Mihomo 安装状态。

## Docker 部署

Docker 镜像包含八卦炉和独立的 Mihomo 内核，不需要在宿主机另行安装 Mihomo。容器只对外暴露八卦炉管理端口 `18787`，Mihomo 控制 API 和代理端口仅在容器内部监听，不会占用宿主机已有的 `9090`、`7890` 或其他端口。

### 方式一：Docker Compose

复制 `.env.example` 为 `.env`，再设置唯一的后台密码和 Mihomo 控制密钥：

```sh
cp .env.example .env
```

编辑 `.env`：

```dotenv
BAGUALU_ADMIN_PASSWORD=请替换为至少8位随机密码
BAGUALU_MIHOMO_TOKEN=请替换为随机控制密钥
# 可选：固定 Mihomo 版本；默认使用官方 latest
# MIHOMO_VERSION=vX.Y.Z
```

启动：

```sh
docker compose up -d --build
docker compose ps
docker compose logs -f bagualu
```

浏览器打开 `http://服务器地址:18787/`。数据库、节点、订阅、分组和测速历史保存在 Docker volume `bagualu-data` 中；删除容器不会删除该 volume。

停止或升级：

```sh
docker compose down
docker compose up -d --build
```

### 方式二：直接运行

```sh
docker build --build-arg MIHOMO_VERSION=latest -t bagualu:local .
docker volume create bagualu-data
docker run -d --name bagualu \
  --restart unless-stopped \
  -p 18787:18787 \
  -e BAGUALU_ADMIN_PASSWORD='请替换为至少8位随机密码' \
  -e BAGUALU_MIHOMO_TOKEN='请替换为随机控制密钥' \
  -v bagualu-data:/var/lib/bagualu \
  bagualu:local
```

镜像构建会根据 Docker 目标架构下载官方 Mihomo Linux 发行版，当前支持 `amd64`、`arm64` 和 `386`。如需离线环境部署，先在可联网机器构建镜像，再将镜像导出并复制到目标服务器：

```sh
docker save bagualu:local | gzip > bagualu-docker.tar.gz
docker load < bagualu-docker.tar.gz
```

Docker 部署使用容器自己的网络命名空间。节点下载测速仍然通过容器内八卦炉管理的 Mihomo 代理完成；如果宿主机或上游路由器开启了透明代理，测速基线可能仍受其策略影响，应在网络策略中为 Docker 容器网段保留直连出口。

## 第一次使用

推荐按照以下顺序配置：

1. 在“系统设置”确认 Mihomo 已安装并处于可用状态。
2. 在“上游订阅”添加第一个机场订阅，保存后执行刷新。
3. 在“节点”检查节点来源、协议、地址和当前状态。
4. 对单个节点执行连通性、Ping 或下载测速，打开节点详情查看历史。
5. 在“分组”创建一个资源组，将多个机场节点统一加入。
6. 在“订阅链接”创建一个订阅链接，设置分组、最低评分、健康节点和输出格式。
7. 将生成的订阅 URL 粘贴到 Clash、sing-box、v2rayNG 或其他客户端。
8. 后续由八卦炉负责订阅刷新、节点状态更新、每日下载测速和订阅输出。

手动节点导入时，在“节点”页面选择单节点添加或导入分享链接；这类节点不会被上游订阅刷新删除。

## 任务与测速说明

八卦炉只展示正在处理的任务和最近操作结果，不保留无意义的未知任务。启动时会清理异常退出遗留的活动任务，服务重启后不会继续显示永久 `pending` 任务。

如果任务无法执行，优先检查：

1. LuCI 状态页中的八卦炉服务是否为 `running`。
2. Mihomo 是否已安装、可执行且状态为 `running`。
3. `127.0.0.1:19090` 控制 API 和 `127.0.0.1:17890` 代理是否由八卦炉实例监听。
4. 节点是否已通过连通性测试，节点配置是否包含正确的认证和 Reality/TLS 参数。
5. “运行后台”是否存在正在占用队列的任务。

可以在“测试中心”使用清理任务功能清除已结束或异常遗留的任务；正常情况下不应出现大量未知任务。

## 常见问题

### 页面打不开或访问 503

先通过 SSH 检查：

```sh
/etc/init.d/bagualu status
logread | grep -i bagualu
ss -ltnp | grep -E ':18787|:19090|:17890'
```

如果服务没有运行：

```sh
/etc/init.d/bagualu restart
```

如果只有 Mihomo 不可用，进入 LuCI 的八卦炉状态页或管理后台系统设置，检查 Mihomo 安装状态和错误信息。

### 端口冲突

八卦炉默认不使用系统常见的 `8787`、`9090`、`7890`。如果仍然发现端口冲突，请确认设备上是否残留了旧版本八卦炉进程：

```sh
ps | grep -E 'bagualu|mihomo'
```

停止服务后再启动：

```sh
/etc/init.d/bagualu stop
/etc/init.d/bagualu start
```

### 下载测速失败

下载测速依赖节点协议链路、受管 Mihomo 和外部测速源。单次失败不代表节点一定失效，先查看测速历史中的失败阶段、测试地址、负载状态和 Mihomo 证据。

如果 WAN 本身正在大流量下载，结果可能被标记为基础设施或高负载，不会直接作为节点速度评分。建议在网络空闲时等待每日批次完成。

### 订阅里没有节点

检查订阅刷新是否成功、分组是否包含节点、订阅链接是否设置了过高的最低评分，以及 `healthy_only` 是否过滤了当前异常节点。也可以先在“节点”页面确认节点是否存在，再检查分组成员。

### 修改密码后仍然不能登录

确认修改的是“八卦炉配置”中的后台密码，而不是 LuCI 的系统密码。保存后重启八卦炉：

```sh
/etc/init.d/bagualu restart
```

浏览器重新打开 `http://路由器地址:18787/`，使用新密码登录。

## 升级、备份与卸载

升级前建议备份数据目录：

```sh
tar -czf /tmp/bagualu-backup.tgz /var/lib/bagualu /etc/config/bagualu
```

上传新版 IPK 后执行：

```sh
opkg install /tmp/bagualu_*.ipk
/etc/init.d/bagualu restart
```

升级会保留 UCI 配置、节点、订阅、分组、API Key 和测速历史。卸载：

```sh
/etc/init.d/bagualu disable
/etc/init.d/bagualu stop
opkg remove bagualu
```

如需彻底删除数据，再确认备份完成后删除：

```sh
rm -rf /var/lib/bagualu
```

## 安全建议

- 不要把八卦炉管理端口直接暴露到公网，优先通过内网、VPN 或安全隧道访问。
- 安装后立即重置后台密码，并使用随机、唯一的密码。
- 订阅 URL 包含访问令牌，应当像密码一样保存，不要公开发布。
- 不要在日志、截图或工单中暴露 VLESS、VMess、Trojan、SS 等节点的完整分享链接。
- 为不同使用场景创建不同订阅链接，泄露时可以单独轮换令牌。
- 八卦炉只负责管理它自己的 Mihomo 实例，不会替换或修改系统已有的 OpenClash/Mihomo 配置。

## 开发与构建

本地检查：

```sh
go test ./...
go vet ./...
./scripts/verify-sdd.sh
npm run build --prefix web/admin
```

OpenWrt IPK workflow 只响应版本 tag：

```sh
git tag -a v0.1.5 -m "八卦炉 v0.1.5"
git push origin main
git push origin v0.1.5
```

推送 `main` 不会触发 IPK 编译；只有 `v*` tag 会触发 x86_64 和 ARM64 构建，并在构建成功后创建 GitHub Release。IPK 使用标准 OpenWrt `2.0` 包格式生成，不依赖本机或路由器上的 OpenWrt SDK。

## 项目文档

- 产品与技术要求：`docs/proxy-pool-sdd.md`
- SDD 验收记录：`docs/sdd-acceptance-2026-08-29.md`
- OpenWrt 安装补充：`docs/openwrt-install.md`
- 架构决策记录：`docs/adr/0001-runtime-and-module-boundaries.md`
