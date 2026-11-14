--[[--
Sends one server everything it hasn't acknowledged (ADR 0016): page rows
from KOReader's statistics after the server's cursor, and journal entries it
hasn't acknowledged, in bounded batches. Each batch is acknowledged whole,
and only then does the cursor move and the entries count as sent, so a sync
cut off partway resumes without sending anything twice.

Journal entries go only with the last batch of rows, and status and Rating
changes after them. The server places a session by its time and lets a
position start a Read-through, so a position from the end of a long offline
stretch must not arrive before the sessions that led up to it, and a reread
the device started must not arrive before its first pages, which the server
then places on it.

History the person chose to send (shisho/history) goes before all of it:
its rows, and then the statuses and Ratings KOReader recorded before signing
in, so a finished book's Read-through begins on the day of its first session,
and reading since sign-in lands on the Read-throughs history made. No UI.
]]
local History = require("shisho/history")
local Responses = require("shisho/responses")
local Statistics = require("shisho/statistics")
local Summaries = require("shisho/summaries")

local Sync = {}

-- The most rows and events one upload carries.
Sync.BATCH = 500
Sync.EVENT_BATCH = 100

-- HISTORY_WHILE_BUSY is how many batches of history rows a sync while
-- reading or going to sleep sends, so a long history holds up neither.
Sync.HISTORY_WHILE_BUSY = 2

-- RAN_RECENTLY is how many seconds a sync stands for: Wi-Fi coming on
-- within it, as it does right after Sync now or sync on sleep turns it on,
-- doesn't sync again.
Sync.RAN_RECENTLY = 60

-- ranRecently reports whether a sync at synced_at counts as just run at
-- now. A clock set back since doesn't count as recent.
function Sync.ranRecently(synced_at, now)
    if not synced_at then return false end
    local since = now - synced_at
    return since >= 0 and since < Sync.RAN_RECENTLY
end

-- PAGES_BETWEEN is how many pages turned with Wi-Fi already on bring a
-- sync, so a device that stays online keeps Shisho current as you read.
Sync.PAGES_BETWEEN = 20

-- dueAfterPages reports whether pages turned since the last sync bring one.
function Sync.dueAfterPages(pages)
    return type(pages) == "number" and pages >= Sync.PAGES_BETWEEN
end

-- SLEEP_GAP is how many seconds sync on sleep waits after the last sync,
-- so short sittings between other things don't turn Wi-Fi on at every sleep.
Sync.SLEEP_GAP = 60 * 60

-- lastSynced is when any of servers last synced, or nil when none has.
function Sync.lastSynced(servers)
    local latest
    for __, server in ipairs(servers) do
        if server.last_synced and (not latest or server.last_synced > latest) then
            latest = server.last_synced
        end
    end
    return latest
end

-- dueOnSleep reports whether going to sleep at now syncs, the last sync
-- having been at last_synced. A clock set back since can't hold it off.
function Sync.dueOnSleep(last_synced, now)
    if not last_synced then return true end
    local since = now - last_synced
    return since < 0 or since >= Sync.SLEEP_GAP
end

-- syncable is the servers a sync can go to: enabled, signed in, and with a
-- plugin they accept.
function Sync.syncable(servers)
    local out = {}
    for __, server in ipairs(servers) do
        if not server.disabled and not server.needs_login and not server.update_required then
            table.insert(out, server)
        end
    end
    return out
end

--[[--
onSleep is what going to sleep does, from opts { online, syncable (how many
servers can take a sync), sync_on_sleep, last_synced, last_attempt,
wifi_turns_on, now }: "sync" on Wi-Fi that's already on, which costs
nothing; "wifi" to turn Wi-Fi on for it, only with sync on sleep on, an hour
since the last sync or attempt, and KOReader turning Wi-Fi on without
asking; or nil.
]]
function Sync.onSleep(opts)
    if (opts.syncable or 0) == 0 then return nil end
    if opts.online then return "sync" end
    if not opts.sync_on_sleep then return nil end
    local last = math.max(opts.last_synced or 0, opts.last_attempt or 0)
    if last > 0 and not Sync.dueOnSleep(last, opts.now) then return nil end
    if not opts.wifi_turns_on then return nil end
    return "wifi"
end

-- BACKOFF is how many seconds a server that couldn't be reached, or failed,
-- is left out of the syncs that happen while reading, so a server that's
-- down doesn't hold up a page turn every few pages.
Sync.BACKOFF = 15 * 60

-- noteOutcome records how a sync with server went at now: a failure of the
-- network or the server starts its back-off, and a success ends it. A
-- refused sign-in or plugin isn't the network, and has its own status.
function Sync.noteOutcome(server, outcome, now)
    if outcome == "ok" then
        server.last_failed = nil
    elseif outcome == "unreachable" or outcome == "error" then
        server.last_failed = now
    end
end

-- backingOff reports whether server is left out of syncs while reading at
-- now. A clock set back since doesn't hold it off.
function Sync.backingOff(server, now)
    if not server.last_failed then return false end
    local since = now - server.last_failed
    return since >= 0 and since < Sync.BACKOFF
end

-- afterPage is what a page turn does, pages having been turned since the
-- last sync: "sync" once enough are, on a connection; "reset" without one,
-- so it isn't tried again on every page until there is; or nil.
function Sync.afterPage(pages, connected)
    if not Sync.dueAfterPages(pages) then return nil end
    return connected and "sync" or "reset"
end

--[[--
onConnected is what Wi-Fi coming on does, from opts { servers (how many are
enabled), background (a sync while away turned it on), screen_saver,
recent_check, ran_recently, awaiting_restart }: { check, sync } for the
update check and a sync. Wi-Fi a sync while away turned on, or that came on
behind the sleep screen, does nothing more: no one is there to answer an
update offer, and the sync has run.
]]
function Sync.onConnected(opts)
    if (opts.servers or 0) == 0 or opts.awaiting_restart or opts.background or opts.screen_saver then
        return { check = false, sync = false }
    end
    return { check = not opts.recent_check, sync = not opts.ran_recently }
end

-- noteRefusal marks server as refusing the sign-in or the plugin, which
-- waits for the person, unlike a network failure.
local function noteRefusal(server, outcome)
    if outcome == "needs_login" then
        server.needs_login = true
    elseif outcome == "update_required" then
        server.update_required = true
    end
end

local function payloadRow(row)
    return {
        document = row.document,
        page = row.page,
        total_pages = row.total_pages,
        start_time = row.start_time,
        duration = row.duration,
    }
end

--[[--
sendHistory sends the history still to go to opts.server, as Sync.run does,
through send(payload), which returns the outcome, adding what it sent to
sent. Up to opts.history_batches batches of rows go, when set. Its statuses
and Ratings wait until every row has gone, so with statistics off, they
wait for statistics. Returns the outcome, and an error reading statistics.
]]
local function sendHistory(opts, send, sent)
    local state, server = opts.state, opts.server
    if type(server.history) ~= "table" then return "ok" end
    local batch = opts.batch or Sync.BATCH
    local batches = 0
    while opts.read_pages and History.rowsPending(server) do
        if opts.history_batches and batches >= opts.history_batches then return "ok" end
        batches = batches + 1
        local read, err = opts.read_pages(server.stats_cursor, batch)
        if not read then return "error", err end
        local rows = History.rows(server.history, read)
        if #rows == 0 then
            -- Rows statistics leaves out, such as ones with no time, end it.
            server.stats_cursor = { rowid = server.history.through }
            state:flush()
            break
        end
        local payload = { backfill = true, pages = {} }
        for __, row in ipairs(rows) do table.insert(payload.pages, payloadRow(row)) end
        local outcome = send(payload)
        if outcome ~= "ok" then return outcome end
        server.stats_cursor = Statistics.cursorAfter(rows[#rows])
        sent.pages = sent.pages + #rows
        state:flush()
        if opts.progress then opts.progress(sent.pages) end
    end
    if History.rowsPending(server) then return "ok" end
    local events = opts.history or {}
    local first = 1
    while first <= #events do
        local chunk = History.batch(events, first, Sync.EVENT_BATCH)
        local outcome = send({ backfill = true, events = chunk })
        if outcome ~= "ok" then return outcome end
        for __, event in ipairs(chunk) do Summaries.acknowledge(opts.store, event) end
        sent.events = sent.events + #chunk
        first = first + #chunk
        state:flush()
    end
    return "ok"
end

--[[--
run syncs opts.server. opts:

- state, journal: the plugin's state and journal
- post(payload): sends an upload, returning the status and decoded answer
- read_pages(cursor, limit): the statistics reader, or nil when statistics
  is off, in which case only the journal goes
- clock(): the device's clock, os.time by default
- batch: rows per upload
- changes: status and Rating changes for this server (Summaries.collect)
- store: what was synced with this server (Summaries:server), where each
  change the server takes in is noted
- history: the statuses and Ratings to send as history (History.events)
- progress(pages): told how many rows of history went so far
- history_batches: the most batches of history rows to send, for a sync
  that shouldn't hold anything up long; the rest of the sync then waits
  for one that sends the rest

Returns the outcome, as Responses.classify names it, and the number of
rows and events sent.
]]
function Sync.run(opts)
    local state, journal, server = opts.state, opts.journal, opts.server
    local batch = opts.batch or Sync.BATCH
    local clock = opts.clock or os.time
    local changes = opts.changes or {}
    local next_change = 1
    local sent = { pages = 0, events = 0 }
    -- send posts payload, noting a refusal, which waits for the person.
    local function send(payload)
        payload.clock = clock()
        local status, body = opts.post(payload)
        local outcome = Responses.classify(status, body)
        if outcome ~= "ok" then
            -- Changes not taken in are worked out again by the next sync.
            noteRefusal(server, outcome)
            state:flush()
        else
            server.needs_login = nil
        end
        return outcome
    end

    -- Statistics off leaves history rows to go, and reading after it can go
    -- once statistics are on again. Only a sync cut short waits.
    local outcome, err = sendHistory(opts, send, sent)
    if outcome ~= "ok" then return outcome, sent.pages, sent.events, err end
    if opts.read_pages and History.rowsPending(server) then return "ok", sent.pages, sent.events end
    while true do
        local rows = {}
        if opts.read_pages then
            local read, err = opts.read_pages(server.stats_cursor, batch)
            if not read then return "error", sent.pages, sent.events, err end
            rows = read
        end
        -- Positions follow every row, so they go only once the rows run out,
        -- and status and Rating changes follow the positions.
        local last_rows = #rows < batch
        local events = last_rows and journal:pending(server.url, Sync.EVENT_BATCH) or {}
        local batch_changes = {}
        while last_rows and #events < Sync.EVENT_BATCH and next_change <= #changes do
            table.insert(events, changes[next_change])
            table.insert(batch_changes, changes[next_change])
            next_change = next_change + 1
        end
        if #rows == 0 and #events == 0 then break end

        local payload = {}
        if #rows > 0 then
            payload.pages = {}
            for __, row in ipairs(rows) do table.insert(payload.pages, payloadRow(row)) end
        end
        if #events > 0 then payload.events = events end

        outcome = send(payload)
        if outcome ~= "ok" then return outcome, sent.pages, sent.events end

        if #rows > 0 then
            server.stats_cursor = Statistics.cursorAfter(rows[#rows])
        end
        local ids = {}
        for __, event in ipairs(events) do table.insert(ids, event.id) end
        journal:acknowledge(server.url, ids)
        for __, change in ipairs(batch_changes) do
            Summaries.acknowledge(opts.store, change)
        end
        sent.pages = sent.pages + #rows
        sent.events = sent.events + #events
        state:flush()
        if last_rows and #events < Sync.EVENT_BATCH then break end
    end
    server.last_synced = clock()
    journal:prune()
    state:flush()
    return "ok", sent.pages, sent.events
end

--[[--
pullStates asks opts.server for the statuses and Ratings the device should
hold, and passes each state to opts.write(state, outright): every book whose
status or Rating changed since its last pull, and, outright, the books in
opts.documents it hasn't been asked about (Summaries.toAsk). opts.store is
what was synced with the server (Summaries:server), opts.post(payload) sends
a pull, and opts.clock() is the device's clock. Returns the outcome.
]]
function Sync.pullStates(opts)
    local server, store = opts.server, opts.store
    local clock = opts.clock or os.time
    local first = true
    while true do
        local now = clock()
        local ask = Summaries.toAsk(store.asked, opts.documents or {}, now)
        if not first and #ask == 0 then break end
        local payload = {}
        if #ask > 0 then payload.statuses = ask end
        if first and server.states_cursor then payload.since = server.states_cursor end
        local status, body = opts.post(payload)
        local outcome = Responses.classify(status, body)
        if outcome ~= "ok" then
            noteRefusal(server, outcome)
            return outcome
        end
        local outright, recognized = {}, {}
        for __, document in ipairs(ask) do outright[document] = true end
        for __, pulled in ipairs(type(body.states) == "table" and body.states or {}) do
            if type(pulled) == "table" and pulled.document then
                recognized[pulled.document] = true
                opts.write(pulled, outright[pulled.document] or false)
            end
        end
        -- A book the server didn't recognize is asked about again later.
        for __, document in ipairs(ask) do
            store.asked[document] = recognized[document] or now
        end
        if first and tonumber(body.cursor) then
            server.states_cursor = tonumber(body.cursor)
        end
        first = false
        if #ask < Summaries.ASK_BATCH then break end
    end
    return "ok"
end

return Sync
