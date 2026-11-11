--[[--
The plugin's own state, in its own file in KOReader's settings directory:

- servers: every server signed in to, each { url, username, credential,
  device_id, device_name, needs_login, update_required, info, stats_cursor,
  last_synced }, where info is what the server last answered about itself
  and stats_cursor is the last statistics row it acknowledged.
- installed_from: the server the plugin was last installed from.
- pairing_handled: the short code of a paired zip the plugin already acted
  on, so it acts on it once.
- journal: positions not yet acknowledged by every server (shisho/journal).
- settings: sync on sleep and what to do with pulled positions
  (shisho/positions).

Each write replaces the file atomically, so a crash or a dead battery leaves
the old state or the new one, never half of either. No UI.
]]
local LuaSettings = require("luasettings")
local Urls = require("shisho/urls")
local dump = require("dump")
local ffiutil = require("ffi/util")

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

function State:find(url)
    for __, server in ipairs(self.data.servers) do
        if Urls.same(server.url, url) then
            return server
        end
    end
end

-- signIn records a sign-in to the server at fields.url, replacing an earlier
-- one, and clears what needed attention. Returns the server. A server signed
-- in to for the first time takes reading from fields.stats_cursor on, the
-- statistics recorded by the time of sign-in; signing in again keeps its
-- place, so nothing queued is lost.
function State:signIn(fields)
    local server = self:find(fields.url)
    if not server then
        server = { url = Urls.normalize(fields.url) }
        table.insert(self.data.servers, server)
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
    return server
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

-- flush writes the state to a temporary file beside it, then renames it
-- over the old one.
function State:flush()
    local tmp = self.path .. ".tmp"
    local file, err = io.open(tmp, "wb")
    if not file then
        return nil, err
    end
    local written, write_err = file:write("-- The Shisho KOReader plugin's state.\nreturn ", dump(self.data, nil, true), "\n")
    if written then
        ffiutil.fsyncOpenedFile(file)
    end
    local closed, close_err = file:close()
    if not written or not closed then
        -- A short write, as on a full disk, must never replace good state.
        os.remove(tmp)
        return nil, write_err or close_err
    end
    local ok, rename_err = os.rename(tmp, self.path)
    if not ok then
        os.remove(tmp)
        return nil, rename_err
    end
    ffiutil.fsyncDirectory(self.path)
    return true
end

return State
