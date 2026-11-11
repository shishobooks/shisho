--[[--
What the server that built this plugin's zip stamped into it:

- build.lua: { version, hash } of the plugin. Absent in a copy of the
  source, which then counts as version 0.0.0-dev.
- pairing.lua: { server_url, short_code, expires_at }, only in a zip
  downloaded from a Device's page, so the plugin can sign itself in.

They're loaded by path, not require, so another plugin's module of the same
name can't stand in for them. No UI.
]]
local Stamps = {}

local function load(path)
    local ok, value = pcall(dofile, path)
    if ok and type(value) == "table" then
        return value
    end
end

function Stamps.load(plugin_path)
    local build = load(plugin_path .. "/build.lua") or {}
    return {
        version = type(build.version) == "string" and build.version or "0.0.0-dev",
        hash = type(build.hash) == "string" and build.hash or "",
        pairing = load(plugin_path .. "/pairing.lua"),
    }
end

-- pairingState says what to do with a paired zip at time now: "sign_in"
-- while its short code is unexpired, "ask" to open sign-in with its server
-- filled in once it has expired, or nil when there's nothing to do, because
-- there's no pairing, it was handled already, or its server is signed in.
function Stamps.pairingState(pairing, handled_code, signed_in, now)
    if type(pairing) ~= "table" or type(pairing.server_url) ~= "string" then
        return nil
    end
    if signed_in or (pairing.short_code ~= nil and pairing.short_code == handled_code) then
        return nil
    end
    if type(pairing.short_code) == "string" and type(pairing.expires_at) == "number" and now < pairing.expires_at then
        return "sign_in"
    end
    return "ask"
end

return Stamps
