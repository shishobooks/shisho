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

-- write writes data to path, with comment as its first line.
function Files.write(path, data, comment)
    local tmp = path .. ".tmp"
    local file, err = io.open(tmp, "wb")
    if not file then
        return nil, err
    end
    local written, write_err = file:write("-- ", comment, "\nreturn ", dump(data, nil, true), "\n")
    if written then
        ffiutil.fsyncOpenedFile(file)
    end
    local closed, close_err = file:close()
    if not written or not closed then
        -- A short write, as on a full disk, must never replace a good file.
        os.remove(tmp)
        return nil, write_err or close_err
    end
    local ok, rename_err = os.rename(tmp, path)
    if not ok then
        os.remove(tmp)
        return nil, rename_err
    end
    ffiutil.fsyncDirectory(path)
    return true
end

--[[--
writeSidecar saves doc_settings, a book's DocSettings, where
DocSettings:flush would, but through Files.write. flush truncates the
sidecar and then writes it, and DocSettings:open deletes a sidecar it finds
empty or cut short. The file browser's cover extraction runs in a process
of its own that opens each new book's sidecar, so one opening it mid-write
would delete it, and everything in it. It reads DocSettings fields that
aren't KOReader's API, so without them, or when no location takes the file,
it saves with flush after all. Returns the sidecar's directory, or what
flush returns.
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
    if doc_settings.sidecar_filename and doc_settings.purge then
        for __, dir in ipairs(dirs) do
            util.makePath(dir)
            local sidecar = dir .. "/" .. doc_settings.sidecar_filename
            if Files.write(sidecar, doc_settings.data, sidecar) then
                -- Copies in other locations would shadow this one.
                doc_settings:purge(sidecar)
                return dir
            end
        end
    end
    return doc_settings:flush()
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
