--[[--
Installs a plugin zip over the running plugin. The zip holds one directory,
shisho.koplugin. It's unpacked beside the plugin first, and only a complete
unpack replaces the plugin, so a failure partway leaves the old plugin
whole. KOReader loads the new one when it restarts.
]]
local Archiver = require("ffi/archiver")
local ffiutil = require("ffi/util")
local lfs = require("libs/libkoreader-lfs")
local util = require("util")

local Installer = {}

local ROOT = "shisho.koplugin/"

local function writeFile(path, content)
    local file, err = io.open(path, "wb")
    if not file then return nil, err end
    file:write(content)
    file:close()
    return true
end

-- install unpacks zip_path over the plugin at plugin_path.
function Installer.install(zip_path, plugin_path)
    local parent = plugin_path:match("^(.*)/[^/]+/?$")
    if not parent then
        return nil, "no plugin directory"
    end
    local staging = parent .. "/.shisho-update"
    local retired = parent .. "/.shisho-retired"
    ffiutil.purgeDir(staging)
    lfs.mkdir(staging)

    local reader = Archiver.Reader:new()
    if not reader:open(zip_path) then
        return nil, "not a zip"
    end
    local files = {}
    for entry in reader:iterate() do
        if entry.mode == "file" then
            table.insert(files, entry.path)
        end
    end
    for __, path in ipairs(files) do
        if path:sub(1, #ROOT) ~= ROOT or path:find("%.%.") then
            reader:close()
            ffiutil.purgeDir(staging)
            return nil, "unexpected file in the zip: " .. path
        end
        local content = reader:extractToMemory(path)
        if not content then
            reader:close()
            ffiutil.purgeDir(staging)
            return nil, "couldn't unpack " .. path
        end
        local dest = staging .. "/" .. path
        util.makePath(dest:match("^(.*)/[^/]+$"))
        local ok, err = writeFile(dest, content)
        if not ok then
            reader:close()
            ffiutil.purgeDir(staging)
            return nil, err
        end
    end
    reader:close()

    local unpacked = staging .. "/shisho.koplugin"
    if lfs.attributes(unpacked .. "/main.lua", "mode") ~= "file" then
        ffiutil.purgeDir(staging)
        return nil, "the zip holds no plugin"
    end
    ffiutil.purgeDir(retired)
    local ok, err = os.rename(plugin_path, retired)
    if not ok then
        ffiutil.purgeDir(staging)
        return nil, err
    end
    ok, err = os.rename(unpacked, plugin_path)
    if not ok then
        os.rename(retired, plugin_path)
        ffiutil.purgeDir(staging)
        return nil, err
    end
    ffiutil.purgeDir(retired)
    ffiutil.purgeDir(staging)
    return true
end

return Installer
