# 八卦炉 OpenWrt 安装

工作流会为每个目标架构生成一个 ipk artifact。先下载与路由器架构匹配的 `bagualu_*.ipk`，并确认设备已经提供匹配架构的 `mihomo` 包。

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
