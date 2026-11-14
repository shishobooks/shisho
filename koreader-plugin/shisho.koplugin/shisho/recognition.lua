--[[--
Which server recognizes which book (ADR 0017), so a book is reported as
unrecognized only when no server the plugin signs in to recognizes it or has
it linked, and each server gets the data of every book it recognizes.

Before a sync sends anything, it asks every server it can reach what it
knows of each book with data waiting (Recognition:record): recognized,
held (the server holds its data as unrecognized), dismissed, or unknown.
For one server and one book, decide then says what to do with the book's
data:

- "send": the server recognizes it, or holds or dismissed it, so it takes
  the data as it is.
- "skip": another server recognizes it and this one never held it, so the
  data isn't this server's.
- "claim": another server recognizes it now and this one holds it, so this
  one is told to drop it, and its data is skipped from then on.
- "unrecognized": no server recognizes it, and every enabled server has
  answered since the book first appeared, so it goes as unrecognized, with
  what KOReader knows of it.
- "wait": not every server has answered since, such as one that's down or
  one that needs a new sign-in, which doesn't count as answering. Its page
  rows are noted as deferred, sent once the wait is over, and its other
  data stays queued.

"Since" counts syncs, not the device's clock, which can be set back: each
sync is a round (Recognition:beginRound), and answers and first sightings
are noted by round.

It lives in its own file in KOReader's settings directory:

round = the latest sync's round,
first_seen = { [document] = round },
servers = { [url] = { known = { [document] = { state, at = round } }, deferred = { [document] = true } } },

No UI.
]]
local Files = require("shisho/files")
local LuaSettings = require("luasettings")
local Urls = require("shisho/urls")

local Recognition = {}
Recognition.__index = Recognition

-- The most books one lookup asks about.
Recognition.ASK_BATCH = 500

Recognition.STATES = { recognized = true, held = true, dismissed = true, unknown = true }

function Recognition.open(path)
    local data = LuaSettings:open(path).data
    if type(data.servers) ~= "table" then data.servers = {} end
    if type(data.first_seen) ~= "table" then data.first_seen = {} end
    return setmetatable({
        path = path,
        data = data,
        writer = Files.writer(path, "Which Shisho server recognizes which book, for the Shisho KOReader plugin."),
    }, Recognition)
end

-- server returns what the server at url said of each book, and the books
-- whose rows wait for the others.
function Recognition:server(url)
    local server = self.data.servers[url]
    if type(server) ~= "table" then
        server = {}
        self.data.servers[url] = server
    end
    server.known = type(server.known) == "table" and server.known or {}
    server.deferred = type(server.deferred) == "table" and server.deferred or {}
    return server
end

-- remove forgets everything about the server at url.
function Recognition:remove(url)
    self.data.servers[url] = nil
end

-- flush writes the file, only when it changed.
function Recognition:flush()
    return self.writer:write(self.data)
end

-- beginRound starts a sync's round of lookups, returning its number.
function Recognition:beginRound()
    self.data.round = (tonumber(self.data.round) or 0) + 1
    return self.data.round
end

-- current is the latest sync's round.
function Recognition:current()
    return tonumber(self.data.round) or 0
end

-- noteSeen records that documents have data waiting in round now, keeping
-- the round each was first seen.
function Recognition:noteSeen(documents, now)
    for __, document in ipairs(documents) do
        if not self.data.first_seen[document] then
            self.data.first_seen[document] = now
        end
    end
end

-- record notes what the server at url answered about each book in round now:
-- answers is the lookup's documents, each { document, state }.
function Recognition:record(url, answers, now)
    local known = self:server(url).known
    for __, answer in ipairs(type(answers) == "table" and answers or {}) do
        if type(answer) == "table" and answer.document and Recognition.STATES[answer.state] then
            known[answer.document] = { state = answer.state, at = now }
        end
    end
end

-- answer is what the server at url last said of document, or nil.
function Recognition:answer(url, document)
    local server = self.data.servers[url]
    return type(server) == "table" and type(server.known) == "table" and server.known[document] or nil
end

--[[--
decide is what to do with document's data for the server at url, given the
enabled servers, as the module's comment describes.
]]
function Recognition:decide(servers, url, document)
    local mine = self:answer(url, document)
    if mine and mine.state == "recognized" then return "send" end
    for __, other in ipairs(servers) do
        if not Urls.same(other.url, url) then
            local theirs = self:answer(other.url, document)
            if theirs and theirs.state == "recognized" then
                return (mine and mine.state == "held") and "claim" or "skip"
            end
        end
    end
    if mine and (mine.state == "held" or mine.state == "dismissed") then return "send" end
    local since = self.data.first_seen[document] or 0
    for __, server in ipairs(servers) do
        if server.needs_login then return "wait" end
        local answer = self:answer(server.url, document)
        if not answer or (tonumber(answer.at) or 0) < since then return "wait" end
    end
    return "unrecognized"
end

-- sendable reports whether an action from decide sends the data.
function Recognition.sendable(action)
    return action == "send" or action == "unrecognized"
end

-- claimed notes that the server at url dropped documents another server
-- recognizes, so they're never claimed again.
function Recognition:claimed(url, documents, now)
    local known = self:server(url).known
    for __, document in ipairs(documents) do
        known[document] = { state = "unknown", at = now }
    end
end

-- claims is the documents of documents the server at url holds that it
-- should be told to drop.
function Recognition:claims(servers, url, documents)
    local out = {}
    for __, document in ipairs(documents) do
        if self:decide(servers, url, document) == "claim" then table.insert(out, document) end
    end
    return out
end

-- held is every document a server holds as unrecognized, in order, which
-- each sync asks about again, so a link made on one server reaches the
-- others even when nothing more was read.
function Recognition:held()
    local seen, out = {}, {}
    for __, server in pairs(self.data.servers) do
        for document, answer in pairs(type(server) == "table" and type(server.known) == "table" and server.known or {}) do
            if type(answer) == "table" and answer.state == "held" and not seen[document] then
                seen[document] = true
                table.insert(out, document)
            end
        end
    end
    table.sort(out)
    return out
end

-- lines is a list from what KOReader keeps as one value per line, such as
-- a book's authors or identifiers, or a list already.
function Recognition.lines(value)
    local out = {}
    local function add(line)
        line = tostring(line):gsub("^%s+", ""):gsub("%s+$", "")
        if line ~= "" then table.insert(out, line) end
    end
    if type(value) == "table" then
        for __, line in ipairs(value) do add(line) end
    elseif type(value) == "string" then
        for line in value:gmatch("[^\n]+") do add(line) end
    end
    return out
end

-- waiting is every book with data waiting for one of plans' servers, each
-- plan { server, changes, history }: statistics rows after its cursor
-- (sources.statistics(server)), rows deferred for it, journal entries
-- (sources.journal(url)), status and Rating changes, and history, plus every
-- book a server holds as unrecognized, in case another server recognizes it
-- now. In order.
function Recognition:waiting(plans, sources)
    local seen, documents = {}, {}
    local function add(document)
        if document and not seen[document] then
            seen[document] = true
            table.insert(documents, document)
        end
    end
    for __, plan in ipairs(plans) do
        local url = plan.server.url
        for __, document in ipairs(sources.statistics and sources.statistics(plan.server) or {}) do add(document) end
        for __, document in ipairs(self:deferred(url)) do add(document) end
        for __, entry in ipairs(sources.journal and sources.journal(url) or {}) do add(entry.document) end
        for __, change in ipairs(plan.changes or {}) do add(change.document) end
        for __, event in ipairs(plan.history or {}) do add(event.document) end
    end
    for __, document in ipairs(self:held()) do add(document) end
    table.sort(documents)
    return documents
end

-- The most a server takes of each thing KOReader knows of a book.
Recognition.LIMITS = { text = 1000, list = 50, item = 500, language = 64, file_type = 16 }

local function clip(value, limit)
    if type(value) ~= "string" or value == "" then return nil end
    return #value > limit and value:sub(1, limit) or value
end

local function clipList(list)
    local out = {}
    for __, item in ipairs(type(list) == "table" and list or {}) do
        if #out >= Recognition.LIMITS.list then break end
        item = clip(item, Recognition.LIMITS.item)
        if item then table.insert(out, item) end
    end
    -- KOReader's JSON writes an empty list as an object, which the server
    -- refuses, so a list with nothing in it is left out.
    return #out > 0 and out or nil
end

--[[--
about is what KOReader knows of a book, for a server to list it as
unrecognized, from stats (Statistics.book), the book's path when the plugin
knows it, and its sidecar's doc_props. Statistics wins over the sidecar,
which fills in what statistics lacks, and the identifiers come from the
sidecar alone. Everything is cut to what a server takes.
]]
function Recognition.about(stats, path, props)
    stats = type(stats) == "table" and stats or {}
    props = type(props) == "table" and props or {}
    local authors = stats.authors
    if type(authors) ~= "table" or #authors == 0 then authors = Recognition.lines(props.authors) end
    local file_type
    if type(path) == "string" then
        local lower = path:lower()
        file_type = lower:match("%.kepub%.epub$") and "kepub" or lower:match("%.([^./]+)$")
    end
    return {
        title = clip(stats.title or props.title, Recognition.LIMITS.text),
        authors = clipList(authors),
        identifiers = clipList(Recognition.lines(props.identifiers)),
        language = clip(stats.language or props.language, Recognition.LIMITS.language),
        -- An extension cut short would name the wrong kind of file.
        file_type = file_type and #file_type <= Recognition.LIMITS.file_type and file_type or nil,
        reading_seconds = tonumber(stats.reading_seconds),
        first_read = tonumber(stats.first_read),
        last_read = tonumber(stats.last_read),
    }
end

-- defer notes that document's rows wait for the server at url.
function Recognition:defer(url, document)
    self:server(url).deferred[document] = true
end

-- deferred is the documents whose rows wait for the server at url, in
-- order.
function Recognition:deferred(url)
    local out = {}
    for document in pairs(self:server(url).deferred) do table.insert(out, document) end
    table.sort(out)
    return out
end

-- undefer notes that document's rows no longer wait for the server at url.
function Recognition:undefer(url, document)
    self:server(url).deferred[document] = nil
end

return Recognition
