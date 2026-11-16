-- Sending the history KOReader recorded before the plugin signed in.
local History = require("shisho/history")
local State = require("shisho/state")
local Summaries = require("shisho/summaries")

local function strip(events)
    -- The ids are random; the rest is what's compared.
    for __, event in ipairs(events) do
        assert(type(event.id) == "string" and #event.id > 0, "each event has an id")
        event.id = nil
    end
    return events
end

return {
    ["the setting asks by default"] = function()
        assert_eq(History.setting(nil), "ask")
        assert_eq(History.setting({}), "ask")
        assert_eq(History.setting({ send_history = "send" }), "send")
        assert_eq(History.setting({ send_history = "skip" }), "skip")
        assert_eq(History.setting({ send_history = "bogus" }), "ask")
    end,

    ["only a first sign-in to a server asks, as the setting says"] = function()
        assert_eq(History.onSignIn("ask", true), "ask")
        assert_eq(History.onSignIn("send", true), "send")
        assert_eq(History.onSignIn("skip", true), nil)
        assert_eq(History.onSignIn("ask", false), nil, "signing in again keeps what was chosen")
        assert_eq(History.onSignIn("send", false), nil)
    end,

    ["a first sign-in sends history or asks, as the setting says, and waits for an answer"] = function()
        local sent = { stats_cursor = { rowid = 42 } }
        History.signIn(sent, "send", true)
        assert_eq(sent.history, { through = 42 })
        assert_eq(sent.asking_history, nil)

        local asked = { url = "https://b.example", stats_cursor = { rowid = 42 } }
        History.signIn(asked, "ask", true)
        assert_eq(asked.history, nil)
        assert_eq(asked.asking_history, true)
        local other = { url = "https://c.example", asking_history = true }
        assert_eq(History.waiting({ sent, asked, other }), asked)

        History.answer(asked, false)
        assert_eq(asked.asking_history, nil)
        assert_eq(asked.history, nil, "declined: only reading from sign-in on")
        assert_eq(asked.stats_cursor, { rowid = 42 })
        assert_eq(History.waiting({ sent, asked, other }), other, "the next server still asking is asked next")
        History.answer(other, true)
        assert_eq(other.history, { through = 0 })
        assert_eq(History.waiting({ sent, asked, other }), nil)

        local again = { stats_cursor = { rowid = 7 } }
        History.signIn(again, "ask", false)
        assert_eq(again.asking_history, nil, "signing in again doesn't ask")
    end,

    ["sending history reads from the first row up to where sign-in left off"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        local server, _other, first = state:signIn{ url = "https://a.example", username = "r", credential = "C",
            stats_cursor = { rowid = 42, id_book = 3, page = 7, start_time = 900 } }
        assert_eq(first, true)
        assert_eq(server.stats_from, 42)
        assert_eq(server.stats_from_row, { id_book = 3, page = 7, start_time = 900 }, "naming the row syncing starts after")
        History.accept(server)
        assert_eq(server.history, { through = 42, through_row = { id_book = 3, page = 7, start_time = 900, latest = 900 } })
        assert_eq(server.stats_from_row, nil)
        assert_eq(server.stats_cursor, { rowid = 0 })
        assert_eq(History.rowsPending(server), true)

        -- Some of it sent, then accepted again: nothing starts over.
        server.stats_cursor = { rowid = 10 }
        History.accept(server)
        assert_eq(server.stats_cursor, { rowid = 10 })
        server.stats_cursor = { rowid = 42 }
        assert_eq(History.rowsPending(server), false)

        local _same, _other2, again = state:signIn{ url = "https://a.example", username = "r", credential = "D", stats_cursor = { rowid = 99 } }
        assert_eq(again, false)
    end,

    ["a server that wasn't sent history has none pending"] = function()
        assert_eq(History.rowsPending({ stats_cursor = { rowid = 0 } }), false)
    end,

    ["history rows are the ones up to where sign-in left off"] = function()
        local rows = { { rowid = 4 }, { rowid = 5 }, { rowid = 6 } }
        assert_eq(History.rows({ through = 5 }, rows), { { rowid = 4 }, { rowid = 5 } })
        assert_eq(History.rows({ through = 3 }, rows), {})
    end,

    ["the statuses and stars recorded before signing in go as history"] = function()
        local store = { synced = {}, asked = {} }
        local books = {
            finished = { current = Summaries.read({ status = "complete", rating = 4, modified = "2025-03-01" }) },
            held = { current = Summaries.read({ status = "abandoned", modified = "2025-05-01" }) },
            recent = { current = Summaries.read({ status = "reading", modified = "2026-11-12" }) },
            unread = { unread = true },
        }
        Summaries.collect(store, books, { signed_in_day = "2026-11-01", today = "2026-11-13" })
        assert_eq(strip(History.events(store, books)), {
            { type = "status", document = "finished", status = "complete", day = "2025-03-01" },
            { type = "rating", document = "finished", rating = 4, day = "2025-03-01" },
            { type = "status", document = "held", status = "abandoned", day = "2025-05-01" },
        })

        -- Once a server takes them in, they're synced like any other.
        for __, event in ipairs(History.events(store, books)) do Summaries.acknowledge(store, event) end
        assert_eq(History.events(store, books), {})
        assert_eq(store.synced.finished, { status = "complete", rating = 4 })
    end,

    ["a batch of history events never splits a book"] = function()
        local events = {
            { document = "a" }, { document = "b" }, { document = "b" }, { document = "c" },
        }
        assert_eq(History.batch(events, 1, 2), { { document = "a" } })
        assert_eq(History.batch(events, 2, 2), { { document = "b" }, { document = "b" } })
        assert_eq(History.batch(events, 4, 2), { { document = "c" } })
        -- A batch holds at least one book, however small.
        assert_eq(History.batch(events, 2, 1), { { document = "b" }, { document = "b" } })
    end,

    ["history sent later knows the latest reading from before sign-in"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        local server = state:signIn{ url = "https://a.example", username = "r", credential = "C",
            stats_cursor = { rowid = 42, id_book = 1, page = 1, start_time = 900, latest = 1500 } }
        History.answer(server, false)
        server.stats_cursor = { rowid = 50, id_book = 1, page = 9, start_time = 1200 }
        History.accept(server)
        assert_eq(server.history.through_row, { id_book = 1, page = 9, start_time = 1200, latest = 1500 })

        local later = state:signIn{ url = "https://b.example", username = "r", credential = "C",
            stats_cursor = { rowid = 42, id_book = 1, page = 1, start_time = 900, latest = 1500 } }
        History.answer(later, false)
        later.stats_cursor = { rowid = 60, id_book = 1, page = 9, start_time = 2500 }
        History.accept(later)
        assert_eq(later.history.through_row.latest, 2500, "or of what was read since, when that's later")
    end,

    ["history can be sent later to a server that wasn't sent it"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        local server = state:signIn{ url = "https://a.example", username = "r", credential = "C", stats_cursor = { rowid = 42 } }
        server.asking_history = true
        assert_eq(History.sendable(server), false, "not while sign-in is still asking")
        History.answer(server, false)
        assert_eq(History.sendable(server), true, "declined at sign-in")
        -- Reading since sign-in went as usual.
        server.stats_cursor = { rowid = 50 }

        History.accept(server)

        assert_eq(server.history, { through = 50 }, "every row recorded so far, which the server takes once")
        assert_eq(server.stats_cursor, { rowid = 0 })
        assert_eq(server.stats_from, 0)
        assert_eq(History.rowsPending(server), true)
        assert_eq(History.sendable(server), false, "once is enough")
    end,

    ["history is unsent while it's still to go or can still be sent"] = function()
        assert_eq(History.unsent({}), true, "declined, or never asked")
        assert_eq(History.unsent({ asking_history = true }), false, "the sign-in is still asking")
        assert_eq(History.unsent({ history = { through = 5 }, stats_cursor = { rowid = 2 } }), true, "still going")
        assert_eq(History.unsent({ history = { through = 5 }, stats_cursor = { rowid = 5 } }), false, "sent")
    end,

    ["every row can be sent again, from the first, once history has gone"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        local server = state:signIn{ url = "https://a.example", username = "r", credential = "C", stats_cursor = { rowid = 42 } }
        History.answer(server, false)
        assert_eq(History.resendable(server), false, "never sent: Send reading history sends it")
        History.accept(server)
        assert_eq(History.resendable(server), false, "not while it's still going")
        server.stats_cursor = { rowid = 60, id_book = 2, page = 3, start_time = 1800 }
        assert_eq(History.resendable(server), true)

        History.resend(server)

        assert_eq(server.history.through, 42, "where history ends stays, so later rows go as reading again")
        assert_eq(server.history.resend, true)
        assert_eq(server.stats_cursor, { rowid = 0 })
        assert_eq(History.rowsPending(server), true)
        assert_eq(History.resendable(server), false, "not again while it's going")
        server.stats_cursor = { rowid = 60 }
        assert_eq(History.resendable(server), false, "nor while its statuses are still to go")
        History.resent(server)
        assert_eq(server.history.resend, nil)
        assert_eq(History.resendable(server), true)
    end,

    ["history sent again carries the statuses and stars of the books the server holds as unrecognized"] = function()
        local store = { synced = { held = { status = "complete" } }, asked = {} }
        local books = {
            held = { current = Summaries.read({ status = "complete", rating = 4, modified = "2026-11-03" }) },
            recognized = { current = Summaries.read({ status = "reading", modified = "2026-11-12" }) },
            nodate = { current = { status = "reading" } },
            unread = { unread = true },
        }
        local states = { held = "held", recognized = "recognized", nodate = "held", unread = "held" }
        local events = History.resendEvents(books, function(document) return states[document] end)
        assert_eq(strip(events), {
            { type = "status", document = "held", status = "complete", day = "2026-11-03" },
            { type = "rating", document = "held", rating = 4, day = "2026-11-03" },
        })
    end,

    ["history sent again adds the statuses of the books the server holds, only while it's going"] = function()
        local books = {
            held = { current = Summaries.read({ status = "complete", modified = "2026-11-03" }) },
            recognized = { current = Summaries.read({ status = "reading", modified = "2026-11-12" }) },
        }
        local recognition = { answer = function(_self, url, document)
            assert_eq(url, "https://a.example")
            return { state = document }
        end }
        local first = { { document = "before", type = "status" } }
        local server = { url = "https://a.example", history = { through = 5 } }
        assert_eq(History.withResent(first, server, books, recognition), { { document = "before", type = "status" } })

        server.history.resend = true
        local events = strip(History.withResent({}, server, books, recognition))
        assert_eq(events, { { type = "status", document = "held", status = "complete", day = "2026-11-03" } })
    end,
}
