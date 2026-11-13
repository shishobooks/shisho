-- Sending page rows and journal events to one server.
local Journal = require("shisho/journal")
local State = require("shisho/state")
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

    ["a sync that just ran isn't repeated"] = function()
        assert_eq(Sync.ranRecently(nil, 1000), false)
        assert_eq(Sync.ranRecently(1000, 1000 + Sync.RAN_RECENTLY - 1), true)
        assert_eq(Sync.ranRecently(1000, 1000 + Sync.RAN_RECENTLY), false)
        assert_eq(Sync.ranRecently(1000, 900), false, "a clock set back since doesn't count as recent")
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
