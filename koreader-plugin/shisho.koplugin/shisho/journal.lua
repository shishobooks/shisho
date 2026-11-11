--[[--
The journal of what KOReader doesn't record itself (ADR 0016): for now, each
book's latest position, with when the reader was there. An entry waits until
every signed-in server has acknowledged it, so a server that needs a new
sign-in, or that's offline, keeps it queued. A newer position replaces the
one waiting for its book, under a new id, since only the latest place
matters. It lives in the plugin's state file. No UI.

state.journal = {
    entries = { { id, type, document, progress, percentage, at, acked = { [url] = true } } },
    last_position = { [document] = { progress, percentage, at } },
}
]]
local random = require("frontend/random")

local Journal = {}
Journal.__index = Journal

function Journal.new(state)
    local data = state:get("journal")
    if type(data) ~= "table" then
        data = {}
        state:set("journal", data)
    end
    data.entries = type(data.entries) == "table" and data.entries or {}
    data.last_position = type(data.last_position) == "table" and data.last_position or {}
    return setmetatable({ state = state, data = data }, Journal)
end

--[[--
recordPosition journals that the reader was at progress (an XPointer, or the
1-based page as a string), percentage of the way through document, at the
instant at on the device's clock. The same place as last time is not a new
position. Returns whether it recorded one.
]]
function Journal:recordPosition(document, progress, percentage, at)
    if not document or not progress then return false end
    local last = self.data.last_position[document]
    if last and last.progress == progress then
        return false
    end
    self.data.last_position[document] = { progress = progress, percentage = percentage, at = at }
    for i, entry in ipairs(self.data.entries) do
        if entry.type == "position" and entry.document == document then
            table.remove(self.data.entries, i)
            break
        end
    end
    table.insert(self.data.entries, {
        id = random.uuid(true),
        type = "position",
        document = document,
        progress = progress,
        percentage = percentage,
        at = at,
        acked = {},
    })
    return true
end

-- lastPositionAt is when the reader was last at a new place in document on
-- this device, or nil when it never was.
function Journal:lastPositionAt(document)
    local last = self.data.last_position[document]
    return last and last.at
end

-- pending returns up to limit entries the server at url hasn't acknowledged,
-- oldest first, as the upload sends them.
function Journal:pending(url, limit)
    local out = {}
    for __, entry in ipairs(self.data.entries) do
        if not entry.acked[url] then
            if limit and #out >= limit then break end
            table.insert(out, {
                id = entry.id,
                type = entry.type,
                document = entry.document,
                progress = entry.progress,
                percentage = entry.percentage,
                at = entry.at,
            })
        end
    end
    return out
end

-- acknowledge notes that the server at url has taken in the entries ids.
function Journal:acknowledge(url, ids)
    local set = {}
    for __, id in ipairs(ids) do set[id] = true end
    for __, entry in ipairs(self.data.entries) do
        if set[entry.id] then entry.acked[url] = true end
    end
end

-- prune drops the entries every signed-in server has acknowledged.
function Journal:prune()
    local servers = self.state:servers()
    local kept = {}
    for __, entry in ipairs(self.data.entries) do
        local done = true
        for __, server in ipairs(servers) do
            if not entry.acked[server.url] then done = false end
        end
        if not done then table.insert(kept, entry) end
    end
    self.data.entries = kept
end

return Journal
