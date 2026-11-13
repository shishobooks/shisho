--[[--
The document hash of each book file under KOReader's home folder (ADR
0016), so a status or Rating from Shisho can be written to a book this device
has never opened, which has no sidecar recording its hash. A file is hashed
again only when its size or modification time changes, and only during a
sync. It lives in its own file in KOReader's settings directory:

files = { [path] = { size, mtime, hash } }, scanned_at

No UI.
]]
local LuaSettings = require("luasettings")
local State = require("shisho/state")
local lfs = require("libs/libkoreader-lfs")

local HashIndex = {}
HashIndex.__index = HashIndex

-- SCAN_EVERY is how many seconds the home folder goes between scans, except
-- for a sync someone asked for. A sync while reading never scans.
HashIndex.SCAN_EVERY = 10 * 60

-- LIMIT is the most files one scan hashes, so a first scan of a large
-- library is spread over several syncs instead of holding one up.
HashIndex.LIMIT = 200

-- The extensions of the books Shisho delivers to KOReader. A KePub ends in
-- .epub.
local BOOKS = { epub = true, pdf = true, cbz = true, mobi = true, azw3 = true }

-- isBook reports whether path is a kind of book Shisho delivers, leaving out
-- everything else KOReader opens, such as images.
function HashIndex.isBook(path)
    local extension = path:match("%.([^./]+)$")
    return extension ~= nil and BOOKS[extension:lower()] == true
end

function HashIndex.open(path)
    local data = LuaSettings:open(path).data
    if type(data.files) ~= "table" then
        data.files = {}
    end
    return setmetatable({ path = path, data = data }, HashIndex)
end

function HashIndex:flush()
    return State.write(self.path, self.data, "The document hash of each book in KOReader's home folder, for the Shisho KOReader plugin.")
end

-- due reports whether a sync at now scans the home folder: one asked for
-- does, as does one an hour after the last scan. A clock set back since
-- doesn't hold it off.
function HashIndex.due(scanned_at, now, asked)
    if asked or not scanned_at then return true end
    local since = now - scanned_at
    return since < 0 or since >= HashIndex.SCAN_EVERY
end

--[[--
scan lists the books under root (HashIndex.isBook), by path, each
{ size, mtime }. Hidden folders, KOReader's sidecar folders (.sdr), and
links are left out, so a link to a folder above can't loop.
]]
function HashIndex.scan(root)
    local files = {}
    local function walk(dir)
        local ok, iter, dir_obj = pcall(lfs.dir, dir)
        if not ok then return end
        for name in iter, dir_obj do
            if name ~= "." and name ~= ".." and name:sub(1, 1) ~= "." then
                local path = dir .. "/" .. name
                local attr = lfs.symlinkattributes(path)
                if attr and attr.mode == "directory" then
                    if not name:match("%.sdr$") then walk(path) end
                elseif attr and attr.mode == "file" and HashIndex.isBook(path) then
                    files[path] = { size = attr.size, mtime = attr.modification }
                end
            end
        end
    end
    if root and lfs.attributes(root, "mode") == "directory" then
        walk((root:gsub("/+$", "")))
    end
    return files
end

--[[--
refresh brings the index up to date with files, from scan, at now: a file
that's new, or whose size or modification time changed, is hashed with
hash(path), and a file that's gone is dropped. It hashes at most limit
(HashIndex.LIMIT by default), and only once every file is hashed does the
scan count as done, so the next sync carries on. Returns how many it hashed.
]]
function HashIndex:refresh(files, hash, now, limit)
    limit = limit or HashIndex.LIMIT
    local hashed, complete = 0, true
    for path, attr in pairs(files) do
        local entry = self.data.files[path]
        if hashed >= limit then
            if not entry or entry.size ~= attr.size or entry.mtime ~= attr.mtime then complete = false end
        elseif not entry or entry.size ~= attr.size or entry.mtime ~= attr.mtime then
            hashed = hashed + 1
            local value = hash(path)
            if value then
                self.data.files[path] = { size = attr.size, mtime = attr.mtime, hash = value }
            else
                self.data.files[path] = nil
            end
        end
    end
    for path in pairs(self.data.files) do
        if not files[path] then self.data.files[path] = nil end
    end
    if complete then self.data.scanned_at = now end
    return hashed
end

function HashIndex:scannedAt()
    return self.data.scanned_at
end

-- files is the index by path, each { size, mtime, hash }.
function HashIndex:files()
    return self.data.files
end

return HashIndex
