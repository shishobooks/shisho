-- Installing a plugin zip over the running plugin.
local Archiver = require("ffi/archiver")
local Installer = require("shisho/installer")
local lfs = require("libs/libkoreader-lfs")

local function writeZip(path, files)
    local writer = Archiver.Writer:new()
    assert(writer:open(path, "zip"))
    for name, content in pairs(files) do
        assert(writer:addFileFromMemory(name, content))
    end
    writer:close()
end

local function read(path)
    local file = io.open(path, "rb")
    if not file then return nil end
    local content = file:read("*a")
    file:close()
    return content
end

local function write(path, content)
    local file = assert(io.open(path, "wb"))
    file:write(content)
    file:close()
end

-- installed is a plugin directory holding the old plugin.
local function installed()
    local dir = scratch_dir()
    local plugin = dir .. "/shisho.koplugin"
    lfs.mkdir(plugin)
    write(plugin .. "/main.lua", "old")
    write(plugin .. "/stale.lua", "old")
    return dir, plugin
end

return {
    ["replaces the plugin with the zip's"] = function()
        local dir, plugin = installed()
        writeZip(dir .. "/new.zip", {
            ["shisho.koplugin/main.lua"] = "new",
            ["shisho.koplugin/shisho/module.lua"] = "module",
        })

        assert_eq(Installer.install(dir .. "/new.zip", plugin), true)

        assert_eq(read(plugin .. "/main.lua"), "new")
        assert_eq(read(plugin .. "/shisho/module.lua"), "module")
        assert_eq(read(plugin .. "/stale.lua"), nil, "the old plugin's files are gone")
        assert_eq(lfs.attributes(dir .. "/.shisho-update", "mode"), nil)
        assert_eq(lfs.attributes(dir .. "/.shisho-retired", "mode"), nil)
    end,

    ["leaves the plugin whole when the zip is broken"] = function()
        local dir, plugin = installed()
        write(dir .. "/broken.zip", "not a zip")

        local ok = Installer.install(dir .. "/broken.zip", plugin)

        assert_eq(ok, nil)
        assert_eq(read(plugin .. "/main.lua"), "old")
    end,

    ["leaves the plugin whole when the zip holds no plugin"] = function()
        local dir, plugin = installed()
        writeZip(dir .. "/empty.zip", { ["shisho.koplugin/readme.txt"] = "no main.lua" })

        local ok, err = Installer.install(dir .. "/empty.zip", plugin)

        assert_eq(ok, nil)
        assert_eq(err, "the zip holds no plugin")
        assert_eq(read(plugin .. "/main.lua"), "old")
    end,

    ["refuses files outside the plugin"] = function()
        local dir, plugin = installed()
        writeZip(dir .. "/escape.zip", {
            ["shisho.koplugin/main.lua"] = "new",
            ["shisho.koplugin/../escaped.lua"] = "bad",
        })
        writeZip(dir .. "/other.zip", {
            ["shisho.koplugin/main.lua"] = "new",
            ["other.koplugin/main.lua"] = "bad",
        })

        assert_eq(Installer.install(dir .. "/escape.zip", plugin), nil)
        assert_eq(Installer.install(dir .. "/other.zip", plugin), nil)
        assert_eq(read(plugin .. "/main.lua"), "old")
        assert_eq(read(dir .. "/escaped.lua"), nil)
        assert_eq(lfs.attributes(dir .. "/other.koplugin", "mode"), nil)
    end,
}
