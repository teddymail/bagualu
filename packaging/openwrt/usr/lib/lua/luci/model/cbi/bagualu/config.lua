local m, s, o, control

m = Map("bagualu", translate("八卦炉配置"))
m.description = translate("这里只保留服务开关、管理端口和后台密码重置；Mihomo、测速、WAN、目录和内核参数使用内置默认值。")

s = m:section(TypedSection, "bagualu", translate("八卦炉基础设置"))
s.anonymous = true
s.addremove = false

o = s:option(Flag, "enabled", translate("启用八卦炉服务"))
o.default = "1"
o.description = translate("控制八卦炉是否随系统运行。")

o = s:option(Value, "listen", translate("管理监听地址"))
o.datatype = "ipaddr"
o.default = "0.0.0.0"

o = s:option(Value, "port", translate("八卦炉管理端口"))
o.datatype = "port"
o.default = "18787"
o.description = translate("管理后台监听端口，修改后保存将自动重载八卦炉。")

o = s:option(Value, "admin_password", translate("重置八卦炉后台密码"))
o.password = true
o.rmempty = true
o.datatype = "minlength(8)"
o.description = translate("填写后保存即可直接覆盖后台密码，不需要旧密码；重置完成后该值会自动清空。")

control = m:section(SimpleSection, translate("服务状态与控制"))
control.template = "bagualu/control"

function m.on_after_commit(self)
  local uci = require("uci").cursor()
  local util = require("luci.util")
  local password = uci:get("bagualu", "main", "admin_password")
  if password and password ~= "" then
    local command = "printf '%s' " .. util.shellquote(password) .. " | /usr/bin/bagualu -config /etc/config/bagualu -reset-password-stdin >/tmp/bagualu-password-reset.log 2>&1"
    local exit_code = luci.sys.call(command)
    uci:delete("bagualu", "main", "admin_password")
    uci:commit("bagualu")
    if exit_code ~= 0 then
      self.message = translate("八卦炉密码重置失败，请检查 /tmp/bagualu-password-reset.log。")
    end
  end
  require("luci.sys").call("/etc/init.d/bagualu reload >/dev/null 2>&1")
end

return m
