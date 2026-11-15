--[[--
Reads KOReader's statistics database (ADR 0016): its page_stat_data rows,
each one visit to a page, with the document hash of the book they belong
to. Rows are read in the order they were recorded, by rowid, after a
per-server cursor, a bounded batch at a time, so a row recorded after the
device's clock was set back, which starts earlier, is still read.

Rowids aren't stable, though. KOReader deletes rows when a book's
statistics are reset or books are deleted, then vacuums, and a deleted or
restored database starts again, so later rows can take numbers a cursor is
already past. So each place the plugin keeps (a server's cursor, the row
its syncing started after, the last row of its history) names the row it
was taken from by KOReader's own key for it, { id_book, page, start_time },
and repair checks each against the database before a sync. The plugin
opens this database read-only, and never holds it open between calls.
No UI.
]]
local SQ3 = require("lua-ljsqlite3/init")
local lfs = require("libs/libkoreader-lfs")
local logger = require("logger")

local Statistics = {}

-- How long a read waits on the statistics plugin's own writes, in ms.
local BUSY_TIMEOUT = 2000

-- The rows synced, after a rowid: ones with a book's hash and some time.
local ROWS = [[
    FROM page_stat_data AS p
    JOIN book AS b ON b.id = p.id_book
    WHERE p.rowid > ? AND b.md5 IS NOT NULL AND b.md5 != '' AND p.duration > 0
]]

local QUERY = "SELECT p.rowid, p.start_time, p.id_book, p.page, p.total_pages, p.duration, b.md5 "
    .. ROWS .. " ORDER BY p.rowid LIMIT ?;"

local COUNT = "SELECT COUNT(*) " .. ROWS .. " AND p.rowid <= ?;"

-- withConnection runs fn with the database at path open read-only, and
-- closes it. Returns fn's result, or nil and an error.
local function withConnection(path, fn)
    local result
    local ok, err = pcall(function()
        local conn = SQ3.open(path, "ro")
        local run_ok, run_err = pcall(function()
            conn:set_busy_timeout(BUSY_TIMEOUT)
            result = fn(conn)
        end)
        conn:close()
        if not run_ok then error(run_err) end
    end)
    if not ok then
        logger.warn("Shisho: reading statistics failed:", err)
        return nil, err
    end
    return result
end

local function scalar(conn, sql, ...)
    local stmt = conn:prepare(sql)
    stmt:bind(...)
    local row = stmt:step()
    stmt:close()
    return row and tonumber(row[1]) or nil
end

-- rowidOf is cursor's rowid. A cursor from before rowids, { start_time,
-- id_book, page }, reads on after the last row recorded by its start time.
local function rowidOf(conn, cursor)
    if type(cursor) ~= "table" then return 0 end
    if cursor.rowid then return cursor.rowid end
    if cursor.start_time then
        return scalar(conn, "SELECT MAX(rowid) FROM page_stat_data WHERE start_time <= ?;", cursor.start_time) or 0
    end
    return 0
end

local function readRows(stmt)
    local rows = {}
    for row in stmt:rows() do
        table.insert(rows, {
            rowid = tonumber(row[1]),
            start_time = tonumber(row[2]),
            id_book = tonumber(row[3]),
            page = tonumber(row[4]),
            total_pages = tonumber(row[5]),
            duration = tonumber(row[6]),
            document = tostring(row[7]),
        })
    end
    return rows
end

--[[--
read returns up to limit rows after cursor ({ rowid }, nil for the start),
each { rowid, document, id_book, page, total_pages, start_time, duration }.
A database that doesn't exist yet reads as empty. Returns nil and an error
when it can't be read.
]]
function Statistics.read(path, cursor, limit)
    if lfs.attributes(path, "mode") ~= "file" then
        return {}
    end
    return withConnection(path, function(conn)
        local stmt = conn:prepare(QUERY)
        stmt:bind(rowidOf(conn, cursor), limit)
        local rows = readRows(stmt)
        stmt:close()
        return rows
    end)
end

local DOCUMENTS = "SELECT DISTINCT b.md5 " .. ROWS .. " ORDER BY b.md5;"

-- documents is the document hash of every book with rows after cursor, as
-- read would read them, or nil and an error.
function Statistics.documents(path, cursor)
    if lfs.attributes(path, "mode") ~= "file" then
        return {}
    end
    return withConnection(path, function(conn)
        local out = {}
        local stmt = conn:prepare(DOCUMENTS)
        stmt:bind(rowidOf(conn, cursor))
        for row in stmt:rows() do table.insert(out, tostring(row[1])) end
        stmt:close()
        return out
    end)
end

local DOCUMENT_ROWS = "SELECT p.rowid, p.start_time, p.id_book, p.page, p.total_pages, p.duration, b.md5 "
    .. ROWS .. " AND b.md5 = ? AND p.rowid <= ? ORDER BY p.rowid;"

-- readDocument returns every row of the book document after the row from
-- and up to the row through, as read returns rows, or nil and an error.
function Statistics.readDocument(path, document, from, through)
    if lfs.attributes(path, "mode") ~= "file" then
        return {}
    end
    return withConnection(path, function(conn)
        local stmt = conn:prepare(DOCUMENT_ROWS)
        stmt:bind(from or 0, document, through)
        local rows = readRows(stmt)
        stmt:close()
        return rows
    end)
end

local BOOK = [[
    SELECT b.title, b.authors, b.language,
        (SELECT SUM(p.duration) FROM page_stat_data AS p WHERE p.id_book = b.id),
        (SELECT MIN(p.start_time) FROM page_stat_data AS p WHERE p.id_book = b.id),
        (SELECT MAX(p.start_time + p.duration) FROM page_stat_data AS p WHERE p.id_book = b.id)
    FROM book AS b WHERE b.md5 = ? ORDER BY b.id LIMIT 1;
]]

local function text(value)
    if value == nil then return nil end
    value = tostring(value)
    return value ~= "" and value or nil
end

--[[--
book is what statistics knows of the book document: { title, authors,
language, reading_seconds, first_read, last_read }, where authors is a list
(KOReader keeps one per line) and the times are the device's. Nil when
statistics has no such book.
]]
function Statistics.book(path, document)
    if lfs.attributes(path, "mode") ~= "file" then
        return nil
    end
    return withConnection(path, function(conn)
        local stmt = conn:prepare(BOOK)
        stmt:bind(document)
        local row = stmt:step()
        stmt:close()
        if not row then return nil end
        local authors = {}
        for author in (text(row[2]) or ""):gmatch("[^\n]+") do
            author = author:gsub("^%s+", ""):gsub("%s+$", "")
            -- KOReader records a book with no authors as N/A.
            if author ~= "" and author ~= "N/A" then table.insert(authors, author) end
        end
        return {
            title = text(row[1]),
            authors = authors,
            language = text(row[3]),
            reading_seconds = tonumber(row[4]) or 0,
            first_read = tonumber(row[5]),
            last_read = tonumber(row[6]),
        }
    end)
end

-- count is how many rows read would read after cursor up to the row
-- through, or 0 when they can't be counted.
function Statistics.count(path, cursor, through)
    if lfs.attributes(path, "mode") ~= "file" then
        return 0
    end
    local count = withConnection(path, function(conn)
        return scalar(conn, COUNT, rowidOf(conn, cursor), through)
    end)
    return count or 0
end

-- cursorAfter is the cursor that reads on after row, naming row.
function Statistics.cursorAfter(row)
    return { rowid = row.rowid, id_book = row.id_book, page = row.page, start_time = row.start_time }
end

--[[--
key is the row cursor names, { id_book, page, start_time }, or nil for a
cursor at the start or one that names no row. A cursor taken at sign-in
also carries latest, the latest start of any row up to it, which key keeps.
]]
function Statistics.key(cursor)
    if type(cursor) ~= "table" or cursor.id_book == nil or cursor.page == nil or cursor.start_time == nil then
        return nil
    end
    return { id_book = cursor.id_book, page = cursor.page, start_time = cursor.start_time, latest = cursor.latest }
end

-- place is the cursor at rowid naming row (key), as the plugin's state keeps
-- them apart for where syncing started and where history ends.
function Statistics.place(rowid, row)
    local place = Statistics.cursorAfter(type(row) == "table" and row or {})
    place.rowid = tonumber(rowid) or 0
    place.latest = type(row) == "table" and row.latest or nil
    return place
end

local MAX_ROWID = "SELECT MAX(rowid) FROM page_stat_data;"
local ROW_AT_OR_BEFORE =
    "SELECT rowid, id_book, page, start_time FROM page_stat_data WHERE rowid <= ? ORDER BY rowid DESC LIMIT 1;"
local FIRST_AFTER = "SELECT MIN(rowid) FROM page_stat_data WHERE start_time > ?;"

-- rowAtOrBefore is the cursor after the last row numbered rowid or less, or
-- the cursor at the start when there's none.
local function rowAtOrBefore(conn, rowid)
    local stmt = conn:prepare(ROW_AT_OR_BEFORE)
    stmt:bind(rowid)
    local row = stmt:step()
    stmt:close()
    if not row then return { rowid = 0 } end
    return Statistics.cursorAfter({
        rowid = tonumber(row[1]), id_book = tonumber(row[2]), page = tonumber(row[3]), start_time = tonumber(row[4]),
    })
end

local function sameRow(a, b)
    return a.rowid == b.rowid and a.id_book == b.id_book and a.page == b.page and a.start_time == b.start_time
end

--[[--
check is what the database says of place: the place itself when its row
is still there; the row at or just before it when it names no row, for a
place saved before places named their row; or nil when its row is gone or
renumbered.
]]
local function check(conn, place, max)
    local rowid = tonumber(place.rowid) or 0
    if rowid == 0 then return { rowid = 0 } end
    if not Statistics.key(place) then
        if rowid > max then return nil end
        return rowAtOrBefore(conn, rowid)
    end
    if sameRow(rowAtOrBefore(conn, rowid), place) then return place end
    return nil
end

-- byTime is where a boundary whose row is gone goes once rows may have
-- been renumbered: after the rows recorded before the first row that started
-- after after, so a row from before it stays before it.
local function byTime(conn, after, max)
    local first = after and scalar(conn, FIRST_AFTER, after)
    if not first then return rowAtOrBefore(conn, max) end
    return rowAtOrBefore(conn, first - 1)
end

--[[--
repair checks server's places in the statistics database at path, where
rowids can change under them, and moves each that no longer names its row:

- while the cursor still names its row, no row up to it was renumbered,
  only deleted, so where syncing started (stats_from, stats_from_row) and
  the last row of history (history.through, history.through_row) up to it
  keep their number, naming the row now at or before it
- otherwise each goes by time, to the rows recorded before the first that
  started after the latest row up to it did when it was taken (or after its
  own row, when that isn't known), so a server not sent history is never
  sent reading from before sign-in
- a cursor that no longer names its row goes back to where syncing started,
  so everything since is sent again; the server takes in only what it
  doesn't have, and keeps what the person deleted deleted

A place saved before places named their row takes the row it's at. Returns
whether anything moved, so the state needs saving. A missing database, or
one that can't be read, leaves every place alone.
]]
function Statistics.repair(path, server)
    if lfs.attributes(path, "mode") ~= "file" then return false end
    if type(server.stats_cursor) ~= "table" or not server.stats_cursor.rowid then return false end
    local history = type(server.history) == "table" and server.history or nil
    local found = withConnection(path, function(conn)
        local max = scalar(conn, MAX_ROWID) or 0
        local cursor = check(conn, server.stats_cursor, max)
        local function boundary(place)
            local kept = check(conn, place, max)
            if not kept and cursor and place.rowid <= cursor.rowid then
                kept = rowAtOrBefore(conn, place.rowid)
            end
            kept = kept or byTime(conn, place.latest or place.start_time, max)
            kept.latest = place.latest
            return kept
        end
        local from = boundary(Statistics.place(server.stats_from, server.stats_from_row))
        local through = history and boundary(Statistics.place(history.through, history.through_row))
        return { cursor = cursor or Statistics.cursorAfter(from), from = from, through = through }
    end)
    if not found then return false end

    local changed = false
    if not sameRow(found.cursor, server.stats_cursor) then
        server.stats_cursor = found.cursor
        changed = true
    end
    if not sameRow(found.from, Statistics.place(server.stats_from, server.stats_from_row)) then
        server.stats_from, server.stats_from_row = found.from.rowid, Statistics.key(found.from)
        changed = true
    end
    if history and not sameRow(found.through, Statistics.place(history.through, history.through_row)) then
        history.through, history.through_row = found.through.rowid, Statistics.key(found.through)
        changed = true
    end
    return changed
end

-- currentCursor is the cursor past every row recorded so far, where a
-- server signed in to now starts, with latest, the latest start of any row.
function Statistics.currentCursor(path)
    if lfs.attributes(path, "mode") ~= "file" then
        return { rowid = 0 }
    end
    local cursor = withConnection(path, function(conn)
        local cursor = rowAtOrBefore(conn, scalar(conn, MAX_ROWID) or 0)
        cursor.latest = scalar(conn, "SELECT MAX(start_time) FROM page_stat_data;")
        return cursor
    end)
    return cursor or { rowid = 0 }
end

--[[--
enabled reports whether KOReader's statistics plugin is recording: its
settings (G_reader_settings "statistics") don't turn it off, and it isn't
among the disabled plugins.
]]
function Statistics.enabled(settings, plugins_disabled)
    if type(plugins_disabled) == "table" and plugins_disabled.statistics then
        return false
    end
    return not (type(settings) == "table" and settings.is_enabled == false)
end

return Statistics
