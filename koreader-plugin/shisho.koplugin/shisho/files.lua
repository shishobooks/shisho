--[[--
Writes the plugin's own files in KOReader's settings directory as Lua files
LuaSettings reads, atomically: through a temporary file beside each, renamed
over the old one, so a crash or a dead battery leaves the old file or the
new one, never half of either. No UI.
]]
local dump = require("dump")
local ffiutil = require("ffi/util")

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
