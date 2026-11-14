--[[--
Reads KOReader's statistics database (ADR 0016): its page_stat_data rows,
each one visit to a page, with the document hash of the book they belong
to. Rows are read in the order they were recorded, by rowid, after a
per-server cursor, a bounded batch at a time. The statistics plugin only
ever inserts rows (INSERT OR IGNORE), so the order holds even when the
device's clock is set back and a later row starts earlier. The plugin
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

-- currentCursor is the cursor past every row recorded so far, where a
-- server signed in to now starts.
function Statistics.currentCursor(path)
    if lfs.attributes(path, "mode") ~= "file" then
        return { rowid = 0 }
    end
    local rowid = withConnection(path, function(conn)
        return scalar(conn, "SELECT MAX(rowid) FROM page_stat_data;")
    end)
    return { rowid = rowid or 0 }
end

-- cursorAfter is the cursor that reads on after row.
function Statistics.cursorAfter(row)
    return { rowid = row.rowid }
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
