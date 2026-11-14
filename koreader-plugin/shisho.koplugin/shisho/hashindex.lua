--[[--
The document hash of each book file on the device (ADR 0016), so a status
or Rating from Shisho can be written to a book this device has never opened,
which has no sidecar recording its hash. HashIndex.roots says which folders
it covers. A file is hashed
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

-- SCAN_EVERY is how many seconds the index goes between scans, except
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
    return State.write(self.path, self.data, "The document hash of each book on the device, for the Shisho KOReader plugin.")
end

-- due reports whether a sync at now scans the device's folders: one asked for
-- does, as does one SCAN_EVERY after the last scan. A clock set back since
-- doesn't hold it off.
function HashIndex.due(scanned_at, now, asked)
    if asked or not scanned_at then return true end
    local since = now - scanned_at
    return since < 0 or since >= HashIndex.SCAN_EVERY
end

local function trimmed(folder)
    if type(folder) ~= "string" or folder == "" then return nil end
    return folder == "/" and folder or (folder:gsub("/+$", ""))
end

local function inside(folder, parent)
    return folder == parent or parent == "/" or folder:sub(1, #parent + 1) == parent .. "/"
end

--[[--
roots is the folders the index covers, from folders { storage, home,
download, ereader }: storage is the device's own storage, home KOReader's
home folder, and download the folder KOReader's OPDS catalogs download to.
An e-reader's storage holds only what was put on it, so all of it is
covered, wherever books were copied or downloaded. Elsewhere, as on Android
or a computer, the storage is everything, so only the home and download
folders are. A folder inside another one covered is left out.
]]
function HashIndex.roots(folders)
    local candidates = {}
    if folders.ereader then table.insert(candidates, trimmed(folders.storage)) end
    table.insert(candidates, trimmed(folders.home))
    table.insert(candidates, trimmed(folders.download))
    -- Shorter first, so a parent comes before what's inside it.
    table.sort(candidates, function(a, b) return #a < #b or (#a == #b and a < b) end)
    local roots = {}
    for __, candidate in ipairs(candidates) do
        local covered = false
        for __, root in ipairs(roots) do
            if inside(candidate, root) then covered = true end
        end
        if not covered then table.insert(roots, candidate) end
    end
    table.sort(roots)
    return roots
end

--[[--
scan lists the books in roots (a folder, or a list from HashIndex.roots),
by path, each { size, mtime }, as HashIndex.isBook takes them. Hidden
folders, KOReader's sidecar folders (.sdr), and links are left out, so a link
to a folder above can't loop.
]]
function HashIndex.scan(roots)
    if type(roots) ~= "table" then roots = { roots } end
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
    for __, root in ipairs(roots) do
        root = trimmed(root)
        if root and lfs.attributes(root, "mode") == "directory" then
            walk(root == "/" and "" or root)
        end
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
