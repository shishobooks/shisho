--[[--
Statuses and Ratings both ways (ADR 0016). KOReader keeps a book's status
(reading, abandoned for On hold, complete for Finished) and its stars in the
book's sidecar summary, with the day the summary was last saved, and fires
no event when they change. So on each sync the plugin compares every book's
summary with what it last synced with each server, and sends a server what
changed, with the value it last synced, so the server can tell whether
Shisho changed too. What a server sends back is written to the sidecar and
noted as synced, so it's never sent back as a change: On hold written for
Did Not Finish never returns as Paused, and a Rating rounded to whole stars
never overwrites its quarter stars.

KOReader marks a book reading the first time it opens one with no status,
and that goes to the server like any other change, so Shisho reads it too.
What KOReader recorded before the day the plugin signed in to the server
isn't a change: bringing that over is for the history sent on first sign-in
(ADR 0016), so Shisho's state, asked for outright, doesn't overwrite it
either. Only a later change in Shisho does.

A sync while reading reads only the sidecars of the open book and of the
books in KOReader's history. A book found only by the hash index is compared
by a sync that scans, and until then nothing is written to it, so a change
made on the device always reaches the server before Shisho's state can
overwrite it.

While a book is open, the plugin watches its status and stars, so a change
goes with the moment it was made, not just a day.

What was synced lives in its own file in KOReader's settings directory, so
the plugin's frequently written state stays small:

servers = { [url] = {
    synced = { [document] = { status, rating, before } },
    asked = { [document] = true or when },
} },
seen = { [document] = { status, at, rating, rating_at } },

where before marks a book whose summary predates signing in, asked marks the
books the server has been asked about outright (true when it recognized the
book, or when it didn't), and seen holds when the plugin saw an open book's
status or stars change. No UI.
]]
local Files = require("shisho/files")
local LuaSettings = require("luasettings")
local random = require("frontend/random")

local Summaries = {}
Summaries.__index = Summaries

-- The most books one pull asks about outright.
Summaries.ASK_BATCH = 500

-- RETRY is how many seconds a book a server didn't recognize waits before
-- it's asked about again, as a copy linked in Shisho since would now be.
Summaries.RETRY = 24 * 60 * 60

-- FORGET is how many seconds the plugin remembers when it saw a change.
Summaries.FORGET = 30 * 24 * 60 * 60

Summaries.STATUSES = { reading = true, abandoned = true, complete = true }

function Summaries.open(path)
    local data = LuaSettings:open(path).data
    if type(data.servers) ~= "table" then data.servers = {} end
    if type(data.seen) ~= "table" then data.seen = {} end
    return setmetatable({
        path = path,
        data = data,
        writer = Files.writer(path, "What the Shisho KOReader plugin last synced of each book's status and Rating."),
    }, Summaries)
end

-- server returns what was synced with the server at url.
function Summaries:server(url)
    local server = self.data.servers[url]
    if type(server) ~= "table" then
        server = {}
        self.data.servers[url] = server
    end
    server.synced = type(server.synced) == "table" and server.synced or {}
    server.asked = type(server.asked) == "table" and server.asked or {}
    return server
end

-- remove forgets everything synced with the server at url.
function Summaries:remove(url)
    self.data.servers[url] = nil
end

-- seen is when the plugin saw open books' statuses and stars change.
function Summaries:seen()
    return self.data.seen
end

-- flush writes the file, only when it changed: it can hold every book on
-- the device.
function Summaries:flush()
    return self.writer:write(self.data)
end

-- isDay reports whether value is a real day as YYYY-MM-DD, which a server
-- takes: a hand-edited sidecar's 2026-02-30 isn't.
local function isDay(value)
    if type(value) ~= "string" then return false end
    local y, m, d = value:match("^(%d%d%d%d)%-(%d%d)%-(%d%d)$")
    if not y then return false end
    local t = os.time{ year = tonumber(y), month = tonumber(m), day = tonumber(d), hour = 12 }
    return t ~= nil and os.date("%Y-%m-%d", t) == value
end

--[[--
read is a sidecar's summary as the plugin compares it: { status, rating, day },
where status is one KOReader shows, rating whole stars from 1 to 5, and day
when the summary was last saved. Nil for a book with no summary.
]]
function Summaries.read(summary)
    if type(summary) ~= "table" then return nil end
    local rating = tonumber(summary.rating)
    if rating then
        rating = math.floor(rating)
        if rating < 1 or rating > 5 then rating = nil end
    end
    return {
        status = Summaries.STATUSES[summary.status] and summary.status or nil,
        rating = rating,
        day = isDay(summary.modified) and summary.modified or nil,
    }
end

--[[--
observe notes in seen when the open book document's status or stars changed
from before to now, both from Summaries.read, at the device's time at. It
forgets what it saw long ago. Returns whether it noted anything.
]]
function Summaries.observe(seen, document, before, now, at)
    if not before or not now then return false end
    local entry = seen[document] or {}
    local noted = false
    if now.status and now.status ~= before.status then
        entry.status, entry.at = now.status, at
        noted = true
    end
    if now.rating ~= before.rating then
        entry.rating, entry.rating_at = now.rating or 0, at
        noted = true
    end
    if not noted then return false end
    seen[document] = entry
    for other, value in pairs(seen) do
        if math.max(value.at or 0, value.rating_at or 0) < at - Summaries.FORGET then
            seen[other] = nil
        end
    end
    return true
end

--[[--
instant is when the plugin saw a book's status become what current has,
from seen, or nil when it didn't: only an instant on the day KOReader
recorded counts, so one from an earlier change to the same status doesn't
date this one.
]]
function Summaries.instant(seen, current)
    if type(seen) ~= "table" or not current or not seen.at or seen.status ~= current.status then
        return nil
    end
    if current.day and os.date("%Y-%m-%d", seen.at) ~= current.day then
        return nil
    end
    return seen.at
end

--[[--
diff compares one book's summary as the device has it, current (from
Summaries.read, nil without one), with what was last synced with a server,
synced (nil when never). It returns the changes to send the server, each
with the value last synced, and a table of what to note as synced at once
without sending, or nil. opts:

- signed_in_day: the day the plugin first signed in to the server
- today: the device's day, for a change KOReader didn't date
- seen: when the plugin saw this book's status or stars change

A status goes with the instant the plugin saw it, or the day KOReader
saved the summary. KOReader dates no Rating, so one goes with the instant
the plugin saw it, or else the day the summary was saved, but no earlier
than signing in.
]]
function Summaries.diff(document, current, synced, opts)
    local changes, baseline = {}, nil
    if not current then return changes, nil end
    local function note(key, value)
        baseline = baseline or {}
        baseline[key] = value
    end
    -- Recorded before signing in: the history backfill's, not a change.
    local before = synced == nil and current.day ~= nil and opts.signed_in_day ~= nil and current.day < opts.signed_in_day
    if before then note("before", true) end
    synced = synced or {}
    local seen = opts.seen

    if current.status and current.status ~= synced.status then
        if before then
            note("status", current.status)
        else
            local at = Summaries.instant(seen, current)
            table.insert(changes, {
                id = random.uuid(true),
                type = "status",
                document = document,
                status = current.status,
                previous_status = synced.status,
                day = current.day or (not at and opts.today) or nil,
                at = at,
            })
        end
    end

    if current.rating ~= synced.rating then
        if before then
            note("rating", current.rating)
        else
            local change = {
                id = random.uuid(true),
                type = "rating",
                document = document,
                rating = current.rating or 0,
                previous_rating = synced.rating or 0,
            }
            if type(seen) == "table" and seen.rating_at and seen.rating == change.rating then
                change.at = seen.rating_at
            else
                local day = current.day or opts.today
                if opts.signed_in_day and day < opts.signed_in_day then day = opts.signed_in_day end
                change.day = day
            end
            table.insert(changes, change)
        end
    end
    return changes, baseline
end

local function sortedKeys(t)
    local keys = {}
    for key in pairs(t) do table.insert(keys, key) end
    table.sort(keys)
    return keys
end

--[[--
collect diffs each of books (Summaries.addBook) with what was synced with a
server, store (Summaries:server), notes what diff says to note, and returns
the changes to send, book by book. A book whose sidecar wasn't read this
sync is left for one that reads it. opts are diff's, with seen by document.
]]
function Summaries.collect(store, books, opts)
    local changes = {}
    for __, document in ipairs(sortedKeys(books)) do
        local book = books[document]
        if not book.unread then
            local found, baseline = Summaries.diff(document, book.current, store.synced[document], {
                signed_in_day = opts.signed_in_day,
                today = opts.today,
                seen = opts.seen and opts.seen[document],
            })
            if baseline then
                local entry = store.synced[document] or {}
                for key, value in pairs(baseline) do entry[key] = value end
                store.synced[document] = entry
            end
            for __, change in ipairs(found) do table.insert(changes, change) end
        end
    end
    return changes
end

--[[--
addBook adds the book at path to books, by its document hash: the one its
sidecar records, as KOReader computed it on first opening the book, or else
indexed, from the hash index. The first book added for a hash keeps it.
summary is its sidecar's summary, or unread when the sidecar wasn't read.
]]
function Summaries.addBook(books, path, recorded, indexed, summary, unread)
    local document = recorded or indexed
    if not document or books[document] then return end
    books[document] = { path = path, current = not unread and Summaries.read(summary) or nil, unread = unread or nil }
end

--[[--
localBooks is every book whose status and Rating sync, by document hash
(Summaries.addBook), from opts:

- open: the open book, { path, recorded, summary }, as KOReader holds it
- history: the paths of the books in KOReader's history that still exist
- index: the hash index's files, by path (HashIndex:files)
- read(path): a book's sidecar, as its recorded hash and summary
- read_index: whether to read the sidecars of books only the index found;
  a sync while reading doesn't, and leaves them unread

The open book comes first, then the history, then the index, so a book
found twice keeps the place that says most about it.
]]
function Summaries.localBooks(opts)
    local books, seen = {}, {}
    local index = opts.index or {}
    local function indexed(path) return index[path] and index[path].hash end
    if opts.open then
        seen[opts.open.path] = true
        Summaries.addBook(books, opts.open.path, opts.open.recorded, indexed(opts.open.path), opts.open.summary)
    end
    for __, path in ipairs(opts.history or {}) do
        if not seen[path] then
            seen[path] = true
            local recorded, summary = opts.read(path)
            Summaries.addBook(books, path, recorded, indexed(path), summary)
        end
    end
    local paths = {}
    for path in pairs(index) do
        if not seen[path] then table.insert(paths, path) end
    end
    table.sort(paths)
    for __, path in ipairs(paths) do
        if opts.read_index then
            local recorded, summary = opts.read(path)
            Summaries.addBook(books, path, recorded, index[path].hash, summary)
        else
            Summaries.addBook(books, path, nil, index[path].hash, nil, true)
        end
    end
    return books
end

-- documents lists the books whose state can be asked for outright: the ones
-- whose sidecars were read this sync, in order.
function Summaries.documents(books)
    local out = {}
    for __, document in ipairs(sortedKeys(books)) do
        if not books[document].unread then table.insert(out, document) end
    end
    return out
end

-- acknowledge records a change the server at store took in as synced. The
-- server may have refused it for a later change of its own, so it's asked
-- about the book outright until a pull answers.
function Summaries.acknowledge(store, change)
    local entry = store.synced[change.document] or {}
    if change.type == "status" then
        entry.status = change.status
    elseif change.type == "rating" then
        entry.rating = change.rating > 0 and change.rating or nil
    end
    entry.before = nil
    store.synced[change.document] = entry
    store.asked[change.document] = nil
end

--[[--
apply decides what a pulled state, { document, status, rating }, writes to a
book's sidecar, whose summary is current (nil without one), and records it
as synced. A state with no status leaves the device's alone, as does one
with no rating; rating 0 removes the device's. Asked for outright, it
doesn't overwrite a status or stars KOReader recorded before signing in, only
fills in what's missing. Returns { status, rating } to write, rating 0 for removing
it, or nil when the sidecar already says so.
]]
function Summaries.apply(synced, state, current, outright)
    local entry = synced[state.document] or {}
    synced[state.document] = entry
    local keep = outright and entry.before
    entry.before = nil
    local write = {}
    local have_status = current and current.status
    if Summaries.STATUSES[state.status] and not (keep and have_status) then
        entry.status = state.status
        if have_status ~= state.status then
            write.status = state.status
        end
    end
    local rating = tonumber(state.rating)
    local have = current and current.rating
    if rating and not (keep and have) then
        if rating > 0 then
            entry.rating = rating
            if have ~= rating then write.rating = rating end
        else
            entry.rating = nil
            if have then write.rating = 0 end
        end
    end
    return next(write) and write or nil
end

--[[--
applyTo is apply for the book a pulled state names, with what was synced
with its server, store. A book whose sidecar wasn't read this sync isn't
written: its own change may not have reached the server yet. It's asked about
outright at the next sync that reads it instead.
]]
function Summaries.applyTo(store, state, book, outright)
    if book.unread then
        store.asked[state.document] = nil
        return nil
    end
    return Summaries.apply(store.synced, state, book.current, outright)
end

-- written is summary, a sidecar's, with write (from apply) in it, as
-- KOReader's file manager saves a status: dated today.
function Summaries.written(summary, write, today)
    summary = summary or {}
    if write.status then
        summary.status = write.status
        summary.modified = today
    end
    if write.rating then
        summary.rating = write.rating > 0 and write.rating or nil
    end
    return summary
end

--[[--
toAsk is the books in documents whose state to ask a server for outright, at
now: ones it hasn't been asked about, or was asked about a day ago and didn't
recognize. Up to limit.
]]
function Summaries.toAsk(asked, documents, now, limit)
    local out = {}
    for __, document in ipairs(documents) do
        if #out >= (limit or Summaries.ASK_BATCH) then break end
        local when = asked[document]
        if when == nil or (type(when) == "number" and (now - when >= Summaries.RETRY or now < when)) then
            table.insert(out, document)
        end
    end
    return out
end

return Summaries
