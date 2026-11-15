-- Sending page rows and journal events to one server.
local Journal = require("shisho/journal")
local Recognition = require("shisho/recognition")
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
        assert_eq(s.stats_cursor, { rowid = 3, id_book = 1, page = 1, start_time = 300 }, "naming the row it's past")
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
        s.history = { through = 5, through_at = { id_book = 1, page = 9, start_time = 500 } }
        local post, sent = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = s, post = post, batch = 2,
            read_pages = rowsFrom({ row(100), row(200) }) }, "ok")
        assert_eq(#sent, 1)
        assert_eq(s.stats_cursor, { rowid = 5, id_book = 1, page = 9, start_time = 500 }, "naming history's last row")
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

    ["each server gets only the books it recognizes, and rows that wait are sent once it's decided"] = function()
        local state, journal = setup("https://a.example", "https://b.example")
        local a, b = state:find("https://a.example"), state:find("https://b.example")
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        local function at(start, document) local r = row(start); r.document = document; return r end
        local list = { at(100, "a-book"), at(200, "b-book"), at(300, "old") }
        recognition:noteSeen({ "a-book", "b-book", "old" }, 50)
        -- B is down: only A answered.
        recognition:record(a.url, {
            { document = "a-book", state = "recognized" },
            { document = "b-book", state = "unknown" },
            { document = "old", state = "unknown" },
        }, 50)
        journal:recordPosition("old", "5", 0.05, 900)
        local opts = function(post)
            return {
                state = state, journal = journal, server = a, post = post, read_pages = rowsFrom(list),
                recognition = recognition, servers = state:enabledServers(),
                documents = { "a-book", "b-book", "old" },
                about = function(document) return { title = "Title of " .. document, authors = {}, identifiers = { "isbn:1" } } end,
                read_document = function(document, from, through)
                    local out = {}
                    for __, r in ipairs(list) do
                        if r.document == document and r.rowid > from and r.rowid <= through then table.insert(out, r) end
                    end
                    return out
                end,
            }
        end

        local post, sent = server()
        assert_eq(Sync.run(opts(post)), "ok")
        assert_eq(#sent, 1)
        assert_eq(starts(sent[1]), { 100 }, "only A's own book")
        assert_eq(sent[1].events, nil, "the old copy's position waits too")
        assert_eq(sent[1].unrecognized, nil)
        assert_eq(a.stats_cursor.rowid, 3, "the cursor moves past rows that wait")
        assert_eq(recognition:deferred(a.url), { "b-book", "old" })
        assert_eq(#journal:pending(a.url), 1)

        -- B comes back: it recognizes its book, and not the old copy.
        recognition:record(b.url, {
            { document = "a-book", state = "unknown" },
            { document = "b-book", state = "recognized" },
            { document = "old", state = "unknown" },
        }, 60)
        local post2, sent2 = server()
        assert_eq(Sync.run(opts(post2)), "ok")
        assert_eq(#sent2, 2)
        assert_eq(starts(sent2[1]), { 300 }, "the old copy's rows, deferred, go now")
        assert_eq(sent2[1].unrecognized, { { document = "old", title = "Title of old", identifiers = { "isbn:1" } } },
            "an empty list is left out, since KOReader's JSON would write it as an object")
        assert_eq(sent2[2].events[1].document, "old")
        assert_eq(recognition:answer(a.url, "old").state, "held", "A holds it now, so each sync asks again")
        assert_eq(recognition:held(), { "old" })
        assert_eq(recognition:deferred(a.url), {}, "B's book is never A's")
        assert_eq(#journal:pending(a.url), 0)
    end,

    ["a status or Rating change for a book no server recognizes waits for every server, as positions do"] = function()
        local state, journal = setup("https://a.example", "https://b.example")
        local a, b = state:find("https://a.example"), state:find("https://b.example")
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        recognition:noteSeen({ "old" }, 50)
        -- B is down: only A answered, and it doesn't know the book.
        recognition:record(a.url, { { document = "old", state = "unknown" } }, 50)
        local changes = {
            { id = "c1", type = "status", document = "old", status = "complete", day = "2026-11-12" },
            { id = "c2", type = "rating", document = "old", rating = 4, previous_rating = 0, at = 950 },
        }
        local store = { synced = {}, asked = {} }
        local opts = function(post)
            return {
                state = state, journal = journal, server = a, post = post, changes = changes, store = store,
                recognition = recognition, servers = state:enabledServers(), documents = { "old" },
                about = function(document) return { title = "Title of " .. document } end,
            }
        end

        local post, sent = server()
        assert_eq(Sync.run(opts(post)), "ok")
        assert_eq(#sent, 0, "neither goes")
        assert_eq(store.synced, {}, "nor is either taken as synced")

        -- B comes back, and doesn't know it either: both go to A, with the
        -- book as unrecognized.
        recognition:record(b.url, { { document = "old", state = "unknown" } }, 60)
        local post2, sent2 = server()
        assert_eq(Sync.run(opts(post2)), "ok")
        assert_eq(#sent2, 1)
        assert_eq(sent2[1].events, changes)
        assert_eq(sent2[1].unrecognized, { { document = "old", title = "Title of old" } })
        assert_eq(store.synced, { old = { status = "complete", rating = 4 } })
    end,

    ["a server holding a book another now recognizes is told to drop it"] = function()
        local state, journal = setup("https://a.example", "https://b.example")
        local a, b = state:find("https://a.example"), state:find("https://b.example")
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        recognition:record(a.url, { { document = "old", state = "recognized" } }, 100)
        recognition:record(b.url, { { document = "old", state = "held" } }, 100)
        journal:recordPosition("old", "5", 0.05, 900)
        local change = { id = "c1", type = "status", document = "old", status = "complete", day = "2026-01-01" }
        local store = { synced = {}, asked = {} }
        local post, sent = server()
        local r = function() return row(100) end
        local rowOld = r(); rowOld.document = "old"
        assert_eq(Sync.run{
            state = state, journal = journal, server = b, post = post, read_pages = rowsFrom({ rowOld }),
            recognition = recognition, servers = state:enabledServers(), documents = { "old" },
            changes = { change }, store = store,
        }, "ok")
        assert_eq(#sent, 1)
        assert_eq(sent[1].claimed, { "old" })
        assert_eq(sent[1].pages, nil)
        assert_eq(recognition:decide(state:enabledServers(), b.url, "old"), "skip")
        assert_eq(#journal:pending(b.url), 0, "its position is done with for B")
        assert_eq(store.synced.old.status, "complete", "and so is its status")
        assert_eq(b.stats_cursor.rowid, 1)

        local post2, sent2 = server()
        Sync.run{ state = state, journal = journal, server = b, post = post2, read_pages = rowsFrom({ rowOld }),
            recognition = recognition, servers = state:enabledServers(), documents = { "old" } }
        assert_eq(#sent2, 0, "claimed once")
    end,

    ["looking books up records each server's answers, and a refusal leaves them unanswered"] = function()
        local state = setup("https://a.example")
        local a = state:find("https://a.example")
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        for _ = 1, 70 do recognition:beginRound() end
        local asked = {}
        local outcome = Sync.lookUp{
            server = a, recognition = recognition, documents = { "one", "two" },
            post = function(documents)
                table.insert(asked, documents)
                return 200, { documents = { { document = "one", state = "recognized" }, { document = "two", state = "held" } } }
            end,
        }
        assert_eq(outcome, "ok")
        assert_eq(asked, { { "one", "two" } })
        assert_eq(recognition:answer(a.url, "one"), { state = "recognized", at = 70 })
        assert_eq(recognition:answer(a.url, "two").state, "held")

        local refused = Sync.lookUp{ server = a, recognition = recognition, documents = { "three" },
            post = function() return 401 end }
        assert_eq(refused, "needs_login")
        assert_eq(a.needs_login, true)
        assert_eq(recognition:answer(a.url, "three"), nil)
    end,

    ["removing a server forgets only the plugin's own state and queue for it"] = function()
        local state, journal = setup("https://a.example", "https://b.example")
        local dir = scratch_dir()
        local summaries = Summaries.open(dir .. "/summaries.lua")
        local recognition = Recognition.open(dir .. "/recognition.lua")
        summaries:server("https://a.example").synced.doc1 = { status = "complete" }
        summaries:server("https://b.example").synced.doc1 = { status = "reading" }
        recognition:record("https://a.example", { { document = "doc1", state = "held" } }, 10)
        recognition:record("https://b.example", { { document = "doc1", state = "recognized" } }, 10)
        journal:recordPosition("doc1", "5", 0.05, 900)
        journal:acknowledge("https://a.example", { journal:pending("https://a.example")[1].id })
        -- KOReader's own statistics, which removing never touches.
        local stats = dir .. "/statistics.sqlite3"
        local f = io.open(stats, "w"); f:write("koreader's"); f:close()

        assert_eq(Sync.removeServer{ url = "https://b.example", state = state, journal = journal,
            summaries = summaries, recognition = recognition }, true)

        assert_eq(#state:servers(), 1)
        assert_eq(state:find("https://b.example"), nil)
        assert_eq(State.open(state.path):find("https://b.example"), nil, "written")
        assert_eq(Summaries.open(dir .. "/summaries.lua").data.servers["https://b.example"], nil)
        assert_eq(summaries:server("https://a.example").synced.doc1.status, "complete")
        assert_eq(Recognition.open(dir .. "/recognition.lua"):answer("https://b.example", "doc1"), nil)
        assert_eq(recognition:answer("https://a.example", "doc1").state, "held")
        assert_eq(#state:get("journal").entries, 0, "nothing waits for a removed server")
        f = io.open(stats, "r"); assert_eq(f:read("*a"), "koreader's"); f:close()
        assert_eq(Sync.removeServer{ url = "https://b.example", state = state, journal = journal,
            summaries = summaries, recognition = recognition }, false)
    end,

    ["claims go in batches, and a book a server holds keeps what KOReader knows of it current"] = function()
        local state, journal = setup("https://a.example", "https://b.example")
        local a, b = state:find("https://a.example"), state:find("https://b.example")
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        local documents, answers = {}, {}
        for i = 1, Sync.CLAIM_BATCH + 1 do
            local document = string.format("doc%04d", i)
            table.insert(documents, document)
            table.insert(answers, { document = document, state = "held" })
        end
        recognition:record(b.url, answers, 1)
        local recognized = {}
        for __, document in ipairs(documents) do table.insert(recognized, { document = document, state = "recognized" }) end
        recognition:record(a.url, recognized, 1)
        local post, sent = server()
        assert_eq(Sync.run{ state = state, journal = journal, server = b, post = post, read_pages = rowsFrom({}),
            recognition = recognition, servers = state:enabledServers(), documents = documents }, "ok")
        assert_eq(#sent, 2)
        assert_eq(#sent[1].claimed, Sync.CLAIM_BATCH)
        assert_eq(sent[2].claimed, { documents[#documents] })

        -- A book only this server holds goes with what KOReader knows of it,
        -- so its reading time there stays current.
        local c = Recognition.open(scratch_dir() .. "/recognition.lua")
        c:record(a.url, { { document = "old", state = "held" } }, 1)
        local r = row(100); r.document = "old"
        local post2, sent2 = server()
        Sync.run{ state = state, journal = journal, server = a, post = post2, read_pages = rowsFrom({ r }),
            recognition = c, servers = state:enabledServers(), documents = { "old" },
            about = function() return { title = "Old", reading_seconds = 90 } end }
        assert_eq(sent2[1].unrecognized, { { document = "old", title = "Old", reading_seconds = 90 } })
    end,

    ["deferred rows start where the server's syncing did"] = function()
        local state, journal = setup("https://a.example")
        local a = state:find("https://a.example")
        a.stats_from = 1
        a.stats_cursor = { rowid = 2 }
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        recognition:defer(a.url, "old")
        recognition:noteSeen({ "old" }, recognition:beginRound())
        recognition:record(a.url, { { document = "old", state = "unknown" } }, recognition:current())
        local asked
        local post, sent = server()
        Sync.run{ state = state, journal = journal, server = a, post = post, read_pages = rowsFrom({}),
            recognition = recognition, servers = state:enabledServers(), documents = { "old" },
            read_document = function(document, from, through)
                asked = { document, from, through }
                local r = row(200); r.document = "old"
                return { r }
            end }
        assert_eq(asked, { "old", 1, 2 }, "rows before sign-in, not sent as history, stay out")
        assert_eq(starts(sent[1]), { 200 })
        assert_eq(recognition:deferred(a.url), {})
    end,

    ["a book a server holds with nothing known of it gets its details, once, with nothing else to send"] = function()
        local state, journal = setup("https://a.example")
        local a = state:find("https://a.example")
        local recognition = Recognition.open(scratch_dir() .. "/recognition.lua")
        -- A lookup asking about nothing still says which it holds bare.
        local asked = {}
        assert_eq(Sync.lookUp{ server = a, recognition = recognition, documents = {},
            post = function(documents)
                table.insert(asked, documents)
                return 200, { documents = {}, undescribed = { "bare" } }
            end }, "ok")
        assert_eq(asked, { {} })
        recognition:record(a.url, { { document = "known", state = "held" } }, recognition:current())
        recognition:undescribed(a.url, { "gone" }, recognition:current())
        assert_eq(recognition:needingDetails(a.url), { "bare", "gone" })
        local opts = function(post)
            return { state = state, journal = journal, server = a, post = post, read_pages = rowsFrom({}),
                recognition = recognition, servers = state:enabledServers(), documents = {},
                about = function(document)
                    -- KOReader knows nothing of a book no longer on the device.
                    if document == "gone" then return {} end
                    return { title = "Title of " .. document, authors = {} }
                end }
        end

        local post, sent = server()
        assert_eq(Sync.run(opts(post)), "ok")
        assert_eq(#sent, 1)
        assert_eq(sent[1].unrecognized, { { document = "bare", title = "Title of bare" } })
        assert_eq(sent[1].pages, nil)
        assert_eq(sent[1].events, nil)
        assert_eq(recognition:answer(a.url, "bare").needs_details, nil, "described")

        local post2, sent2 = server()
        Sync.run(opts(post2))
        assert_eq(#sent2, 0, "sent once")
    end,
}
