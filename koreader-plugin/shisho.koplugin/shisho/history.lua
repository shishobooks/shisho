--[[--
Sending the history KOReader recorded before the plugin signed in to a
server (ADR 0016), when the person chooses to on first sign-in: every
statistics row from the first up to where sign-in left off, and then each
book's status and stars KOReader recorded before signing in (the books
shisho/summaries notes as before). The server fills in only what Shisho has
none of from it, so sending it again changes nothing.

A server that was sent history has server.history = { through, through_row },
where through is the last statistics row recorded by sign-in, and
through_row names that row (shisho/statistics). Its statistics
cursor starts at the first row, and rows after through are reading like any
other. A server whose sign-in is asking whether to send it has
server.asking_history, and nothing goes to it until the person answers.

Once history has gone, it can be sent again (History.resend): every
statistics row once more, from the first, with server.history.resend set
until it has all gone. Rows up to through go as history again, and the rest
as reading like any other, by the same rules as when they first went. The
server takes each row once, so this only brings back what it dropped: the
reading history of a book whose history was deleted on the Device's page
and has since been restored there (ADR 0017). It also sends the status and
stars of each book the server holds as unrecognized (History.withResent),
which the server keeps only for a book that holds none, and applies by
history's rules once the book is linked. An older server keeps them
regardless, so history goes again only to a server that lists the
resend_history capability (shisho/protocol). No UI.
]]
local Protocol = require("shisho/protocol")
local Statistics = require("shisho/statistics")
local Versions = require("shisho/versions")
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

-- sendable reports whether server can still be sent history: it wasn't,
-- and its sign-in isn't asking. History sent later (accept) goes up to
-- every row recorded by then; the server ignores the ones it already has.
function History.sendable(server)
    return type(server.history) ~= "table" and not server.asking_history
end

-- unsent reports whether server's history is still to go, or can still be
-- sent, so what KOReader recorded before signing in stays marked for it
-- (Summaries.apply).
function History.unsent(server)
    return History.rowsPending(server) or History.sendable(server)
end

-- accept sends server history from the first statistics row up to where
-- sign-in left off, or up to its cursor, when sent later. Where history ends
-- knows the latest start of any row up to it (Statistics.repair), as far as
-- the plugin knows it. Accepted again, it keeps its place.
function History.accept(server)
    if type(server.history) == "table" then return end
    local cursor = server.stats_cursor
    local through_row = Statistics.key(cursor)
    if through_row then
        local from = server.stats_from_row
        through_row.latest = math.max(type(from) == "table" and tonumber(from.latest) or 0,
            tonumber(cursor.latest) or 0, tonumber(cursor.start_time) or 0)
    end
    server.history = {
        through = type(cursor) == "table" and tonumber(cursor.rowid) or 0,
        through_row = through_row,
    }
    server.stats_cursor = { rowid = 0 }
    server.stats_from = 0
    server.stats_from_row = nil
end

-- takesResent reports whether server, by its last answer, keeps history
-- sent again by the rules it needs.
local function takesResent(server)
    return Versions.supports(server, Protocol.capabilities.resend_history)
end

-- resendable reports whether server's history can be sent again: it has
-- gone, all of it, and the server takes it.
function History.resendable(server)
    local history = server.history
    return type(history) == "table" and not history.resend and not History.rowsPending(server)
        and takesResent(server)
end

-- resend sends server every statistics row again, from the first, keeping
-- where history ends.
function History.resend(server)
    server.history.resend = true
    server.stats_cursor = { rowid = 0 }
end

-- resent records that the history server was sent again has all gone.
function History.resent(server)
    if type(server.history) == "table" then server.history.resend = nil end
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

--[[--
resendEvents is the status and stars of each of books (Summaries.addBook)
that the server holds as unrecognized, by state(document), the server's
last answer about it, as events to send with history sent again, dated by
the day KOReader saved them. A book's events are next to each other.
]]
function History.resendEvents(books, state)
    local events = {}
    for __, document in ipairs(sortedKeys(books)) do
        local current = books[document].current
        if not books[document].unread and current and current.day and state(document) == "held" then
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

-- withResent adds to events, the history events for server, the status and
-- stars of the books the server holds as unrecognized (resendEvents), by
-- recognition's last answer from it, while its history is being sent again.
-- A server rolled back since history started going again gets none, while
-- its rows still go: the server takes each row once.
function History.withResent(events, server, books, recognition)
    if type(server.history) ~= "table" or not server.history.resend then return events end
    if not takesResent(server) then return events end
    local resent = History.resendEvents(books, function(document)
        local answer = recognition:answer(server.url, document)
        return answer and answer.state
    end)
    for __, event in ipairs(resent) do table.insert(events, event) end
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
