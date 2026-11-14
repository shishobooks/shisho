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
        assert_eq(cursor, { rowid = 2 })
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
}
