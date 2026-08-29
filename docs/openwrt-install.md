# 八卦炉 OpenWrt 安装

向 `main` 推送或手动运行 workflow 会生成各架构 ipk artifact；推送符合 `v<major>.<minor>.<patch>` 格式的 tag（例如 `v0.1.0`）还会按 tag 版本打包，并自动创建 GitHub Release，附带 x86_64、ARM64 和 MIPS24Kc 安装包。先下载与路由器架构匹配的 `bagualu_*.ipk`。Bagualu 包不强制依赖系统 Mihomo，首次安装后可以在管理后台“系统设置”中下载并安装匹配架构的官方 Mihomo 内核。

## 安装

```sh
scp bagualu_*.ipk root@192.168.1.1:/tmp/
ssh root@192.168.1.1
opkg update
opkg install /tmp/bagualu_*.ipk
uci set bagualu.main.mihomo_token='请替换为随机密钥'
uci commit bagualu
/etc/init.d/bagualu enable
/etc/init.d/bagualu start
```

如果设备没有 `/usr/bin/mihomo`，登录管理后台后进入“系统设置”，点击“下载并安装 Mihomo”。安装器会根据 OpenWrt 当前架构选择发行版资产，校验下载内容后原子替换 `mihomo_binary` 指定路径，并重新启动受管内核。

如果设备访问 GitHub 时出现 TLS 或网络限制，在电脑上下载匹配架构的 Mihomo Linux 文件，点击同页“上传 Mihomo 文件”即可；上传文件会经过 ELF 校验后安装，不需要 SSH 手工复制。

也可以在 LuCI 的八卦炉配置中指定 `mihomo_repository` 和 `mihomo_version`，默认使用 `MetaCubeX/mihomo` 的 `latest` 发行版。

LuCI 页面：

- 状态：`服务 → 八卦炉`
- 配置：`服务 → 八卦炉 → Configuration`
- 管理后台：LuCI 状态页中的“管理后台”按钮

## 升级与卸载

```sh
opkg install /tmp/bagualu_*.ipk
/etc/init.d/bagualu restart
opkg remove bagualu
```

升级会保留 `/var/lib/bagualu` 中的数据库和 UCI 配置。卸载前如需保留节点、订阅和测速历史，请先备份该目录；删除配置文件由 OpenWrt 的 `opkg` 策略决定。
