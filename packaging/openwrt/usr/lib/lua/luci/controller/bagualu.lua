module("luci.controller.bagualu", package.seeall)

local jsonc = require("luci.jsonc")

function index()
  entry({"admin", "services", "bagualu"}, template("bagualu/status"),
    _("八卦炉"), 10).acl_depends = {"luci-app-bagualu"}
  entry({"admin", "services", "bagualu", "config"}, cbi("bagualu/config"),
    _("Configuration"), 20).leaf = true
  entry({"admin", "services", "bagualu", "action"}, call("action")).leaf = true
  entry({"admin", "services", "bagualu", "status"}, call("status")).leaf = true
end

function status()
  local conn = require("ubus").connect()
  if not conn then
    luci.http.status(503, "ubus unavailable")
    return
  end
  local result = conn:call("service", "list", {name = "bagualu"})
  conn:close()
  local service = result and result.bagualu or {}
  local instances = service.instances or {}
  local instance = instances.instance or instances.bagualu
  if not instance then
    for _, candidate in pairs(instances) do
      if type(candidate) == "table" and (candidate.running ~= nil or candidate.pid ~= nil) then
        instance = candidate
        break
      end
    end
  end
  instance = instance or {}
  local running = instance.running == true
  local runtime = read_runtime_status()
  result = result or {}
  if runtime then
    for key, value in pairs(runtime) do result[key] = value end
  end
  local operation = read_operation()
  if operation and operation.status == "submitted" then
    if operation.action == "start" or operation.action == "restart" or operation.action == "enable" then
      if running then operation.status = "succeeded" end
    elseif operation.action == "stop" or operation.action == "disable" then
      if not running then operation.status = "succeeded" end
    end
    if operation.status == "submitted" and os.time() - operation.created_at >= 30 then
      operation.status = "failed"
      operation.error_code = "service_operation_timeout"
    end
    write_operation(operation)
  end
  result.service_state = running and (result.service_state or "starting") or "stopped"
  result.expected_state = read_enabled() and "enabled" or "disabled"
  result.bagualu_pid = running and (instance.pid or result.bagualu_pid or 0) or 0
  result.instance = instance
  if operation then
    result = result or {}
    result.operation_id = operation.operation_id
    result.operation_status = operation.status
    result.operation_action = operation.action
  end
  luci.http.prepare_content("application/json")
  luci.http.write_json(result or {error = "service status unavailable"})
end

function read_runtime_status()
  local uci = require("uci").cursor()
  local path = uci:get("bagualu", "main", "status_file") or "/var/run/bagualu/status.json"
  local file = io.open(path, "r")
  if not file then return nil end
  local content = file:read("*a")
  file:close()
  local ok, value = pcall(jsonc.parse, content)
  if ok then return value end
  return nil
end

function action(name)
  local allowed = {start = true, stop = true, restart = true, enable = true, disable = true}
  if not allowed[name] then
    luci.http.status(400, "invalid action")
    return
  end
  if name == "enable" or name == "disable" then
    local uci = require("uci").cursor()
    uci:set("bagualu", "main", "enabled", name == "enable" and "1" or "0")
    uci:commit("bagualu")
  end
  local exit_code = require("luci.sys").call("/etc/init.d/bagualu " .. name .. " >/dev/null 2>&1")
  if exit_code ~= 0 then
    luci.http.status(500, "service operation failed")
    return
  end
  math.randomseed(os.time())
  local operation = {
    operation_id = string.format("luci-%d-%d", os.time(), math.random(100000, 999999)),
    action = name,
    status = "submitted",
    created_at = os.time()
  }
  write_operation(operation)
  luci.http.prepare_content("application/json")
  luci.http.write_json(operation)
end

function read_enabled()
  local uci = require("uci").cursor()
  return uci:get("bagualu", "main", "enabled") ~= "0"
end

function read_operation()
  local file = io.open("/tmp/bagualu-luci-operation.json", "r")
  if not file then return nil end
  local content = file:read("*a")
  file:close()
  local ok, value = pcall(jsonc.parse, content)
  if ok then return value end
  return nil
end

function write_operation(operation)
  local file = io.open("/tmp/bagualu-luci-operation.json", "w")
  if not file then return end
  file:write(jsonc.stringify(operation))
  file:close()
end
