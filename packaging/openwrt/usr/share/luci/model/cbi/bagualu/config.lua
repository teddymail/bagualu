local m, s, o

m = Map("bagualu", translate("八卦炉配置"))
m.description = translate("系统设置保存在 UCI 中，并由八卦炉 procd 服务应用。")

s = m:section(TypedSection, "bagualu", translate("Service and Mihomo"))
s.anonymous = true
s.addremove = false

o = s:option(Flag, "enabled", translate("Enable service"))
o.default = "1"

o = s:option(Value, "listen", translate("Management listen address"))
o.datatype = "ipaddr"
o.default = "127.0.0.1"

o = s:option(Value, "port", translate("Management port"))
o.datatype = "port"
o.default = "8787"

o = s:option(Value, "data_dir", translate("Data directory"))
o.default = "/var/lib/bagualu"

o = s:option(Value, "status_file", translate("Runtime status file"))
o.default = "/var/run/bagualu/status.json"
o.description = translate("Used by LuCI to show diagnostics even when the management HTTP port is unavailable.")

o = s:option(Value, "mihomo_control_port", translate("Mihomo control port"))
o.datatype = "port"
o.default = "9090"

o = s:option(Value, "mihomo_proxy_port", translate("Mihomo proxy port"))
o.datatype = "port"
o.default = "7890"

o = s:option(Value, "mihomo_binary", translate("Mihomo binary"))
o.default = "/usr/bin/mihomo"

o = s:option(Value, "mihomo_token", translate("Mihomo control token"))
o.password = true

o = s:option(Value, "mihomo_selector", translate("Mihomo test selector"))
o.default = "八卦炉-Test"

o = s:option(Value, "admin_password", translate("Initial admin password"))
o.password = true
o.description = translate("Used only when the admin password has not been initialized in the data directory.")

o = s:option(Value, "test_queue_limit", translate("Test queue limit"))
o.datatype = "uinteger"
o.default = "32"

o = s:option(Value, "interval_seconds", translate("Ping interval (seconds)"))
o.datatype = "uinteger"
o.default = "60"

o = s:option(Flag, "throughput_enabled", translate("Enable daily download tests"))
o.default = "1"
o.description = translate("Each active node is tested once per day through the selected Mihomo route.")

o = s:option(Value, "throughput_url", translate("Download test URL"))
o.default = "https://speed.cloudflare.com/__down?bytes=1048576"

o = s:option(Value, "throughput_urls", translate("Download test URLs"))
o.description = translate("Optional comma-separated sources. One stable source is selected for each daily batch.")

o = s:option(Value, "throughput_bytes", translate("Per-node download bytes"))
o.datatype = "uinteger"
o.default = "1048576"
o.description = translate("The download size used for each daily node test.")

o = s:option(Value, "throughput_windows", translate("Daily test time windows"))
o.default = "02:00-06:00"
o.description = translate("Comma-separated local time windows, for example 02:00-06:00. Leave empty to allow all day.")

o = s:option(Value, "wan_download_bps", translate("WAN download capacity (B/s)"))
o.datatype = "uinteger"
o.default = "0"

o = s:option(Value, "wan_upload_bps", translate("WAN upload capacity (B/s)"))
o.datatype = "uinteger"
o.default = "0"

o = s:option(Value, "load_threshold", translate("WAN load threshold"))
o.datatype = "ufloat"
o.default = "0.1"

function m.on_after_commit(self)
  local conn = require("ubus").connect()
  if conn then
    conn:call("service", "set", {name = "bagualu", action = "reload"})
    conn:close()
  end
end

return m
