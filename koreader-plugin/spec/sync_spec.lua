-- Sending page rows and journal events to one server.
local Journal = require("shisho/journal")
local State = require("shisho/state")
local Summaries = require("shisho/summaries")
local Sync = require("shisho/sync")

local function rowsFrom(list)
    -- A reader over list, in the order statistics recorded it, after a cursor.
    return function(cursor, limit)
        local out = {}
        for __, r in ipairs(list) do
            if (not cursor or r.rowid > cursor.rowid) and #out < limit then table.insert(out, r) end
        end
        return out
    end
end

-- row is a statistics row recorded as rowid start / 100, for short.
local function row(start, page)
    return { rowid = math.floor(start / 100), document = "doc1", id_book = 1, page = page or 1, total_pages = 100,
        start_time = start, duration = 30 }
end

local function setup(...)
    local state = State.open(scratch_dir() .. "/shisho.lua")
    for __, url in ipairs({ ... }) do
        state:signIn{ url = url, username = "reader", credential = "C", stats_cursor = { rowid = 0 } }
    end
    return state, Journal.new(state)
end

-- server answers each post with the next status in statuses (200 after
-- they run out), recording what it was sent.
local function server(statuses)
    local sent = {}
    local post = function(payload)
        table.insert(sent, payload)
        local status = table.remove(statuses or {}, 1) or 200
        -- "down" is no answer at all, as when the connection drops.
        if status == "down" then return nil end
        return status, status == 200 and { pages = #(payload.pages or {}), events = {} } or nil
    end
    return post, sent
end

local function starts(payload)
    local out = {}
    for __, r in ipairs(payload.pages or {}) do table.insert(out, r.start_time) end
    return out
end

return {
    ["sends batches and advances the server's cursor as each is acknowledged"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        local post, sent = server()
        local outcome = Sync.run{
            state = state, journal = journal, server = s, post = post, batch = 2,
            read_pages = rowsFrom({ row(100), row(200), row(300) }), clock = function() return 5000 end,
        }
        assert_eq(outcome, "ok")
        assert_eq(#sent, 2)
        assert_eq(starts(sent[1]), { 100, 200 })
        assert_eq(starts(sent[2]), { 300 })
        assert_eq(sent[1].clock, 5000)
        assert_eq(sent[1].pages[1], { document = "doc1", page = 1, total_pages = 100, start_time = 100, duration = 30 })
        assert_eq(s.stats_cursor, { rowid = 3 })
        assert_eq(state:flush(), true)
        assert_eq(State.open(state.path):find("https://a.example").stats_cursor.rowid, 3)
    end,

    ["nothing to send sends nothing"] = function()
        local state, journal = setup("https://a.example")
        local post, sent = server()
        local outcome = Sync.run{ state = state, journal = journal, server = state:find("https://a.example"),
            post = post, read_pages = rowsFrom({}) }
        assert_eq(outcome, "ok")
        assert_eq(#sent, 0)
    end,

    ["an interrupted sync resumes without sending anything twice"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        local rows = rowsFrom({ row(100), row(200), row(300), row(400) })
        local post, sent = server({ 200, "down" })
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post, batch = 2, read_pages = rows }, "unreachable")
        assert_eq(s.stats_cursor.rowid, 2, "the batch that failed isn't counted")

        local post2, sent2 = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post2, batch = 2, read_pages = rows }, "ok")
        assert_eq(starts(sent2[1]), { 300, 400 })
        assert_eq(#sent2, 1)
        assert_eq(#sent, 2)
    end,

    ["a 401 marks the server Needs login and keeps everything queued"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        s.stats_cursor = { rowid = 0 }
        journal:recordPosition("doc1", "7", 0.07, 900)
        local post = server({ 401 })
        local outcome = Sync.run{ state = state, journal = journal, server = s, post = post,
            read_pages = rowsFrom({ row(100) }) }
        assert_eq(outcome, "needs_login")
        assert_eq(s.needs_login, true)
        assert_eq(s.stats_cursor.rowid, 0)
        assert_eq(#journal:pending("https://a.example"), 1)
    end,

    ["events go with the pages and are acknowledged per server"] = function()
        local state, journal = setup("https://a.example", "https://b.example")
        journal:recordPosition("doc1", "/body/p[2]", 0.5, 900)
        local post, sent = server()
        Sync.run{ state = state, journal = journal, server = state:find("https://a.example"), post = post,
            read_pages = rowsFrom({}) }
        assert_eq(#sent, 1)
        assert_eq(sent[1].pages, nil, "an empty list isn't sent")
        assert_eq(sent[1].events[1].progress, "/body/p[2]")
        assert_eq(sent[1].events[1].at, 900)
        assert_eq(#journal:pending("https://a.example"), 0)
        assert_eq(#journal:pending("https://b.example"), 1)

        local post2, sent2 = server()
        Sync.run{ state = state, journal = journal, server = state:find("https://b.example"), post = post2,
            read_pages = rowsFrom({}) }
        assert_eq(#sent2, 1)
        assert_eq(#state:get("journal").entries, 0, "dropped once every server has it")
    end,

    ["without statistics, positions still go"] = function()
        local state, journal = setup("https://a.example")
        journal:recordPosition("doc1", "2", 0.02, 900)
        local post, sent = server()
        local outcome = Sync.run{ state = state, journal = journal, server = state:find("https://a.example"),
            post = post, read_pages = nil }
        assert_eq(outcome, "ok")
        assert_eq(#sent[1].events, 1)
    end,

    ["a server signed in for the first time starts at the sign-in"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        local s = state:signIn{ url = "https://a.example", username = "reader", credential = "C", stats_cursor = { rowid = 42 } }
        assert_eq(s.stats_cursor, { rowid = 42 })
        s.stats_cursor.rowid = 50
        state:signIn{ url = "https://a.example", username = "reader", credential = "D", stats_cursor = { rowid = 99 } }
        assert_eq(s.stats_cursor.rowid, 50, "signing in again keeps its place")
    end,

    ["history goes first, flagged, then its statuses, then reading after sign-in"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        s.history = { through = 3 }
        journal:recordPosition("doc1", "9", 0.9, 900)
        local history = { { id = "h1", type = "status", document = "doc1", status = "complete", day = "2025-01-01" } }
        local store = { synced = { doc1 = { status = "complete", before = true } }, asked = {} }
        local post, sent = server()
        local progress = {}
        local outcome = Sync.run{ state = state, journal = journal, server = s, post = post, batch = 2,
            read_pages = rowsFrom({ row(100), row(200), row(300), row(400), row(500) }),
            history = history, store = store, progress = function(n) table.insert(progress, n) end }
        assert_eq(outcome, "ok")
        assert_eq(#sent, 5)
        assert_eq(sent[1].backfill, true)
        assert_eq(starts(sent[1]), { 100, 200 })
        assert_eq(sent[2].backfill, true)
        assert_eq(starts(sent[2]), { 300 }, "a batch of history ends where sign-in left off")
        assert_eq(sent[3].backfill, true)
        assert_eq(sent[3].pages, nil)
        assert_eq(sent[3].events, history, "statuses follow every page of history")
        assert_eq(sent[4].backfill, nil)
        assert_eq(starts(sent[4]), { 400, 500 })
        assert_eq(sent[5].events[1].type, "position")
        assert_eq(progress, { 2, 3 })
        assert_eq(store.synced.doc1, { status = "complete" }, "synced once taken in")
        assert_eq(s.stats_cursor.rowid, 5)
    end,

    ["history cut off partway resumes, and its statuses wait for every page"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        s.history = { through = 3 }
        local rows = rowsFrom({ row(100), row(200), row(300) })
        local history = { { id = "h1", type = "status", document = "doc1", status = "complete", day = "2025-01-01" } }
        local store = { synced = { doc1 = { status = "complete", before = true } }, asked = {} }
        local post, sent = server({ 200, "down" })
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post, batch = 2, read_pages = rows,
            history = history, store = store }, "unreachable")
        assert_eq(#sent, 2)
        assert_eq(s.stats_cursor.rowid, 2)
        assert_eq(store.synced.doc1.before, true, "its statuses wait")

        local post2, sent2 = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post2, batch = 2, read_pages = rows,
            history = history, store = store }, "ok")
        assert_eq(starts(sent2[1]), { 300 })
        assert_eq(sent2[1].backfill, true)
        assert_eq(sent2[2].events, history)
        assert_eq(#sent2, 2)
    end,

    ["with statistics off, history statuses wait for its pages"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        s.history = { through = 3 }
        local history = { { id = "h1", type = "status", document = "doc1", status = "complete", day = "2025-01-01" } }
        local store = { synced = { doc1 = { status = "complete", before = true } }, asked = {} }
        local post, sent = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post, read_pages = nil,
            history = history, store = store }, "ok")
        assert_eq(#sent, 0)
        assert_eq(store.synced.doc1.before, true, "sent once statistics are on and its pages have gone")
    end,

    ["a sync while busy sends a little history at a time, and nothing after it"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        s.history = { through = 5 }
        journal:recordPosition("doc1", "9", 0.9, 900)
        local rows = rowsFrom({ row(100), row(200), row(300), row(400), row(500), row(600) })
        local post, sent = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post, batch = 2, read_pages = rows,
            history_batches = 2 }, "ok")
        assert_eq(#sent, 2)
        assert_eq(starts(sent[2]), { 300, 400 })
        assert_eq(s.last_synced, nil, "the rest waits for the next sync")
        assert_eq(#journal:pending("https://a.example"), 1)

        local post2, sent2 = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post2, batch = 2, read_pages = rows,
            history_batches = 2 }, "ok")
        assert_eq(starts(sent2[1]), { 500 })
        assert_eq(sent2[1].backfill, true)
        assert_eq(starts(sent2[2]), { 600 })
        assert_eq(sent2[2].backfill, nil)
        assert_eq(#journal:pending("https://a.example"), 0)
    end,

    ["rows statistics leaves out up to sign-in don't hold history up"] = function()
        local state, journal = setup("https://a.example")
        local s = state:find("https://a.example")
        s.history = { through = 5 }
        local post, sent = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post, batch = 2,
            read_pages = rowsFrom({ row(100), row(200) }) }, "ok")
        assert_eq(#sent, 1)
        assert_eq(s.stats_cursor.rowid, 5)
    end,

    ["positions go only with the last batch, after every page row"] = function()
        local state, journal = setup("https://a.example")
        journal:recordPosition("doc1", "9", 0.9, 900)
        local post, sent = server()
        Sync.run{ state = state, journal = journal, server = state:find("https://a.example"), post = post, batch = 2,
            read_pages = rowsFrom({ row(100), row(200), row(300), row(400) }) }
        assert_eq(#sent, 3)
        assert_eq(sent[1].events, nil)
        assert_eq(sent[2].events, nil)
        assert_eq(starts(sent[3]), {})
        assert_eq(#sent[3].events, 1, "the position follows the rows")
        assert_eq(#journal:pending("https://a.example"), 0)
    end,

    ["status and Rating changes follow the positions, and are synced once taken in"] = function()
        local state, journal = setup("https://a.example")
        journal:recordPosition("doc1", "9", 0.9, 900)
        local changes = {
            { id = "c1", type = "status", document = "doc1", status = "complete", previous_status = "reading", day = "2026-11-12" },
            { id = "c2", type = "rating", document = "doc2", rating = 4, previous_rating = 0, at = 950 },
        }
        local store = { synced = { doc1 = { status = "reading" } }, asked = { doc1 = true, doc2 = true } }
        local post, sent = server()
        local outcome = Sync.run{ state = state, journal = journal, server = state:find("https://a.example"), post = post,
            read_pages = rowsFrom({ row(100) }), changes = changes, store = store }
        assert_eq(outcome, "ok")
        assert_eq(#sent, 1)
        assert_eq(sent[1].events[1].type, "position")
        assert_eq(sent[1].events[2], changes[1])
        assert_eq(sent[1].events[3], changes[2])
        assert_eq(store.synced, { doc1 = { status = "complete" }, doc2 = { rating = 4 } })
        assert_eq(store.asked, {}, "asked about again, in case the server kept its own")
    end,

    ["a change the server didn't take in stays unsynced"] = function()
        local state, journal = setup("https://a.example")
        local store = { synced = { doc1 = { status = "reading" } }, asked = { doc1 = true } }
        local post = server({ 500 })
        local outcome = Sync.run{ state = state, journal = journal, server = state:find("https://a.example"), post = post,
            changes = { { id = "c1", type = "status", document = "doc1", status = "complete", previous_status = "reading" } },
            store = store }
        assert_eq(outcome, "error")
        assert_eq(store.synced.doc1.status, "reading", "the next sync sends it again")
    end,

    ["pulling states asks about new books outright, then what changed since"] = function()
        local state = setup("https://a.example")
        local s = state:find("https://a.example")
        local store = { synced = {}, asked = { old = true } }
        local payloads, written = {}, {}
        local function post(payload)
            table.insert(payloads, payload)
            return 200, { states = { { document = "new", status = "complete" }, { document = "changed", status = "reading" } }, cursor = 7000 }
        end
        local outcome = Sync.pullStates{ server = s, store = store, documents = { "old", "new", "unknown" }, clock = function() return 900 end,
            post = post, write = function(pulled, outright) table.insert(written, { pulled.document, outright }) end }
        assert_eq(outcome, "ok")
        assert_eq(payloads, { { statuses = { "new", "unknown" } } })
        assert_eq(written, { { "new", true }, { "changed", false } })
        assert_eq(store.asked, { old = true, new = true, unknown = 900 }, "one it didn't recognize is asked about later")
        assert_eq(s.states_cursor, 7000)

        Sync.pullStates{ server = s, store = store, documents = { "old", "new", "unknown" }, clock = function() return 1000 end,
            post = post, write = function() end }
        assert_eq(payloads[2], { since = 7000 }, "only what changed")
    end,

    ["pulling states asks about a large library in batches"] = function()
        local state = setup("https://a.example")
        local s = state:find("https://a.example")
        local store = { synced = {}, asked = {} }
        local documents = {}
        for i = 1, Summaries.ASK_BATCH + 3 do table.insert(documents, string.format("doc%04d", i)) end
        local asked_in = {}
        local function post(payload)
            table.insert(asked_in, #(payload.statuses or {}))
            local states = {}
            for __, document in ipairs(payload.statuses or {}) do table.insert(states, { document = document }) end
            return 200, { states = states, cursor = 7000 }
        end
        assert_eq(Sync.pullStates{ server = s, store = store, documents = documents, post = post, write = function() end }, "ok")
        assert_eq(asked_in, { Summaries.ASK_BATCH, 3 })
        assert_eq(store.asked[documents[#documents]], true)
    end,

    ["pulling states that fails asks again next time"] = function()
        local state = setup("https://a.example")
        local s = state:find("https://a.example")
        local store = { synced = {}, asked = {} }
        local outcome = Sync.pullStates{ server = s, store = store, documents = { "new" },
            post = function() return 401 end, write = function() end }
        assert_eq(outcome, "needs_login")
        assert_eq(s.needs_login, true)
        assert_eq(store.asked, {})
        assert_eq(s.states_cursor, nil)
    end,

    ["a sync that just ran isn't repeated"] = function()
        assert_eq(Sync.ranRecently(nil, 1000), false)
        assert_eq(Sync.ranRecently(1000, 1000 + Sync.RAN_RECENTLY - 1), true)
        assert_eq(Sync.ranRecently(1000, 1000 + Sync.RAN_RECENTLY), false)
        assert_eq(Sync.ranRecently(1000, 900), false, "a clock set back since doesn't count as recent")
    end,

    ["reading with Wi-Fi on syncs every 20 pages"] = function()
        assert_eq(Sync.dueAfterPages(nil), false)
        assert_eq(Sync.dueAfterPages(Sync.PAGES_BETWEEN - 1), false)
        assert_eq(Sync.dueAfterPages(Sync.PAGES_BETWEEN), true)
        assert_eq(Sync.PAGES_BETWEEN, 20)
    end,

    ["only enabled servers that can take a sync are syncable"] = function()
        local servers = {
            { url = "a" }, { url = "b", disabled = true }, { url = "c", needs_login = true },
            { url = "d", update_required = true },
        }
        local syncable = Sync.syncable(servers)
        assert_eq(#syncable, 1)
        assert_eq(syncable[1].url, "a")
    end,

    ["going to sleep syncs online, or turns Wi-Fi on only when all of it allows"] = function()
        local now = 100000
        local base = { online = false, syncable = 1, sync_on_sleep = true, wifi_turns_on = true, now = now }
        local function with(changes)
            local o = {}
            for k, v in pairs(base) do o[k] = v end
            for k, v in pairs(changes) do o[k] = v end
            return o
        end
        assert_eq(Sync.onSleep(with{ online = true, sync_on_sleep = false }), "sync", "Wi-Fi already on costs nothing")
        assert_eq(Sync.onSleep(with{}), "wifi")
        assert_eq(Sync.onSleep(with{ syncable = 0 }), nil, "no server could take it")
        assert_eq(Sync.onSleep(with{ syncable = 0, online = true }), nil)
        assert_eq(Sync.onSleep(with{ sync_on_sleep = false }), nil)
        assert_eq(Sync.onSleep(with{ wifi_turns_on = false }), nil, "KOReader would ask")
        assert_eq(Sync.onSleep(with{ last_synced = now - 60 }), nil, "synced within the hour")
        assert_eq(Sync.onSleep(with{ last_attempt = now - 60 }), nil, "tried within the hour, even if it failed")
        assert_eq(Sync.onSleep(with{ last_synced = now - Sync.SLEEP_GAP, last_attempt = now - Sync.SLEEP_GAP }), "wifi")
    end,

    ["a server that just failed is skipped by syncs while reading for a while"] = function()
        local server = { url = "a" }
        local now = 50000
        assert_eq(Sync.backingOff(server, now), false)
        Sync.noteOutcome(server, "unreachable", now)
        assert_eq(Sync.backingOff(server, now + 60), true)
        assert_eq(Sync.backingOff(server, now + Sync.BACKOFF), false)
        assert_eq(Sync.backingOff(server, now - 60), false, "a clock set back since doesn't hold it off")
        Sync.noteOutcome(server, "error", now)
        assert_eq(Sync.backingOff(server, now + 1), true)
        Sync.noteOutcome(server, "ok", now + 2)
        assert_eq(Sync.backingOff(server, now + 3), false, "a success clears it")
        Sync.noteOutcome(server, "needs_login", now + 4)
        assert_eq(Sync.backingOff(server, now + 5), false, "a refusal isn't the network")
    end,

    ["a page turn syncs only on a connection, and otherwise starts the count again"] = function()
        assert_eq(Sync.afterPage(Sync.PAGES_BETWEEN - 1, true), nil)
        assert_eq(Sync.afterPage(Sync.PAGES_BETWEEN, true), "sync")
        assert_eq(Sync.afterPage(Sync.PAGES_BETWEEN, false), "reset", "no retry on every page while offline")
    end,

    ["Wi-Fi coming on for a sync while away does nothing more"] = function()
        local base = { servers = 1, background = false, screen_saver = false, recent_check = false, ran_recently = false, awaiting_restart = false }
        local function with(changes)
            local o = {}
            for k, v in pairs(base) do o[k] = v end
            for k, v in pairs(changes) do o[k] = v end
            return o
        end
        assert_eq(Sync.onConnected(with{}), { check = true, sync = true })
        assert_eq(Sync.onConnected(with{ background = true }), { check = false, sync = false })
        assert_eq(Sync.onConnected(with{ screen_saver = true }), { check = false, sync = false })
        assert_eq(Sync.onConnected(with{ recent_check = true }), { check = false, sync = true })
        assert_eq(Sync.onConnected(with{ ran_recently = true }), { check = true, sync = false })
        assert_eq(Sync.onConnected(with{ servers = 0 }), { check = false, sync = false })
        assert_eq(Sync.onConnected(with{ awaiting_restart = true }), { check = false, sync = false })
    end,

    ["sleep syncs only once the last sync is an hour old"] = function()
        local servers = { { url = "a", last_synced = 1000 }, { url = "b", last_synced = 5000 }, { url = "c" } }
        assert_eq(Sync.lastSynced(servers), 5000, "the latest of any server")
        assert_eq(Sync.lastSynced({ { url = "c" } }), nil)

        assert_eq(Sync.dueOnSleep(nil, 1000), true, "never synced")
        assert_eq(Sync.dueOnSleep(5000, 5000 + Sync.SLEEP_GAP - 1), false)
        assert_eq(Sync.dueOnSleep(5000, 5000 + Sync.SLEEP_GAP), true)
        assert_eq(Sync.dueOnSleep(5000, 4000), true, "a clock set back since can't hold it off")
    end,
}
