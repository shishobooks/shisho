--[[--
Sends one server everything it hasn't acknowledged (ADR 0016): page rows
from KOReader's statistics after the server's cursor, and journal entries it
hasn't acknowledged, in bounded batches. Each batch is acknowledged whole,
and only then does the cursor move and the entries count as sent, so a sync
cut off partway resumes without sending anything twice.

Journal entries go only with the last batch of rows. The server places a
session by its time and lets a position start a Read-through, so a position
from the end of a long offline stretch must not arrive before the sessions
that led up to it. No UI.
]]
local Responses = require("shisho/responses")
local Statistics = require("shisho/statistics")

local Sync = {}

-- The most rows and events one upload carries.
Sync.BATCH = 500
Sync.EVENT_BATCH = 100

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
run syncs opts.server. opts:

- state, journal: the plugin's state and journal
- post(payload): sends an upload, returning the status and decoded answer
- read_pages(cursor, limit): the statistics reader, or nil when statistics
  is off, in which case only the journal goes
- clock(): the device's clock, os.time by default
- batch: rows per upload

Returns the outcome, as Responses.classify names it, and the number of
rows and events sent.
]]
function Sync.run(opts)
    local state, journal, server = opts.state, opts.journal, opts.server
    local batch = opts.batch or Sync.BATCH
    local clock = opts.clock or os.time
    local sent_pages, sent_events = 0, 0
    while true do
        local rows = {}
        if opts.read_pages then
            local read, err = opts.read_pages(server.stats_cursor, batch)
            if not read then return "error", sent_pages, sent_events, err end
            rows = read
        end
        -- Positions follow every row, so they go only once the rows run out.
        local last_rows = #rows < batch
        local events = last_rows and journal:pending(server.url, Sync.EVENT_BATCH) or {}
        if #rows == 0 and #events == 0 then break end

        local payload = { clock = clock() }
        if #rows > 0 then
            payload.pages = {}
            for __, row in ipairs(rows) do table.insert(payload.pages, payloadRow(row)) end
        end
        if #events > 0 then payload.events = events end

        local status, body = opts.post(payload)
        local outcome = Responses.classify(status, body)
        if outcome ~= "ok" then
            if outcome == "needs_login" then
                server.needs_login = true
            elseif outcome == "update_required" then
                server.update_required = true
            end
            state:flush()
            return outcome, sent_pages, sent_events
        end

        if #rows > 0 then
            server.stats_cursor = Statistics.cursorAfter(rows[#rows])
        end
        local ids = {}
        for __, event in ipairs(events) do table.insert(ids, event.id) end
        journal:acknowledge(server.url, ids)
        server.needs_login = nil
        sent_pages = sent_pages + #rows
        sent_events = sent_events + #events
        state:flush()
        if last_rows and #events < Sync.EVENT_BATCH then break end
    end
    server.last_synced = clock()
    journal:prune()
    state:flush()
    return "ok", sent_pages, sent_events
end

return Sync
