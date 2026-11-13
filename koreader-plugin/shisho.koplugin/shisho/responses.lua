-- What a server's answer means to the plugin. No UI.
local Protocol = require("shisho/protocol")
local _ = require("gettext")

local Responses = {}

--[[--
classify turns an HTTP status and Shisho's error body into what happened:
"ok", "needs_login" (the server no longer accepts this Device's sign-in),
"update_required" (it no longer accepts this plugin's protocol), "not_found",
"rate_limited", "demo", "unreachable" (no answer at all), or "error".
]]
function Responses.classify(code, body)
    local error_code = type(body) == "table" and type(body.error) == "table" and body.error.code or nil
    if code == 200 then return "ok" end
    if code == nil or type(code) ~= "number" then return "unreachable" end
    if code == 426 or error_code == "plugin_update_required" then return "update_required" end
    if code >= 300 and code < 400 then return "redirect" end
    if error_code == "demo_mode" then return "demo" end
    if code == 401 then return "needs_login" end
    if code == 404 then return "not_found" end
    if code == 429 then return "rate_limited" end
    return "error"
end

-- message is what to tell the user about an outcome, using the server's
-- own message where it sent one.
function Responses.message(outcome, body, info)
    local server_message = type(body) == "table" and type(body.error) == "table" and body.error.message or nil
    if outcome == "update_required" then
        if type(info) == "table" and type(info.protocol_max) == "number" and info.protocol_max < Protocol.version then
            return _("Update required: this plugin is newer than the server. Upgrade Shisho on it, or install the plugin it serves.")
        end
        return _("Update required: this server needs a newer KOReader plugin.")
    elseif outcome == "redirect" then
        return _("The server sent the plugin to another address. Enter the address the server is at, starting with https:// if it uses it.")
    elseif outcome == "unreachable" then
        return _("Couldn't reach the server. Check the URL and your network.")
    elseif outcome == "not_found" then
        return _("That pairing code is wrong or has expired. Press Pair on the Device's page in Shisho for a new one.")
    elseif outcome == "needs_login" then
        return server_message or _("Wrong username or Device password.")
    end
    return server_message or _("The server couldn't do that. Try again later.")
end

-- infoFromHeaders reads what a server that refused this plugin's protocol
-- said about itself, from the headers it sends a signed-in plugin.
function Responses.infoFromHeaders(headers)
    if type(headers) ~= "table" then return nil end
    local function header(name) return headers[name:lower()] end
    local info = {
        protocol_min = tonumber(header("X-Shisho-Protocol-Min")),
        protocol_max = tonumber(header("X-Shisho-Protocol-Max")),
        plugin_version = header("X-Shisho-Server-Plugin-Version"),
        plugin_hash = header("X-Shisho-Server-Plugin-Hash"),
        plugin_protocol = tonumber(header("X-Shisho-Server-Plugin-Protocol")),
    }
    if not (info.protocol_min and info.protocol_max and info.plugin_version and info.plugin_protocol) then
        return nil
    end
    return info
end

--[[--
checkIn records a server's answer to a check-in on server: what it serves
when it answered, or refused this plugin's protocol, and whether it needs a
new sign-in or a newer plugin. An unreachable server keeps what it last
said. Returns the outcome.
]]
function Responses.checkIn(server, code, body, headers)
    local outcome = Responses.classify(code, body)
    if outcome == "ok" then
        server.info = type(body) == "table" and body.server or server.info
        server.device_name = type(body) == "table" and body.device_name or server.device_name
        server.needs_login = nil
        server.update_required = nil
    elseif outcome == "needs_login" then
        server.needs_login = true
    elseif outcome == "update_required" then
        server.update_required = true
        server.needs_login = nil
        server.info = Responses.infoFromHeaders(headers) or server.info
    end
    return outcome
end

-- serverStatus is a short status for a signed-in server in the menu.
function Responses.serverStatus(server)
    if server.disabled then
        return _("Disabled")
    elseif server.update_required then
        return _("Update required")
    elseif server.needs_login then
        return _("Needs login")
    end
    return nil
end

return Responses
