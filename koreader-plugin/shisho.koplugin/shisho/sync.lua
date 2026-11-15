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
and reading since sign-in lands on the Read-throughs history made.

With several servers, each book's data goes only where shisho/recognition
decides (ADR 0017): a book another server recognizes is skipped, or claimed
from a server that held it, and one no server recognizes waits until every
server has answered, its rows deferred and sent once it's decided. No UI.
]]
local History = require("shisho/history")
local Recognition = require("shisho/recognition")
local Responses = require("shisho/responses")
local Statistics = require("shisho/statistics")
local Summaries = require("shisho/summaries")

local Sync = {}

-- The most rows, events, claims, and books' details one upload carries.
Sync.BATCH = 500
Sync.EVENT_BATCH = 100
Sync.CLAIM_BATCH = 500
Sync.DETAILS_BATCH = 200

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

--[[--
holdBack is what opts.recognition decides for opts.server's data, for one
sync: every book's data goes when there's no recognition to ask.
]]
local function holdBack(opts)
    local recognition, url = opts.recognition, opts.server.url
    local actions = {}
    local h = {}

    function h.action(document)
        if not recognition then return "send" end
        if not actions[document] then
            actions[document] = recognition:decide(opts.servers or {}, url, document)
        end
        return actions[document]
    end

    -- forget drops what was decided for documents, after a claim.
    function h.forget(documents)
        for __, document in ipairs(documents) do actions[document] = nil end
    end

    -- rows keeps the rows of read whose data goes, deferring the ones that
    -- wait, which are sent once it's decided.
    function h.rows(read)
        local out, deferred = {}, false
        for __, row in ipairs(read) do
            local action = h.action(row.document)
            if Recognition.sendable(action) then
                table.insert(out, row)
            elseif action == "wait" then
                recognition:defer(url, row.document)
                deferred = true
            end
        end
        -- Written before the cursor moves past them, so they're never lost.
        if deferred then recognition:flush() end
        return out
    end

    -- split divides events into the ones that go and the ones this server
    -- never gets. The ones that wait are in neither.
    function h.split(events)
        local go, never = {}, {}
        for __, event in ipairs(events) do
            local action = h.action(event.document)
            if Recognition.sendable(action) then
                table.insert(go, event)
            elseif action ~= "wait" then
                table.insert(never, event)
            end
        end
        return go, never
    end

    -- unrecognized reports whether document goes as unrecognized, or the
    -- server already holds it as that, so what KOReader knows of it, such
    -- as its reading time, stays current there.
    local function unrecognized(document)
        local action = h.action(document)
        if action == "unrecognized" then return true end
        local answer = recognition and recognition:answer(url, document)
        return action == "send" and answer ~= nil and answer.state == "held"
    end

    -- about is what KOReader knows of document, as a server takes it.
    function h.about(document)
        local about = opts.about and opts.about(document) or nil
        if type(about) ~= "table" then about = {} end
        about.document = document
        -- KOReader's JSON writes an empty list as an object, which the
        -- server refuses, so a list with nothing in it is left out.
        for __, key in ipairs({ "authors", "identifiers" }) do
            if type(about[key]) ~= "table" or #about[key] == 0 then about[key] = nil end
        end
        return about
    end

    -- describe adds what KOReader knows of each book payload carries that
    -- goes as unrecognized, unless it already describes them.
    function h.describe(payload)
        if payload.unrecognized then return end
        local seen, list = {}, {}
        local function add(document)
            if seen[document] or not unrecognized(document) then return end
            seen[document] = true
            table.insert(list, h.about(document))
        end
        for __, row in ipairs(payload.pages or {}) do add(row.document) end
        for __, event in ipairs(payload.events or {}) do add(event.document) end
        if #list > 0 then payload.unrecognized = list end
    end

    return h
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
local function sendHistory(opts, send, sent, hold)
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
            local through = Statistics.cursorAfter(server.history.through_at or {})
            through.rowid = server.history.through
            server.stats_cursor = through
            state:flush()
            break
        end
        local going = hold.rows(rows)
        if #going > 0 then
            local payload = { backfill = true, pages = {} }
            for __, row in ipairs(going) do table.insert(payload.pages, payloadRow(row)) end
            local outcome = send(payload)
            if outcome ~= "ok" then return outcome end
        end
        server.stats_cursor = Statistics.cursorAfter(rows[#rows])
        sent.pages = sent.pages + #going
        state:flush()
        if opts.progress then opts.progress(sent.pages) end
    end
    if History.rowsPending(server) then return "ok" end
    local events, never = hold.split(opts.history or {})
    for __, event in ipairs(never) do Summaries.acknowledge(opts.store, event) end
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
sendDeferred sends the rows of each book that waited for every server to
answer, now that it's decided, through send, as Sync.run does: the ones
from history flagged as history. It reads them with
opts.read_document(document, from, through), every row of the book after
the rowid its syncing started from and up to the rowid through. Returns the
outcome, and an error reading statistics.
]]
local function sendDeferred(opts, send, sent, hold)
    local recognition, server = opts.recognition, opts.server
    if not recognition or not opts.read_document then return "ok" end
    local batch = opts.batch or Sync.BATCH
    local through = type(server.stats_cursor) == "table" and tonumber(server.stats_cursor.rowid) or 0
    local from = tonumber(server.stats_from) or 0
    local history_through = type(server.history) == "table" and tonumber(server.history.through) or 0
    for __, document in ipairs(recognition:deferred(server.url)) do
        local action = hold.action(document)
        if Recognition.sendable(action) then
            local rows, err = opts.read_document(document, from, through)
            if not rows then return "error", err end
            for __, history in ipairs({ true, false }) do
                local part = {}
                for __, row in ipairs(rows) do
                    if (row.rowid <= history_through) == history then table.insert(part, row) end
                end
                local first = 1
                while first <= #part do
                    local payload = { pages = {}, backfill = history or nil }
                    for i = first, math.min(first + batch - 1, #part) do
                        table.insert(payload.pages, payloadRow(part[i]))
                    end
                    local outcome = send(payload)
                    if outcome ~= "ok" then return outcome end
                    sent.pages = sent.pages + #payload.pages
                    first = first + batch
                end
            end
            recognition:undefer(server.url, document)
        elseif action ~= "wait" then
            recognition:undefer(server.url, document)
        end
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
- recognition, servers: what each server said of each book
  (shisho/recognition), and the enabled servers, to decide where each
  book's data goes; without them, all of it goes
- documents: the books with data waiting, any of which this server may be
  told to drop
- about(document): what KOReader knows of a book going as unrecognized
- read_document(document, from, through): every statistics row of a book
  after the rowid from and up to the rowid through, for rows that were
  deferred

Returns the outcome, as Responses.classify names it, and the number of
rows and events sent.
]]
function Sync.run(opts)
    local state, journal, server = opts.state, opts.journal, opts.server
    local batch = opts.batch or Sync.BATCH
    local clock = opts.clock or os.time
    local hold = holdBack(opts)
    local changes, never = hold.split(opts.changes or {})
    local next_change = 1
    local sent = { pages = 0, events = 0 }
    -- send posts payload, noting a refusal, which waits for the person.
    local function send(payload)
        hold.describe(payload)
        payload.clock = clock()
        local status, body = opts.post(payload)
        local outcome = Responses.classify(status, body)
        if outcome ~= "ok" then
            -- Changes not taken in are worked out again by the next sync.
            noteRefusal(server, outcome)
            state:flush()
        else
            server.needs_login = nil
            -- The server holds what went as unrecognized now, and each sync
            -- asks again, so a link made elsewhere reaches it.
            if opts.recognition and payload.unrecognized then
                local held = {}
                for __, about in ipairs(payload.unrecognized) do
                    table.insert(held, { document = about.document, state = "held" })
                end
                opts.recognition:record(server.url, held, opts.recognition:current())
            end
        end
        return outcome
    end

    -- A book another server recognizes now is dropped here first, so its
    -- data stops going.
    if opts.recognition then
        local claims = opts.recognition:claims(opts.servers or {}, server.url, opts.documents or {})
        local first = 1
        while first <= #claims do
            local chunk = {}
            for i = first, math.min(first + Sync.CLAIM_BATCH - 1, #claims) do table.insert(chunk, claims[i]) end
            local outcome = send({ claimed = chunk })
            if outcome ~= "ok" then return outcome, sent.pages, sent.events end
            opts.recognition:claimed(server.url, chunk, opts.recognition:current())
            hold.forget(chunk)
            first = first + #chunk
        end
        if #claims > 0 then changes, never = hold.split(opts.changes or {}) end
    end
    -- A book this server holds with nothing KOReader knows of it gets what
    -- KOReader does, even with no reading to send, so the Device's page can
    -- say what it is.
    if opts.recognition then
        -- One KOReader knows nothing of either, such as a book no longer
        -- on the device, isn't sent.
        local wanted = {}
        for __, document in ipairs(opts.recognition:needingDetails(server.url)) do
            local about = hold.about(document)
            if about.title or about.file_type then
                table.insert(wanted, about)
            else
                opts.recognition:cannotDescribe(server.url, document, opts.recognition:current())
            end
        end
        local first = 1
        while first <= #wanted do
            local list = {}
            for i = first, math.min(first + Sync.DETAILS_BATCH - 1, #wanted) do
                table.insert(list, wanted[i])
            end
            local outcome = send({ unrecognized = list })
            if outcome ~= "ok" then return outcome, sent.pages, sent.events end
            first = first + #list
        end
    end
    -- What this server never gets is done with for it.
    for __, change in ipairs(never) do Summaries.acknowledge(opts.store, change) end
    local __, journal_never = hold.split(journal:pending(server.url))
    local never_ids = {}
    for __, event in ipairs(journal_never) do table.insert(never_ids, event.id) end
    journal:acknowledge(server.url, never_ids)

    local outcome, err = sendDeferred(opts, send, sent, hold)
    if outcome ~= "ok" then return outcome, sent.pages, sent.events, err end

    -- Statistics off leaves history rows to go, and reading after it can go
    -- once statistics are on again. Only a sync cut short waits.
    outcome, err = sendHistory(opts, send, sent, hold)
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
        local events = last_rows and journal:pending(server.url, Sync.EVENT_BATCH, function(entry)
            return Recognition.sendable(hold.action(entry.document))
        end) or {}
        local batch_changes = {}
        while last_rows and #events < Sync.EVENT_BATCH and next_change <= #changes do
            table.insert(events, changes[next_change])
            table.insert(batch_changes, changes[next_change])
            next_change = next_change + 1
        end
        if #rows == 0 and #events == 0 then break end

        local going = hold.rows(rows)
        local payload = {}
        if #going > 0 then
            payload.pages = {}
            for __, row in ipairs(going) do table.insert(payload.pages, payloadRow(row)) end
        end
        if #events > 0 then payload.events = events end

        if payload.pages or payload.events then
            outcome = send(payload)
            if outcome ~= "ok" then return outcome, sent.pages, sent.events end
        end

        if #rows > 0 then
            server.stats_cursor = Statistics.cursorAfter(rows[#rows])
        end
        local ids = {}
        for __, event in ipairs(events) do table.insert(ids, event.id) end
        journal:acknowledge(server.url, ids)
        for __, change in ipairs(batch_changes) do
            Summaries.acknowledge(opts.store, change)
        end
        sent.pages = sent.pages + #going
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
removeServer stops using the server at opts.url: it forgets the plugin's own
sign-in, sync state, and queue for it, in opts.state, opts.summaries, and
opts.recognition, and drops the journal entries only it was waiting for.
KOReader's own records (its statistics, sidecars, and OPDS catalog) are
never touched. Returns whether there was such a server.
]]
function Sync.removeServer(opts)
    if not opts.state:remove(opts.url) then return false end
    opts.summaries:remove(opts.url)
    opts.recognition:remove(opts.url)
    opts.journal:prune()
    opts.state:flush()
    opts.summaries:flush()
    opts.recognition:flush()
    return true
end

--[[--
lookUp asks opts.server what it knows of each of opts.documents, in
batches, through opts.post(documents), and records each answer in
opts.recognition in its current round. Returns the outcome: a server that can't
answer doesn't count as having answered, so books no server recognizes wait
for it.
]]
function Sync.lookUp(opts)
    local server, documents = opts.server, opts.documents or {}
    local first = 1
    -- At least once, with nothing to ask about, for the books it holds with
    -- nothing known of them (Recognition:undescribed).
    repeat
        local batch = {}
        for i = first, math.min(first + Recognition.ASK_BATCH - 1, #documents) do
            table.insert(batch, documents[i])
        end
        local status, body = opts.post(batch)
        local outcome = Responses.classify(status, body)
        if outcome ~= "ok" then
            noteRefusal(server, outcome)
            return outcome
        end
        opts.recognition:record(server.url, body.documents, opts.recognition:current())
        opts.recognition:undescribed(server.url, body.undescribed, opts.recognition:current())
        first = first + #batch
    until first > #documents
    return "ok"
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
