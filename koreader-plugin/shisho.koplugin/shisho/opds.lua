-- A Shisho server's entry in KOReader's OPDS catalog list. No UI: the list
-- is KOReader's own, read and written by the caller.
local Urls = require("shisho/urls")

local Opds = {}

-- catalogURL is the OPDS catalog of the Shisho server at server_url, as the
-- Device page shows it.
function Opds.catalogURL(server_url)
    return Urls.normalize(server_url) .. "/opds/v1/catalog"
end

--[[--
upsert adds a Shisho server's catalog to KOReader's catalog list, or updates
the entry already there for its URL, however it was added, so the list never
holds the server twice. An entry keeps its title, so one the user renamed
stays renamed.

entry is { title, url, username, password }. Returns the entry in the list
and whether it was added.
]]
function Opds.upsert(servers, entry)
    for __, existing in ipairs(servers) do
        if Urls.same(existing.url, entry.url) then
            existing.username = entry.username
            existing.password = entry.password
            return existing, false
        end
    end
    local added = {
        title = entry.title,
        url = entry.url,
        username = entry.username,
        password = entry.password,
    }
    table.insert(servers, added)
    return added, true
end

return Opds
