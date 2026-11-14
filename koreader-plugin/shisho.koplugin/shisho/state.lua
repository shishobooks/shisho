--[[--
The plugin's own state, in its own file in KOReader's settings directory:

- servers: every server signed in to, each { url, username, credential,
  device_id, device_name, needs_login, update_required, info, stats_cursor,
  last_synced, disabled, signed_in_day, states_cursor, history,
  asking_history }, where info is what
  the server last answered about itself, stats_cursor is the last statistics
  row it acknowledged, disabled marks one the user turned off: the plugin
  leaves it alone, and nothing waits for it, until it's enabled again,
  signed_in_day is the day it was first signed in to, states_cursor is
  where its next pull of statuses and Ratings starts (shisho/summaries),
  history is the history it was sent (shisho/history), and asking_history
  marks one whose sign-in is still asking whether to send it.
- installed_from: the server the plugin was last installed from.
- pairing_handled: the short code of a paired zip the plugin already acted
  on, so it acts on it once.
- journal: positions not yet acknowledged by every server (shisho/journal).
- settings: sync on sleep, what to do with pulled positions
  (shisho/positions), and what first sign-in does about history
  (shisho/history).

Each write replaces the file atomically, so a crash or a dead battery leaves
the old state or the new one, never half of either. No UI.
]]
local Files = require("shisho/files")
local LuaSettings = require("luasettings")
local Urls = require("shisho/urls")

local State = {}
State.__index = State

function State.open(path)
    local data = LuaSettings:open(path).data
    if type(data.servers) ~= "table" then
        data.servers = {}
    end
    return setmetatable({ path = path, data = data }, State)
end

function State:servers()
    return self.data.servers
end

-- isEnabled reports whether the plugin syncs with server.
function State:isEnabled(server)
    return not server.disabled
end

-- enabledServers is every server the plugin syncs with, checks for updates
-- with, and keeps reading waiting for.
function State:enabledServers()
    local enabled = {}
    for __, server in ipairs(self.data.servers) do
        if self:isEnabled(server) then table.insert(enabled, server) end
    end
    return enabled
end

-- setEnabled turns the server at url on or off.
function State:setEnabled(url, enabled)
    local server = self:find(url)
    if server then
        server.disabled = not enabled or nil
    end
    return server
end

function State:find(url)
    for __, server in ipairs(self.data.servers) do
        if Urls.same(server.url, url) then
            return server
        end
    end
end

-- signIn records a sign-in to the server at fields.url, replacing an earlier
-- one, and clears what needed attention. Returns the server, whether it now
-- signs in as another Device, whose statuses and Ratings sync afresh, and
-- whether it's the first sign-in to the server. A server signed in to for
-- the first time takes reading from fields.stats_cursor on, the statistics
-- recorded by the time of sign-in, unless it's sent history
-- (shisho/history); signing in again keeps its place, so nothing queued is
-- lost.
function State:signIn(fields)
    local server = self:find(fields.url)
    local other_device = false
    local first = not server
    if not server then
        server = { url = Urls.normalize(fields.url) }
        table.insert(self.data.servers, server)
    elseif server.device_id and fields.device_id and server.device_id ~= fields.device_id then
        other_device = true
        server.states_cursor = nil
        server.signed_in_day = nil
    end
    server.username = fields.username
    server.credential = fields.credential
    server.device_id = fields.device_id
    server.device_name = fields.device_name
    server.info = fields.info or server.info
    server.needs_login = nil
    server.update_required = nil
    if not server.stats_cursor then
        server.stats_cursor = fields.stats_cursor or { rowid = 0 }
    end
    State.signedInDay(server, fields.signed_in_day or os.date("%Y-%m-%d"))
    return server, other_device, first
end

-- signedInDay is the day the plugin first signed in to server, taking today
-- for a server signed in to before the plugin recorded it.
function State.signedInDay(server, today)
    if not server.signed_in_day then
        server.signed_in_day = today
    end
    return server.signed_in_day
end

function State:remove(url)
    for i, server in ipairs(self.data.servers) do
        if Urls.same(server.url, url) then
            table.remove(self.data.servers, i)
            return true
        end
    end
    return false
end

function State:get(key)
    return self.data[key]
end

function State:set(key, value)
    self.data[key] = value
end

-- flush writes the state atomically.
function State:flush()
    return Files.write(self.path, self.data, "The Shisho KOReader plugin's state.")
end

return State
