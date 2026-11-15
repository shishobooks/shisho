-- Reading KOReader's statistics database.
local SQ3 = require("lua-ljsqlite3/init")
local Statistics = require("shisho/statistics")
local lfs = require("libs/libkoreader-lfs")

-- statsDB makes a statistics database with KOReader's own tables, holding
-- books { id = md5 } and rows { book, page, start, duration, total }.
local function statsDB(books, rows, wal)
    local path = scratch_dir() .. "/statistics.sqlite3"
    local conn = SQ3.open(path)
    if wal then conn:exec("PRAGMA journal_mode=WAL;") end
    conn:exec([[
        CREATE TABLE book (id integer PRIMARY KEY autoincrement, title text, authors text, notes integer,
            last_open integer, highlights integer, pages integer, series text, language text, md5 text,
            total_read_time integer, total_read_pages integer);
        CREATE TABLE page_stat_data (id_book integer, page integer NOT NULL DEFAULT 0,
            start_time integer NOT NULL DEFAULT 0, duration integer NOT NULL DEFAULT 0,
            total_pages integer NOT NULL DEFAULT 0, UNIQUE (id_book, page, start_time),
            FOREIGN KEY(id_book) REFERENCES book(id));
    ]])
    local book = conn:prepare("INSERT INTO book (id, title, md5) VALUES (?, ?, ?);")
    for id, md5 in pairs(books) do
        book:reset():bind(id, "Book " .. id, md5 or nil):step()
    end
    local row = conn:prepare("INSERT INTO page_stat_data VALUES (?, ?, ?, ?, ?);")
    for __, r in ipairs(rows) do
        row:reset():bind(r[1], r[2], r[3], r[4], r[5] or 100):step()
    end
    conn:close()
    return path
end

-- addRows adds rows after the fact, as the statistics plugin's next flush
-- does.
local function addRows(path, rows)
    local conn = SQ3.open(path)
    local row = conn:prepare("INSERT INTO page_stat_data VALUES (?, ?, ?, ?, ?);")
    for __, r in ipairs(rows) do
        row:reset():bind(r[1], r[2], r[3], r[4], r[5] or 100):step()
    end
    conn:close()
end

-- exec runs sql on the database at path, as KOReader's own statistics
-- screens change it.
local function exec(path, sql)
    local conn = SQ3.open(path)
    conn:exec(sql)
    conn:close()
end

-- serverFrom is a server signed in when the database at path held what it
-- holds now, which wasn't sent history, and read everything recorded since.
local function serverFrom(path)
    local cursor = Statistics.currentCursor(path)
    return {
        stats_cursor = cursor,
        stats_from = cursor.rowid,
        stats_from_at = Statistics.key(cursor),
    }
end

-- readAll reads everything after server's cursor, moving it on, as a sync
-- does.
local function readAll(path, server)
    local rows = Statistics.read(path, server.stats_cursor, 100)
    if #rows > 0 then server.stats_cursor = Statistics.cursorAfter(rows[#rows]) end
    return rows
end

local function keys(rows)
    local out = {}
    for __, r in ipairs(rows) do
        table.insert(out, string.format("%s:%d@%d", r.document, r.page, r.start_time))
    end
    return out
end

return {
    ["reads rows in the order they were recorded with their book's hash and KOReader's own fields"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, {
            { 1, 1, 1000, 20, 100 },
            { 1, 2, 1030, 40, 100 },
            { 2, 5, 2000, 30, 200 },
        })
        local rows = Statistics.read(path, nil, 10)
        assert_eq(keys(rows), { "aaa:1@1000", "aaa:2@1030", "bbb:5@2000" })
        assert_eq(rows[3].rowid, 3)
        assert_eq(rows[3].total_pages, 200)
        assert_eq(rows[3].duration, 30)
        assert_eq(type(rows[3].start_time), "number")
    end,

    ["reads only after the cursor, a batch at a time"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, {
            { 1, 1, 1000, 20 }, { 1, 2, 1000, 20 }, { 2, 1, 1000, 20 }, { 1, 3, 1100, 20 },
        })
        local first = Statistics.read(path, nil, 2)
        assert_eq(keys(first), { "aaa:1@1000", "aaa:2@1000" })
        local rest = Statistics.read(path, Statistics.cursorAfter(first[#first]), 10)
        assert_eq(keys(rest), { "bbb:1@1000", "aaa:3@1100" })
        assert_eq(#Statistics.read(path, Statistics.cursorAfter(rest[#rest]), 10), 0)
    end,

    ["a row recorded later with an earlier start, as after the clock was set back, is still read"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 5000, 20 }, { 1, 2, 5100, 20 } })
        local first = Statistics.read(path, nil, 10)
        addRows(path, { { 1, 3, 1000, 20 } })
        assert_eq(keys(Statistics.read(path, Statistics.cursorAfter(first[#first]), 10)), { "aaa:3@1000" })
    end,

    ["the cursor at sign-in is past every row recorded so far"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 5000, 20 }, { 1, 2, 5100, 20 } })
        local cursor = Statistics.currentCursor(path)
        assert_eq(cursor, { rowid = 2, id_book = 1, page = 2, start_time = 5100 })
        addRows(path, { { 1, 3, 1000, 20 } })
        assert_eq(keys(Statistics.read(path, cursor, 10)), { "aaa:3@1000" })
        assert_eq(Statistics.currentCursor(scratch_dir() .. "/none.sqlite3"), { rowid = 0 })
    end,

    ["a cursor from before rowids reads on after its start time"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 1000, 20 }, { 1, 2, 2000, 20 }, { 1, 3, 3000, 20 } })
        local rows = Statistics.read(path, { start_time = 2000, id_book = 1, page = 2 }, 10)
        assert_eq(keys(rows), { "aaa:3@3000" })
    end,

    ["reads a database in WAL mode, as the statistics plugin keeps it on devices"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 1000, 20 } }, true)
        assert_eq(keys(Statistics.read(path, nil, 10)), { "aaa:1@1000" })
    end,

    ["skips rows of books with no hash and rows with no duration"] = function()
        local path = statsDB({ [1] = "aaa", [2] = false }, {
            { 1, 1, 1000, 0 }, { 2, 1, 1000, 20 }, { 1, 2, 1100, 20 },
        })
        assert_eq(keys(Statistics.read(path, nil, 10)), { "aaa:2@1100" })
    end,

    ["counts the rows read would read between two cursors"] = function()
        local path = statsDB({ [1] = "aaa", [2] = false }, {
            { 1, 1, 1000, 20 }, { 2, 1, 1000, 20 }, { 1, 2, 1100, 0 }, { 1, 3, 1200, 20 }, { 1, 4, 1300, 20 },
        })
        assert_eq(Statistics.count(path, { rowid = 0 }, 4), 2, "the rows read would leave out aren't counted")
        assert_eq(Statistics.count(path, { rowid = 1 }, 5), 2)
        assert_eq(Statistics.count(scratch_dir() .. "/missing.sqlite3", { rowid = 0 }, 4), 0)
    end,

    ["a missing database reads as empty and isn't created"] = function()
        local path = scratch_dir() .. "/statistics.sqlite3"
        assert_eq(Statistics.read(path, nil, 10), {})
        assert_eq(lfs.attributes(path, "mode"), nil)
    end,

    ["statistics is on unless turned off or the plugin is disabled"] = function()
        assert_eq(Statistics.enabled(nil, nil), true)
        assert_eq(Statistics.enabled({ is_enabled = true }, {}), true)
        assert_eq(Statistics.enabled({ is_enabled = false }, {}), false)
        assert_eq(Statistics.enabled({ is_enabled = true }, { statistics = true }), false)
    end,

    ["lists the books with rows after a cursor, and reads one book's rows up to a row"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, {
            { 1, 1, 1000, 20 }, { 2, 1, 1100, 20 }, { 1, 2, 1200, 20 }, { 2, 2, 1300, 20 },
        })
        assert_eq(Statistics.documents(path, nil), { "aaa", "bbb" })
        assert_eq(Statistics.documents(path, { rowid = 3 }), { "bbb" })
        assert_eq(keys(Statistics.readDocument(path, "aaa", 0, 3)), { "aaa:1@1000", "aaa:2@1200" })
        assert_eq(keys(Statistics.readDocument(path, "aaa", 1, 3)), { "aaa:2@1200" }, "after where syncing started")
        assert_eq(keys(Statistics.readDocument(path, "bbb", 0, 3)), { "bbb:1@1100" })
    end,

    ["says what it knows of a book: its title, authors, language, and reading time"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 1000, 20 }, { 1, 2, 1100, 40 } })
        local conn = SQ3.open(path)
        conn:exec([[UPDATE book SET authors = 'Ursula K. Le Guin' || char(10) || 'N/A', language = 'en' WHERE id = 1;]])
        conn:close()
        assert_eq(Statistics.book(path, "aaa"), {
            title = "Book 1", authors = { "Ursula K. Le Guin" }, language = "en",
            reading_seconds = 60, first_read = 1000, last_read = 1140,
        })
        assert_eq(Statistics.book(path, "zzz"), nil)
    end,
    ["a cursor names the row it was taken from"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 1000, 20 }, { 1, 2, 1100, 20 } })
        local rows = Statistics.read(path, nil, 10)
        assert_eq(Statistics.cursorAfter(rows[2]), { rowid = 2, id_book = 1, page = 2, start_time = 1100 })
        assert_eq(Statistics.key(Statistics.cursorAfter(rows[2])), { id_book = 1, page = 2, start_time = 1100 })
        assert_eq(Statistics.key({ rowid = 0 }), nil)
    end,

    ["resetting the book being read still sends what it records next"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, {
            { 1, 1, 1000, 20 }, { 2, 1, 2000, 20 }, { 2, 2, 2100, 20 },
        })
        local server = { stats_cursor = { rowid = 0 }, stats_from = 0 }
        readAll(path, server)
        -- KOReader's Reset statistics for the current book deletes the book
        -- and its rows, and records the book again under a new id.
        exec(path, "DELETE FROM book WHERE id = 2; DELETE FROM page_stat_data WHERE id_book = 2;"
            .. "INSERT INTO book (id, title, md5) VALUES (3, 'Book 3', 'bbb');")
        addRows(path, { { 3, 3, 3000, 20 }, { 3, 4, 3100, 20 } })
        assert_eq(Statistics.repair(path, server), true)
        assert_eq(keys(readAll(path, server)), { "aaa:1@1000", "bbb:3@3000", "bbb:4@3100" },
            "the server takes in again what it has, and what's new isn't lost")
        assert_eq(Statistics.repair(path, server), false, "once read, nothing more to do")
        assert_eq(readAll(path, server), {})
    end,

    ["a server not sent history goes back only to the reading after sign-in"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, { { 1, 1, 1000, 20 }, { 2, 1, 2000, 20 } })
        local server = serverFrom(path)
        addRows(path, { { 2, 2, 3000, 20 } })
        assert_eq(keys(readAll(path, server)), { "bbb:2@3000" })
        exec(path, "DELETE FROM page_stat_data WHERE id_book = 2;")
        addRows(path, { { 2, 3, 4000, 20 } })
        assert_eq(Statistics.repair(path, server), true)
        assert_eq(server.stats_from, 1, "the reading from before sign-in stays unsent")
        assert_eq(server.stats_from_at, { id_book = 1, page = 1, start_time = 1000 })
        assert_eq(keys(readAll(path, server)), { "bbb:3@4000" })
    end,

    ["a statistics database started again sends everything in it"] = function()
        -- A server partway through being sent history.
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 1000, 20 }, { 1, 2, 2000, 20 } })
        local server = {
            stats_cursor = Statistics.cursorAfter(Statistics.read(path, nil, 1)[1]),
            stats_from = 0,
            history = { through = 2, through_at = { id_book = 1, page = 2, start_time = 2000 } },
        }
        os.remove(path)
        path = statsDB({ [1] = "ccc" }, { { 1, 1, 5000, 20 }, { 1, 2, 5100, 20 }, { 1, 3, 5200, 20 } })
        assert_eq(Statistics.repair(path, server), true)
        assert_eq(server.history.through, 0, "none of it is history")
        assert_eq(keys(readAll(path, server)), { "ccc:1@5000", "ccc:2@5100", "ccc:3@5200" })
    end,

    ["deleting the last rows read, as deleting short books does, still sends what's recorded next"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, { { 1, 1, 1000, 20 }, { 1, 2, 1100, 20 }, { 2, 1, 1200, 20 } })
        local server = { stats_cursor = { rowid = 0 }, stats_from = 0 }
        readAll(path, server)
        exec(path, "DELETE FROM book WHERE id = 2; DELETE FROM page_stat_data WHERE id_book = 2; VACUUM;")
        addRows(path, { { 1, 3, 1300, 20 } })
        assert_eq(Statistics.repair(path, server), true)
        assert_eq(keys(readAll(path, server)), { "aaa:1@1000", "aaa:2@1100", "aaa:3@1300" })
    end,

    ["where syncing started never moves past a cursor that still names its row"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, { { 1, 1, 1000, 20 }, { 2, 1, 2000, 20 } })
        local server = serverFrom(path)
        -- The clock was set back after sign-in, so nothing since started
        -- after the row syncing started after.
        addRows(path, { { 1, 2, 500, 20 }, { 1, 3, 600, 20 } })
        server.stats_cursor = Statistics.cursorAfter(Statistics.read(path, server.stats_cursor, 1)[1])
        exec(path, "DELETE FROM page_stat_data WHERE id_book = 2;")
        assert_eq(Statistics.repair(path, server), true)
        assert_eq(server.stats_from, 3)
        assert_eq(keys(readAll(path, server)), { "aaa:3@600" }, "what was still to go goes")
    end,

    ["deleting rows the cursor is past leaves it alone"] = function()
        local path = statsDB({ [1] = "aaa", [2] = "bbb" }, { { 1, 1, 1000, 20 }, { 2, 1, 2000, 20 } })
        local server = serverFrom(path)
        exec(path, "DELETE FROM page_stat_data WHERE id_book = 1;")
        addRows(path, { { 2, 2, 3000, 20 } })
        local cursor = server.stats_cursor
        assert_eq(Statistics.repair(path, server), false)
        assert_eq(server.stats_cursor, cursor)
        assert_eq(keys(readAll(path, server)), { "bbb:2@3000" })
    end,

    ["a cursor saved before cursors named their row takes its row, or goes back when past every row"] = function()
        local path = statsDB({ [1] = "aaa" }, { { 1, 1, 1000, 20 }, { 1, 2, 1100, 20 }, { 1, 3, 1200, 20 } })
        local server = { stats_cursor = { rowid = 2 }, stats_from = 1, history = { through = 3 } }
        assert_eq(Statistics.repair(path, server), true)
        assert_eq(server.stats_cursor, { rowid = 2, id_book = 1, page = 2, start_time = 1100 })
        assert_eq(server.stats_from_at, { id_book = 1, page = 1, start_time = 1000 })
        assert_eq(server.history.through_at, { id_book = 1, page = 3, start_time = 1200 })
        assert_eq(Statistics.repair(path, server), false)

        local past = { stats_cursor = { rowid = 9 }, stats_from = 1 }
        assert_eq(Statistics.repair(path, past), true)
        assert_eq(past.stats_cursor, { rowid = 1, id_book = 1, page = 1, start_time = 1000 })
    end,

    ["a missing database leaves every cursor alone"] = function()
        local server = { stats_cursor = { rowid = 5, id_book = 1, page = 1, start_time = 1000 }, stats_from = 0 }
        assert_eq(Statistics.repair(scratch_dir() .. "/none.sqlite3", server), false)
        assert_eq(server.stats_cursor.rowid, 5)
    end,
}
