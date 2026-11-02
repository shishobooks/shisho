-- Drives a real KOReader headless, the way KOReader's own unit tests start it
-- (spec/unit/commonrequire.lua): a dummy framebuffer, dummy input, and a
-- ReaderUI with its plugins loaded. Each run does one command and prints one
-- JSON object. Reading state lasts between runs in KOReader's own settings
-- and sidecars under $HOME.
--
-- Run from KOReader's directory: ./luajit driver.lua '{"command": "open", ...}'

io.stdout:setvbuf("no")
local real_stdout = io.stdout

require("setupkoenv")

local DataStorage = require("datastorage")
local lfs = require("libs/libkoreader-lfs")
lfs.mkdir(DataStorage:getDataDir() .. "/settings")
lfs.mkdir(DataStorage:getDataDir() .. "/docsettings")

require("dbg"):turnOff()
local logger = require("logger")
logger:setLevel(logger.levels.err)

G_defaults = require("luadefaults"):open(DataStorage:getDataDir() .. "/defaults.custom.lua")
G_reader_settings = require("luasettings"):open(DataStorage:getDataDir() .. "/settings.reader.lua")
if not G_reader_settings:has("device_id") then
    G_reader_settings:saveSetting("device_id", require("frontend/random").uuid())
end
-- Sidecars beside the books would land in read-only mounts and the books
-- directory, so they go in KOReader's data directory.
G_reader_settings:saveSetting("document_metadata_folder", "dir")

einkfb = require("ffi/framebuffer") --luacheck: ignore
einkfb.dummy = true --luacheck: ignore

local Device = require("device")
local Screen = Device.screen
Screen:init()
require("document/canvascontext"):init(Device)
Device.input.dummy = true

local JSON = require("json")
local args = JSON.decode(arg[1])

local function respond(result)
    real_stdout:write("\n@@RESULT@@", JSON.encode(result), "\n")
    real_stdout:flush()
end

local function fail(message)
    respond({ ok = false, error = tostring(message) })
    os.exit(1, true)
end

-- Records what KOReader shows, as a person on the device would read it.
local UIManager = require("ui/uimanager")
local shown = {}
local confirm_box
local real_show = UIManager.show
UIManager.show = function(ui, widget, ...)
    if widget and widget.text then
        table.insert(shown, widget.text)
    end
    if widget and widget.ok_callback then
        confirm_box = widget
    end
    return real_show(ui, widget, ...)
end

-- Runs what KOReader has scheduled and repaints, as its event loop would.
-- Timeouts shift back so scheduled work runs now, and dismissing is left to
-- the caller, so a pop-up is still on screen for a screenshot.
local function stepUI()
    UIManager:shiftScheduledTasksBy(-1e9)
    UIManager:setInputTimeout(0)
    pcall(function() UIManager:handleInput() end)
    UIManager:forceRePaint()
end

local function screenshot(path)
    if not path then return end
    stepUI()
    Screen:shot(path)
end

local function withReader(fn)
    if not args.file then fail("--file is required") end
    if lfs.attributes(args.file, "mode") ~= "file" then fail("no such file: " .. args.file) end
    local DocumentRegistry = require("document/documentregistry")
    local ReaderUI = require("apps/reader/readerui")
    local provider = DocumentRegistry:getProvider(args.file)
    local document = DocumentRegistry:openDocument(args.file, provider)
    if not document then fail("KOReader could not open " .. args.file) end
    local ui = ReaderUI:new{ dimen = Screen:getSize(), document = document }
    UIManager:show(ui)
    stepUI()
    local out = fn(ui, ui.kosync) or {}
    return ui, out
end

local function state(ui)
    local kosync = ui.kosync
    local pages = ui.document.info.has_pages
    local page_count = ui.document:getPageCount()
    return {
        hash = kosync:getDocumentDigest(),
        progress = tostring(kosync:getLastProgress()),
        percentage = kosync:getLastPercent(),
        page = ui:getCurrentPage(),
        page_count = page_count,
        page_based = pages and true or false,
    }
end

local function finish(ui, out)
    out.ok = true
    for k, v in pairs(state(ui)) do
        if out[k] == nil then out[k] = v end
    end
    screenshot(args.screenshot)
    out.shown = shown
    ui:saveSettings()
    if ui.kosync then ui.kosync:onFlushSettings() end
    G_reader_settings:flush()
    respond(out)
    os.exit(0, true)
end

local function runWithReader(fn)
    local ui, out = withReader(fn)
    finish(ui, out)
end

local function gotoCommand(ui)
    local Event = require("ui/event")
    if args.xpointer then
        ui:handleEvent(Event:new("GotoXPointer", args.xpointer))
    elseif args.page then
        ui:handleEvent(Event:new("GotoPage", args.page))
    elseif args.percent then
        -- Page 1 is 0%, and the last page is 100%, as KOReader's skim dialog
        -- and its reported percentage have it.
        local count = ui.document:getPageCount()
        local page = math.max(1, math.min(count, math.floor(count * args.percent / 100 + 0.5)))
        ui:handleEvent(Event:new("GotoPage", page))
    else
        fail("goto needs --page, --percent, or --xpointer")
    end
    stepUI()
    -- A page turn is what makes KOReader note the time it compares a pull to.
    if ui.kosync then ui.kosync.last_page_turn_timestamp = os.time() end
end

local commands = {}

commands.digest = function()
    if not args.file then fail("--file is required") end
    respond({ ok = true, hash = require("util").partialMD5(args.file) })
    os.exit(0, true)
end

commands.download = function()
    local http = require("socket.http")
    local ltn12 = require("ltn12")
    local mime = require("mime")
    local socketutil = require("socketutil")
    local output = assert(io.open(args.output, "wb"))
    local headers = {}
    if args.username then
        headers["Authorization"] = "Basic " .. mime.b64(args.username .. ":" .. (args.password or ""))
    end
    socketutil:set_timeout(60, 300)
    local _, status = http.request{
        url = args.url,
        headers = headers,
        sink = ltn12.sink.file(output),
    }
    socketutil:reset_timeout()
    if status ~= 200 then
        os.remove(args.output)
        fail("download returned " .. tostring(status))
    end
    respond({ ok = true, path = args.output, hash = require("util").partialMD5(args.output) })
    os.exit(0, true)
end

commands.open = function()
    runWithReader(function() end)
end

commands["goto"] = function()
    runWithReader(function(ui) gotoCommand(ui) end)
end

commands.screenshot = commands.open

local function accountCommand(call)
    if not args.file then fail("--file is required") end
    runWithReader(function(_, kosync)
        kosync.settings.custom_server = args.server or kosync.settings.custom_server
        call(kosync)
        kosync.updated = true
        return { logged_in = kosync.settings.userkey ~= nil and kosync.settings.username == args.username }
    end)
end

commands.login = function()
    accountCommand(function(kosync)
        kosync.settings.username = nil
        kosync.settings.userkey = nil
        kosync:doLogin(args.username, args.password)
    end)
end

commands.register = function()
    accountCommand(function(kosync)
        kosync.settings.username = nil
        kosync.settings.userkey = nil
        kosync:doRegister(args.username, args.password)
    end)
end

commands.push = function()
    runWithReader(function(_, kosync)
        kosync:updateProgress(false, true)
    end)
end

commands.pull = function()
    local mode = args.mode or "silent"
    runWithReader(function(ui, kosync)
        -- 1 is prompt and 2 is silent, the plugin's SYNC_STRATEGY.
        kosync.settings.sync_forward = mode == "prompt" and 1 or 2
        kosync.settings.sync_backward = mode == "prompt" and 1 or 2
        -- Interactive is the menu's Pull progress; otherwise it's the pull an
        -- open or a wake does, which is the path that prompts.
        kosync:getProgress(false, mode == "interactive")
        stepUI()
        local out = { prompted = confirm_box and confirm_box.text or nil }
        if confirm_box and args.confirm then
            confirm_box.ok_callback()
            stepUI()
        end
        return out
    end)
end

commands["queue-push"] = function()
    runWithReader(function(_, kosync)
        local server = kosync.settings.custom_server
        -- Nothing listens on this port, as with a device out of Wi-Fi range.
        kosync.settings.custom_server = "http://127.0.0.1:1"
        kosync:updateProgress(false, false)
        kosync.settings.custom_server = server
        kosync.push_timestamp = 0
        return { queued = require("KOSyncQueue"):count() }
    end)
end

commands.drain = function()
    runWithReader(function(_, kosync)
        local queue = require("KOSyncQueue")
        local before = queue:count()
        kosync:drainQueue()
        return { drained = before - queue:count(), queued = queue:count() }
    end)
end

local command = commands[args.command]
if not command then fail("unknown command: " .. tostring(args.command)) end
local ok, err = xpcall(command, debug.traceback)
if not ok then fail(err) end
