--[[--
Writes the plugin's own files in KOReader's settings directory, and the
books' sidecars it writes to, as Lua files LuaSettings reads, atomically:
through a temporary file beside each, renamed over the old one, so a crash,
a dead battery, or another process reading it leaves the old file or the new
one, never half of either. No UI.
]]
local dump = require("dump")
local ffiutil = require("ffi/util")
local util = require("util")

local Files = {}

-- replace writes content to path through a temporary file beside it,
-- renamed over the old one.
local function replace(path, content)
    local tmp = path .. ".tmp"
    local file, err = io.open(tmp, "wb")
    if not file then
        return nil, err
    end
    local ok
    ok, err = file:write(content)
    -- A full disk takes a write into the buffer and fails it here.
    if ok then ok, err = file:flush() end
    -- Best effort, as KOReader's own writes: some storage can't sync, and
    -- what flush handed the system is there to read either way.
    if ok then ffiutil.fsyncOpenedFile(file) end
    local closed, close_err = file:close()
    if not ok or not closed then
        -- A short write, as on a full disk, must never replace a good file.
        os.remove(tmp)
        return nil, err or close_err
    end
    local renamed, rename_err = os.rename(tmp, path)
    if not renamed then
        os.remove(tmp)
        return nil, rename_err
    end
    ffiutil.fsyncDirectory(path)
    return true
end

-- write writes data to path, with comment as its first line.
function Files.write(path, data, comment)
    return replace(path, table.concat({ "-- ", comment, "\nreturn ", dump(data, nil, true), "\n" }))
end

-- copy copies the file at from to to, whole or not at all.
function Files.copy(from, to)
    local source, err = io.open(from, "rb")
    if not source then
        return nil, err
    end
    local content = source:read("*a")
    source:close()
    if not content then
        return nil, "can't read " .. from
    end
    return replace(to, content)
end

--[[--
writeSidecar saves doc_settings, a book's DocSettings, where
DocSettings:flush would, but through Files.write. flush truncates the
sidecar and then writes it, and DocSettings:open deletes a sidecar it finds
empty or cut short. The file browser's cover extraction runs in a process
of its own that opens each new book's sidecar, so one opening it mid-write
would delete it, and everything in it. It reads DocSettings fields that
aren't KOReader's API, and without them saves with flush after all.
Returns the sidecar's directory, or nil and an error when no location took
it, leaving the sidecar as it was.
]]
function Files.writeSidecar(doc_settings)
    local location = G_reader_settings:readSetting("document_metadata_folder", "doc")
    local dirs = {}
    if location == "doc" then
        -- Beside the book, or KOReader's own folder when the book's is read-only.
        dirs = { doc_settings.doc_sidecar_dir, doc_settings.dir_sidecar_dir }
    elseif location == "dir" then
        dirs = { doc_settings.dir_sidecar_dir }
    elseif doc_settings.getSidecarDir then
        dirs = { doc_settings.hash_sidecar_dir or doc_settings:getSidecarDir(doc_settings.data.doc_path, "hash") }
    end
    if not (dirs[1] and doc_settings.sidecar_filename and doc_settings.purge) then
        return doc_settings:flush()
    end
    local err
    for __, dir in ipairs(dirs) do
        util.makePath(dir)
        local sidecar = dir .. "/" .. doc_settings.sidecar_filename
        local ok
        ok, err = Files.write(sidecar, doc_settings.data, sidecar)
        if ok then
            Files.moveCustomFiles(doc_settings, dir)
            -- Copies in other locations would shadow this one.
            doc_settings:purge(sidecar)
            return dir
        end
    end
    -- Not flush, which would empty the sidecar where the write just failed.
    return nil, err
end

-- moveCustomFiles moves a book's custom cover and custom metadata into dir,
-- where its sidecar now is, as DocSettings:flush does. One left behind would
-- shadow a newer one KOReader writes in dir. Each old copy goes only once
-- its replacement is written.
function Files.moveCustomFiles(doc_settings, dir)
    for __, get in ipairs({ "getCustomCoverFile", "getCustomMetadataFile" }) do
        local file = doc_settings[get] and doc_settings[get](doc_settings)
        if file then
            local filepath, filename = util.splitFilePathName(file)
            if filepath ~= dir .. "/" and Files.copy(file, dir .. "/" .. filename) then
                os.remove(file)
                doc_settings[get](doc_settings, true) -- KOReader's cache of where it is
            end
        end
    end
end

-- Writer writes data to path only when it changed since the last write, for
-- a file that can hold every book on the device.
local Writer = {}
Writer.__index = Writer

function Files.writer(path, comment)
    return setmetatable({ path = path, comment = comment }, Writer)
end

function Writer:write(data)
    local content = dump(data, nil, true)
    if content == self.written then return true end
    local ok, err = Files.write(self.path, data, self.comment)
    if ok then self.written = content end
    return ok, err
end

return Files
