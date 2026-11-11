-- Runs the KOReader plugin's Lua unit tests with KOReader's own luajit and
-- modules, so they test against KOReader's real settings, JSON, and file
-- utilities rather than mocks. Each spec file under /spec returns a table of
-- test functions by name. Prints one JSON object, as driver.lua does.
--
-- Run from KOReader's directory: ./luajit luatest.lua

io.stdout:setvbuf("no")
local real_stdout = io.stdout

require("setupkoenv")

local DataStorage = require("datastorage")
local lfs = require("libs/libkoreader-lfs")
lfs.mkdir(DataStorage:getDataDir() .. "/settings")

require("dbg"):turnOff()
local logger = require("logger")
logger:setLevel(logger.levels.err)

G_defaults = require("luadefaults"):open(DataStorage:getDataDir() .. "/defaults.custom.lua")
G_reader_settings = require("luasettings"):open(DataStorage:getDataDir() .. "/settings.reader.lua")

package.path = "/plugin/shisho.koplugin/?.lua;" .. package.path

local JSON = require("json")

-- Helpers every spec can use.
local function describe(value)
    if type(value) == "table" then
        return JSON.encode(value)
    end
    return tostring(value)
end

local function deepEqual(a, b)
    if type(a) ~= type(b) then return false end
    if type(a) ~= "table" then return a == b end
    for k, v in pairs(a) do
        if not deepEqual(v, b[k]) then return false end
    end
    for k in pairs(b) do
        if a[k] == nil then return false end
    end
    return true
end

function assert_eq(actual, expected, message) -- luacheck: ignore
    if not deepEqual(actual, expected) then
        error(string.format("%sexpected %s, got %s", message and (message .. ": ") or "", describe(expected), describe(actual)), 2)
    end
end

-- A fresh directory for one test's files.
local scratch_count = 0
function scratch_dir() -- luacheck: ignore
    scratch_count = scratch_count + 1
    local dir = string.format("%s/spec-scratch-%d", DataStorage:getDataDir(), scratch_count)
    require("ffi/util").purgeDir(dir)
    lfs.mkdir(dir)
    return dir
end

local files = {}
for entry in lfs.dir("/spec") do
    if entry:match("_spec%.lua$") then
        table.insert(files, entry)
    end
end
table.sort(files)

local passed, failed = 0, {}
for __, file in ipairs(files) do
    local ok, spec = pcall(dofile, "/spec/" .. file)
    if not ok or type(spec) ~= "table" then
        table.insert(failed, { name = file, error = tostring(spec) })
    else
        local names = {}
        for name in pairs(spec) do table.insert(names, name) end
        table.sort(names)
        for __, name in ipairs(names) do
            local test_ok, err = xpcall(spec[name], debug.traceback)
            if test_ok then
                passed = passed + 1
            else
                table.insert(failed, { name = file .. ": " .. name, error = tostring(err) })
            end
        end
    end
end

real_stdout:write("\n@@RESULT@@", JSON.encode({ ok = #failed == 0, passed = passed, failed = failed }), "\n")
real_stdout:flush()
os.exit(#failed == 0 and 0 or 1, true)
