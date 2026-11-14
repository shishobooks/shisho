--[[--
Sending the history KOReader recorded before the plugin signed in to a
server (ADR 0016), when the person chooses to on first sign-in: every
statistics row from the first up to where sign-in left off, and then each
book's status and stars KOReader recorded before signing in (the books
shisho/summaries notes as before). The server fills in only what Shisho has
none of from it, so sending it again changes nothing.

A server that was sent history has server.history = { through }, where
through is the last statistics row recorded by sign-in. Its statistics
cursor starts at the first row, and rows after through are reading like any
other. A server whose sign-in is asking whether to send it has
server.asking_history, and nothing goes to it until the person answers. No
UI.
]]
local random = require("frontend/random")

local History = {}

local SETTINGS = { ask = true, send = true, skip = true }

-- setting is what first sign-in to a server does about history, from the
-- plugin's saved settings: ask, send, or skip. Asks by default.
function History.setting(saved)
    local value = type(saved) == "table" and saved.send_history or nil
    return SETTINGS[value] and value or "ask"
end

-- onSignIn is what a sign-in does about history, given the setting and
-- whether it's the first to the server: "ask", "send", or nil. Signing in
-- again keeps what was chosen.
function History.onSignIn(setting, first)
    if not first or setting == "skip" then return nil end
    return setting
end

-- signIn does what a sign-in to server does about history, given the
-- setting and whether it's the first to the server: sends it, starts
-- asking, or nothing.
function History.signIn(server, setting, first)
    local action = History.onSignIn(setting, first)
    if action == "send" then
        History.accept(server)
    elseif action == "ask" then
        server.asking_history = true
    end
end

-- answer records the person's answer for server, sending its history or
-- only reading from sign-in on.
function History.answer(server, send)
    server.asking_history = nil
    if send then History.accept(server) end
end

-- waiting is the first of servers still asking whether to send history, or
-- nil.
function History.waiting(servers)
    for __, server in ipairs(servers) do
        if server.asking_history then return server end
    end
end

-- accept sends server history from the first statistics row up to where
-- sign-in left off. Accepted again, it keeps its place.
function History.accept(server)
    if type(server.history) == "table" then return end
    local cursor = server.stats_cursor
    server.history = { through = type(cursor) == "table" and tonumber(cursor.rowid) or 0 }
    server.stats_cursor = { rowid = 0 }
end

-- rowsPending reports whether rows of server's history are still to go.
function History.rowsPending(server)
    local history = server.history
    if type(history) ~= "table" then return false end
    local cursor = type(server.stats_cursor) == "table" and tonumber(server.stats_cursor.rowid) or 0
    return cursor < (tonumber(history.through) or 0)
end

-- rows is the rows of read, in order, that belong to history.
function History.rows(history, read)
    local out = {}
    for __, row in ipairs(read) do
        if row.rowid > history.through then break end
        table.insert(out, row)
    end
    return out
end

local function sortedKeys(t)
    local keys = {}
    for key in pairs(t) do table.insert(keys, key) end
    table.sort(keys)
    return keys
end

--[[--
events is the status and Rating of each of books (Summaries.addBook) that
KOReader recorded before signing in to the server whose synced store is
store, as events to send as history, dated by the day KOReader recorded
them. A book's events are next to each other. Once the server takes them
in (Summaries.acknowledge), the book is synced like any other.
]]
function History.events(store, books)
    local events = {}
    for __, document in ipairs(sortedKeys(books)) do
        local book = books[document]
        local synced = store.synced[document]
        local current = book.current
        if not book.unread and current and current.day and type(synced) == "table" and synced.before then
            if current.status then
                table.insert(events, {
                    id = random.uuid(true), type = "status", document = document,
                    status = current.status, day = current.day,
                })
            end
            if current.rating then
                table.insert(events, {
                    id = random.uuid(true), type = "rating", document = document,
                    rating = current.rating, day = current.day,
                })
            end
        end
    end
    return events
end

-- batch is up to limit of events from first on, never splitting a book's,
-- so a sync cut off between batches can't take in half of a book.
function History.batch(events, first, limit)
    local out = {}
    local i = first
    while i <= #events do
        local last = i
        while last < #events and events[last + 1].document == events[i].document do last = last + 1 end
        if #out > 0 and #out + (last - i + 1) > limit then break end
        for k = i, last do table.insert(out, events[k]) end
        i = last + 1
    end
    return out
end

return History
