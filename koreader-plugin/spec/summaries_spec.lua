-- Comparing books' statuses and Ratings with what was synced, and writing
-- back what a server sends.
local Summaries = require("shisho/summaries")
local lfs = require("libs/libkoreader-lfs")

local OPTS = { signed_in_day = "2026-11-01", today = "2026-11-13" }

local function summary(status, rating, day)
    return Summaries.read({ status = status, rating = rating, modified = day })
end

local function strip(changes)
    -- The ids are random; the rest is what's compared.
    for __, change in ipairs(changes) do
        assert(type(change.id) == "string" and #change.id > 0, "each change has an id")
        change.id = nil
    end
    return changes
end

return {
    ["a sidecar summary reads as a status, whole stars, and a day"] = function()
        assert_eq(Summaries.read({ status = "complete", rating = 4, modified = "2026-11-02", note = "x" }),
            { status = "complete", rating = 4, day = "2026-11-02" })
        assert_eq(Summaries.read({ status = "new", rating = 0, modified = "24.01.2016" }), {})
        assert_eq(Summaries.read(nil), nil)
    end,

    ["a status changed since the last sync goes with the one synced"] = function()
        local changes = Summaries.diff("doc1", summary("complete", nil, "2026-11-10"), { status = "reading" }, OPTS)
        assert_eq(strip(changes), {
            { type = "status", document = "doc1", status = "complete", previous_status = "reading", day = "2026-11-10" },
        })
    end,

    ["nothing changed sends nothing"] = function()
        local changes, baseline = Summaries.diff("doc1", summary("abandoned", 3, "2026-11-10"), { status = "abandoned", rating = 3 }, OPTS)
        assert_eq(changes, {})
        assert_eq(baseline, nil)
        assert_eq(Summaries.diff("doc1", nil, { status = "complete" }, OPTS), {})
    end,

    ["opening a book marks it reading, which isn't a change"] = function()
        local changes, baseline = Summaries.diff("doc1", summary("reading", nil, "2026-11-13"), nil, OPTS)
        assert_eq(changes, {})
        assert_eq(baseline, { status = "reading" })
    end,

    ["a book finished from the file manager after signing in is a change"] = function()
        local changes, baseline = Summaries.diff("doc1", summary("complete", nil, "2026-11-12"), nil, OPTS)
        assert_eq(strip(changes), {
            { type = "status", document = "doc1", status = "complete", day = "2026-11-12" },
        })
        assert_eq(baseline, nil)
    end,

    ["what KOReader recorded before signing in is noted, not sent"] = function()
        local changes, baseline = Summaries.diff("doc1", summary("complete", 5, "2026-10-01"), nil, OPTS)
        assert_eq(changes, {})
        assert_eq(baseline, { before = true, status = "complete", rating = 5 })
    end,

    ["a Rating changed on the device goes, removing one included"] = function()
        -- Dated by the day KOReader saved the summary, but no earlier than
        -- signing in.
        assert_eq(strip(Summaries.diff("doc1", summary("reading", 4, "2026-11-05"), { status = "reading" }, OPTS)), {
            { type = "rating", document = "doc1", rating = 4, previous_rating = 0, day = "2026-11-05" },
        })
        assert_eq(strip(Summaries.diff("doc1", summary("reading", nil, "2026-10-01"), { status = "reading", rating = 4 }, OPTS)), {
            { type = "rating", document = "doc1", rating = 0, previous_rating = 4, day = "2026-11-01" },
        })
    end,

    ["a Rating the plugin saw change goes with when it did"] = function()
        local opts = { signed_in_day = OPTS.signed_in_day, today = OPTS.today, seen = { rating = 3, rating_at = 4000 } }
        assert_eq(strip(Summaries.diff("doc1", summary("reading", 3, "2026-11-05"), { status = "reading", rating = 5 }, opts)), {
            { type = "rating", document = "doc1", rating = 3, previous_rating = 5, at = 4000 },
        })
    end,

    ["the instant the plugin saw goes with a status it saw change that day"] = function()
        local at = os.time{ year = 2026, month = 11, day = 12, hour = 21 }
        local opts = { signed_in_day = OPTS.signed_in_day, seen = { status = "complete", at = at } }
        local changes = Summaries.diff("doc1", summary("complete", nil, "2026-11-12"), { status = "reading" }, opts)
        assert_eq(changes[1].at, at)
        -- An instant from an earlier finish doesn't date this one.
        changes = Summaries.diff("doc1", summary("complete", nil, "2026-11-13"), { status = "reading" }, opts)
        assert_eq(changes[1].at, nil)
        assert_eq(changes[1].day, "2026-11-13")
    end,

    ["watches the open book's status and stars change"] = function()
        local seen = { old = { status = "complete", at = 1 } }
        local now = 100 * 24 * 60 * 60
        assert_eq(Summaries.observe(seen, "doc1", summary("reading"), summary("reading"), now), false)
        assert_eq(Summaries.observe(seen, "doc1", summary("reading"), summary("complete", 4), now), true)
        assert_eq(seen.doc1, { status = "complete", at = now, rating = 4, rating_at = now })
        assert_eq(seen.old, nil, "what it saw long ago is forgotten")
        -- Stars removed are a change too.
        assert_eq(Summaries.observe(seen, "doc1", summary("complete", 4), summary("complete"), now + 5), true)
        assert_eq(seen.doc1.rating, 0)
        assert_eq(seen.doc1.rating_at, now + 5)
    end,

    ["a change the server took in is synced, so it isn't sent again"] = function()
        local store = { synced = { doc1 = { status = "reading" } }, asked = { doc1 = true } }
        local current = summary("complete", 2, "2026-11-12")
        for __, change in ipairs(Summaries.diff("doc1", current, store.synced.doc1, OPTS)) do
            Summaries.acknowledge(store, change)
        end
        assert_eq(store.synced.doc1, { status = "complete", rating = 2 })
        assert_eq(Summaries.diff("doc1", current, store.synced.doc1, OPTS), {})
        -- The server may have refused it, so it's asked about until a pull answers.
        assert_eq(store.asked, {})
    end,

    ["what a server wrote is never sent back"] = function()
        -- Did Not Finish arrives as On hold, and 3.75 stars as 4.
        local synced = { doc1 = { status = "reading" } }
        local current = summary("reading", nil, "2026-11-01")
        local write = Summaries.apply(synced, { document = "doc1", status = "abandoned", rating = 4 }, current, false)
        assert_eq(write, { status = "abandoned", rating = 4 })
        local written = Summaries.read(Summaries.written({ status = "reading", modified = "2026-11-01" }, write, "2026-11-13"))
        assert_eq(written, { status = "abandoned", rating = 4, day = "2026-11-13" })
        assert_eq(Summaries.diff("doc1", written, synced.doc1, OPTS), {})
    end,

    ["a server's state leaves alone what it doesn't say"] = function()
        local synced = {}
        -- To Read or no status, and no Rating: nothing to write.
        assert_eq(Summaries.apply(synced, { document = "doc1" }, summary("complete", 3, "2026-10-01"), true), nil)
        -- The same as the device already has: nothing to write either.
        assert_eq(Summaries.apply(synced, { document = "doc2", status = "complete", rating = 3 }, summary("complete", 3, "2026-10-01"), true), nil)
        assert_eq(synced.doc2, { status = "complete", rating = 3 })
    end,

    ["a Rating removed in Shisho is removed on the device"] = function()
        local synced = { doc1 = { status = "complete", rating = 4 } }
        assert_eq(Summaries.apply(synced, { document = "doc1", status = "complete", rating = 0 }, summary("complete", 4), false), { rating = 0 })
        assert_eq(synced.doc1, { status = "complete" })
        assert_eq(Summaries.apply(synced, { document = "doc1", rating = 0 }, summary("complete"), false), nil)
    end,

    ["a book never opened gets a sidecar"] = function()
        local synced = {}
        local write = Summaries.apply(synced, { document = "doc1", status = "complete", rating = 5 }, nil, true)
        assert_eq(write, { status = "complete", rating = 5 })
        local written = Summaries.written(nil, write, "2026-11-13")
        assert_eq(written, { status = "complete", rating = 5, modified = "2026-11-13" })
        assert_eq(Summaries.diff("doc1", Summaries.read(written), synced.doc1, OPTS), {})
    end,

    ["writing keeps the rest of the summary, and dates only a status"] = function()
        local function before() return { status = "reading", rating = 2, note = "mine", modified = "2026-10-01" } end
        assert_eq(Summaries.written(before(), { rating = 0 }, "2026-11-13"), { status = "reading", note = "mine", modified = "2026-10-01" })
        assert_eq(Summaries.written(before(), { status = "complete" }, "2026-11-13").modified, "2026-11-13")
    end,

    ["Shisho's state asked for outright doesn't overwrite what KOReader recorded before signing in"] = function()
        local store = { synced = {}, asked = {} }
        local current = summary("complete", 5, "2026-10-01")
        assert_eq(Summaries.collect(store, { doc1 = { current = current } }, OPTS), {})
        assert_eq(Summaries.apply(store.synced, { document = "doc1", status = "reading", rating = 3 }, current, true), nil)
        assert_eq(store.synced.doc1, { status = "complete", rating = 5 })
        -- A change in Shisho afterwards does.
        assert_eq(Summaries.apply(store.synced, { document = "doc1", status = "reading" }, current, false), { status = "reading" })
    end,

    ["a book KOReader only opened before signing in takes Shisho's status"] = function()
        local store = { synced = {}, asked = {} }
        local current = summary("reading", nil, "2026-10-01")
        Summaries.collect(store, { doc1 = { current = current } }, OPTS)
        assert_eq(Summaries.apply(store.synced, { document = "doc1", status = "complete", rating = 4 }, current, true),
            { status = "complete", rating = 4 })
    end,

    ["collects every book's changes and notes the rest"] = function()
        local store = { synced = { done = { status = "reading" } }, asked = {} }
        local books = {
            done = { current = summary("complete", nil, "2026-11-12") },
            opened = { current = summary("reading", nil, "2026-11-13") },
            unopened = { current = nil },
        }
        local changes = strip(Summaries.collect(store, books, OPTS))
        assert_eq(changes, { { type = "status", document = "done", status = "complete", previous_status = "reading", day = "2026-11-12" } })
        assert_eq(store.synced.opened, { status = "reading" })
        assert_eq(store.synced.unopened, nil)
        assert_eq(Summaries.documents(books), { "done", "opened", "unopened" })
    end,

    ["a book's hash is the one its sidecar records, and the first book keeps it"] = function()
        local books = {}
        Summaries.addBook(books, "/a.epub", "recorded", "indexed", { status = "complete" })
        Summaries.addBook(books, "/b.epub", nil, "recorded", { status = "reading" })
        Summaries.addBook(books, "/c.epub", nil, nil, { status = "reading" })
        Summaries.addBook(books, "/d.epub", nil, "unread", nil, true)
        assert_eq(books, {
            recorded = { path = "/a.epub", current = { status = "complete" } },
            unread = { path = "/d.epub", unread = true },
        })
        -- A book whose sidecar wasn't read isn't compared.
        local store = { synced = {}, asked = {} }
        assert_eq(Summaries.collect(store, { unread = books.unread }, OPTS), {})
        assert_eq(store.synced, {})
    end,

    ["asks outright about books not asked before, and unrecognized ones a day later"] = function()
        local asked = { a = true, b = 1000, c = 1000 - Summaries.RETRY }
        assert_eq(Summaries.toAsk(asked, { "a", "b", "c", "d", "e" }, 1000), { "c", "d", "e" })
        assert_eq(Summaries.toAsk(asked, { "c", "d", "e" }, 1000, 2), { "c", "d" })
    end,

    ["what was synced is kept per server, in its own file, written only when it changed"] = function()
        local path = scratch_dir() .. "/shisho_summaries.lua"
        local store = Summaries.open(path)
        store:server("https://a.example").synced.doc1 = { status = "complete" }
        store:server("https://b.example").asked.doc1 = true
        assert_eq(store:flush(), true)
        os.remove(path)
        assert_eq(store:flush(), true)
        assert_eq(lfs.attributes(path, "mode"), nil, "nothing changed, so nothing is written")
        store:seen().doc1 = { status = "complete", at = 1 }
        assert_eq(store:flush(), true)

        local again = Summaries.open(path)
        assert_eq(again:seen(), { doc1 = { status = "complete", at = 1 } })
        assert_eq(again:server("https://a.example").synced, { doc1 = { status = "complete" } })
        assert_eq(again:server("https://a.example").asked, {})
        again:remove("https://b.example")
        assert_eq(again:server("https://b.example").asked, {})
    end,
}
