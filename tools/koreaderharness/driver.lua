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
    -- "page" or "scroll", KOReader's page and continuous views. Each run
    -- starts in the default, so a command that reads the position names the
    -- view it was read in.
    if args.view and ui.rolling then
        local Event = require("ui/event")
        ui:handleEvent(Event:new("SetViewMode", args.view))
        stepUI()
    end
    -- KOReader skips a pull or a queued push within 25 seconds of its last,
    -- timed from when the machine booted and starting from 0. So for the
    -- first 25 seconds after a boot it skips them all, silently, and Docker
    -- Desktop boots its VM afresh after it idles. Each command is a new
    -- KOReader that has never synced, so nothing is recent.
    if ui.kosync then
        ui.kosync.pull_timestamp = -math.huge
        ui.kosync.push_timestamp = -math.huge
    end
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
    elseif args.percent and args.view == "scroll" and ui.rolling then
        -- Continuous view has no pages to count; this is its own go-to-percent.
        ui:handleEvent(Event:new("GotoPercent", args.percent))
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

-- Every word start in a reflowable book, as crengine writes its XPointer, with
-- the text from there to the next one: what tests of XPointer translation
-- expect, taken from KOReader rather than written by hand.
commands.xpointers = function()
    runWithReader(function(ui)
        local document = ui.document
        if document.info.has_pages then fail("xpointers needs a reflowable book") end
        local words = {}
        local xp = document:getNextVisibleWordStart("/body/DocFragment[1]")
        -- The first word may be where the document starts.
        local first = document:getPrevVisibleWordStart(xp)
        if first and document:compareXPointers(first, xp) == 1 then xp = first end
        while xp and #words < 5000 do
            local next_xp = document:getNextVisibleWordStart(xp)
            if not next_xp or document:compareXPointers(xp, next_xp) ~= 1 then
                table.insert(words, { xpointer = xp })
                break
            end
            table.insert(words, { xpointer = xp, text = document:getTextFromXPointers(xp, next_xp) })
            xp = next_xp
        end
        return { words = words }
    end)
end

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
    -- What the server answered, if KOReader asked, so a pull that does
    -- nothing says why: no request at all, a failed one, or an answer
    -- KOReader chose to ignore.
    local received
    runWithReader(function(ui, kosync)
        -- The plugin's modules load with the reader.
        local KOSyncClient = require("KOSyncClient")
        local get_progress = KOSyncClient.get_progress
        KOSyncClient.get_progress = function(client, username, userkey, document, callback)
            return get_progress(client, username, userkey, document, function(ok, body)
                received = { ok = ok, body = body }
                callback(ok, body)
            end)
        end
        -- 1 is prompt and 2 is silent, the plugin's SYNC_STRATEGY.
        kosync.settings.sync_forward = mode == "prompt" and 1 or 2
        kosync.settings.sync_backward = mode == "prompt" and 1 or 2
        -- Interactive is the menu's Pull progress; otherwise it's the pull an
        -- open or a wake does, which is the path that prompts.
        kosync:getProgress(false, mode == "interactive")
        stepUI()
        local out = { prompted = confirm_box and confirm_box.text or nil, received = received or false }
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

-- The KOReader plugin ---------------------------------------------------------

-- What the plugin and KOReader's OPDS catalog list hold, read back from
-- their files as a person inspecting the device would.
local function pluginReport(plugin, extra)
    local LuaSettings = require("luasettings")
    local report = extra or {}
    local state = LuaSettings:open(DataStorage:getSettingsDir() .. "/shisho.lua").data
    report.servers = state.servers or {}
    report.journal = type(state.journal) == "table" and type(state.journal.entries) == "table"
        and #state.journal.entries or 0
    report.installed_from = state.installed_from
    report.pairing_handled = state.pairing_handled
    if plugin then
        -- Read from disk, so an install shows the plugin KOReader loads next.
        local stamps = dofile(plugin.path .. "/shisho/stamps.lua").load(plugin.path)
        report.version = stamps.version
        report.hash = stamps.hash
        report.offer = plugin.offer
        if plugin.sign_in_dialog then
            report.sign_in_url = plugin.sign_in_dialog.shisho_prefilled_url or ""
        end
    end
    local opds = LuaSettings:open(DataStorage:getSettingsDir() .. "/opds.lua").data
    return report, opds.servers or {}
end

-- withPlugin starts KOReader's file manager with the plugin loaded, online
-- or not, runs fn, and reports. KOReader's restart is recorded, not done.
-- Wi-Fi's NetworkConnected reaches the plugin before fn, as at launch, or
-- with network_after, KOReader starts offline and Wi-Fi comes on just before
-- fn, which runs before the event arrives, as when a menu item turns Wi-Fi
-- on and runs its action before KOReader's event loop delivers the event.
-- stubNetwork makes Wi-Fi whatever net.online says, which the caller can
-- change. Loading NetworkMgr broadcasts NetworkConnected when the machine
-- running the container is online, so that's dropped, and commands deliver
-- the event themselves.
local function stubNetwork(online)
    local net = { online = online and true or false }
    local broadcast = UIManager.broadcastEvent
    UIManager.broadcastEvent = function(ui, event)
        if event and event.handler == "onNetworkConnected" then return end
        return broadcast(ui, event)
    end
    local NetworkMgr = require("ui/network/manager")
    NetworkMgr.isOnline = function() return net.online end
    NetworkMgr.isConnected = function() return net.online end
    NetworkMgr.isWifiOn = function() return net.online end
    NetworkMgr.runWhenOnline = function(_, callback) if net.online then callback() end end
    NetworkMgr.runWhenConnected = NetworkMgr.runWhenOnline
    NetworkMgr.willRerunWhenOnline = function() return not net.online end
    return net
end

local function withPlugin(fn, network_after)
    local net = stubNetwork(args.online and not network_after)
    local online = net.online
    local restart = false
    UIManager.askForRestart = function() restart = true end
    UIManager.restartKOReader = function() restart = true end

    local FileManager = require("apps/filemanager/filemanager")
    FileManager:showFiles("/books")
    stepUI()
    local fm = FileManager.instance
    local plugin = fm and fm.shisho
    if not plugin then fail("the KOReader plugin didn't load") end
    if online and not network_after then
        plugin:onNetworkConnected()
        stepUI()
    end
    if args.online and network_after then
        online = true
        net.online = true
    end
    local extra = fn and fn(plugin) or {}
    stepUI()
    if online and network_after then
        plugin:onNetworkConnected()
        stepUI()
    end
    screenshot(args.screenshot)
    extra.restart = restart
    extra.prompted = confirm_box and confirm_box.text or nil
    local report, opds = pluginReport(plugin, extra)
    G_reader_settings:flush()
    respond({ ok = true, plugin = report, opds = opds, shown = shown })
    os.exit(0, true)
end

commands["plugin-launch"] = function()
    withPlugin()
end

commands["plugin-signin"] = function()
    withPlugin(function(plugin)
        local ok, outcome, message
        if args.code then
            ok, outcome, message = plugin:signInWithCode(args.server, args.code)
        else
            ok, outcome, message = plugin:signInWithPassword(args.server, args.username, args.password)
        end
        return { signed_in = ok, outcome = outcome, message = message }
    end)
end

-- The Shisho menu under Tools, as KOReader's touch menu shows it, with what
-- each item reads.
commands["plugin-menu"] = function()
    withPlugin(function(plugin)
        local entries = {}
        plugin:addToMainMenu(entries)
        local items = entries.shisho.sub_item_table_func()
        -- A submenu's items read "Parent > Item".
        local texts = {}
        -- Items with nothing to show when held, which KOReader shows as help.
        local without_help = {}
        local function note(text, item)
            table.insert(texts, text)
            if not (item.help_text or item.help_text_func) then
                table.insert(without_help, text)
            end
        end
        for __, item in ipairs(items) do
            local text = item.text_func and item.text_func() or item.text
            note(text, item)
            for __, sub in ipairs(item.sub_item_table or {}) do
                note(text .. " > " .. (sub.text_func and sub.text_func() or sub.text), sub)
            end
        end
        -- The file manager's own Tools tab, as KOReader sorts it.
        local tools = {}
        local menu = plugin.ui.menu
        menu:setUpdateItemTable()
        for __, tab in ipairs(menu.tab_item_table) do
            if tab.icon == "appbar.tools" then
                for __, item in ipairs(tab) do
                    table.insert(tools, item.text_func and item.text_func() or item.text or "----")
                end
            end
        end
        local TouchMenu = require("ui/widget/touchmenu")
        UIManager:show(TouchMenu:new{
            width = Screen:getWidth(),
            tab_item_table = { { text = entries.shisho.text, icon = "appbar.tools", table.unpack(items) } },
            show_parent = plugin.ui,
        })
        -- KOReader's JSON writes an empty table as an object, so none is nil.
        return { menu = texts, tools = tools, without_help = #without_help > 0 and without_help or nil }
    end)
end

commands["plugin-update"] = function()
    withPlugin(function(plugin)
        if args.from then
            local ok, message = plugin:installFrom(args.from)
            return { installed = ok, message = message }
        end
        -- Wi-Fi coming on already checked; this is the menu's check.
        confirm_box = nil
        plugin:checkForUpdate(true)
        stepUI()
        local out = { prompted_update = confirm_box and confirm_box.text or nil }
        if confirm_box and args.confirm then
            confirm_box.ok_callback()
            stepUI()
        end
        return out
    end, true)
end

-- seedStatistics adds rows to KOReader's statistics database, as reading
-- does: args.stats is a list of { document, page, total_pages, start_time,
-- duration }, each book's row found or made by its document hash. The
-- statistics plugin made the database when KOReader started.
local function seedStatistics()
    if type(args.stats) ~= "table" then return end
    local SQ3 = require("lua-ljsqlite3/init")
    local conn = SQ3.open(DataStorage:getSettingsDir() .. "/statistics.sqlite3")
    local books = {}
    local find = conn:prepare("SELECT id FROM book WHERE md5 = ?;")
    local add_book = conn:prepare("INSERT INTO book (title, authors, md5, pages, last_open) VALUES (?, 'Harness', ?, ?, ?);")
    local add_row = conn:prepare("INSERT OR IGNORE INTO page_stat_data VALUES (?, ?, ?, ?, ?);")
    for __, row in ipairs(args.stats) do
        local id = books[row.document]
        if not id then
            local found = find:reset():bind(row.document):step()
            if found then
                id = tonumber(found[1])
            else
                add_book:reset():bind("Book " .. row.document, row.document, row.total_pages, row.start_time):step()
                id = tonumber(conn:rowexec("SELECT last_insert_rowid();"))
            end
            books[row.document] = id
        end
        add_row:reset():bind(id, row.page, row.start_time, row.duration, row.total_pages):step()
    end
    conn:close()
end

-- plugin-sync seeds statistics, with Wi-Fi off, then turns Wi-Fi on, which
-- syncs by itself. With sync_now, it taps Sync now first instead.
commands["plugin-sync"] = function()
    withPlugin(function(plugin)
        if args.statistics_off then
            G_reader_settings:saveSetting("statistics", { is_enabled = false })
        end
        seedStatistics()
        if args.sync_now then
            -- Sync now, as the menu runs it: only once Wi-Fi is on.
            local problems
            require("ui/network/manager"):runWhenOnline(function() problems = plugin:syncAll(true) end)
            -- An empty table encodes as an object, not a list.
            return { problems = problems and #problems > 0 and problems or nil }
        end
        return {}
    end, not args.sync_now)
end

-- plugin-sleep seeds statistics with Wi-Fi off, turns on sync on sleep, and
-- puts the device to sleep. Wi-Fi comes on when asked to, as KOReader's
-- "turn on" Wi-Fi action does, and the report says whether it was turned on
-- and whether it was left on.
commands["plugin-sleep"] = function()
    withPlugin(function(plugin)
        seedStatistics()
        -- KOReader turns Wi-Fi on by itself only when told to; it asks by default.
        G_reader_settings:saveSetting("wifi_enable_action", args.wifi_prompt and "prompt" or "turn_on")
        plugin:saveSetting("sync_on_sleep", true)
        local NetworkMgr = require("ui/network/manager")
        local turned_on = false
        NetworkMgr.willRerunWhenOnline = function(_, callback)
            if NetworkMgr:isOnline() then return false end
            turned_on = true
            NetworkMgr.isOnline = function() return true end
            NetworkMgr.isConnected = NetworkMgr.isOnline
            NetworkMgr.isWifiOn = NetworkMgr.isOnline
            callback()
            return true
        end
        NetworkMgr.disableWifi = function()
            NetworkMgr.isOnline = function() return false end
            NetworkMgr.isConnected = NetworkMgr.isOnline
            NetworkMgr.isWifiOn = NetworkMgr.isOnline
        end
        plugin:onSuspend()
        return { wifi_turned_on = turned_on, wifi_left_on = NetworkMgr:isWifiOn() }
    end)
end

-- plugin-daily turns on daily sync, puts the device to sleep, and reports the
-- wake-up alarm the plugin set. With fire, the alarm then goes off as a Kobo's
-- does: the plugin's task runs, KOReader removes it, and the device stays
-- awake a while before sleeping again. Wi-Fi comes on when asked to.
commands["plugin-daily"] = function()
    withPlugin(function(plugin)
        seedStatistics()
        -- KOReader turns Wi-Fi on by itself only when told to; it asks by default.
        G_reader_settings:saveSetting("wifi_enable_action", args.wifi_prompt and "prompt" or "turn_on")
        plugin:saveSetting("daily_sync", true)
        local NetworkMgr = require("ui/network/manager")
        local turned_on = false
        local function setOnline(online)
            NetworkMgr.isOnline = function() return online end
            NetworkMgr.isConnected = NetworkMgr.isOnline
            NetworkMgr.isWifiOn = NetworkMgr.isOnline
        end
        NetworkMgr.goOnlineToRun = function(_, callback)
            if not NetworkMgr:isOnline() then turned_on = true; setOnline(true) end
            callback()
            return true
        end
        NetworkMgr.disableWifi = function() setOnline(false) end
        -- The device's wake-up alarms, as KOReader's WakeupMgr keeps them.
        local Device = require("device")
        local tasks = {}
        Device.wakeup_mgr = {
            addTask = function(_, seconds, callback)
                table.insert(tasks, { epoch = os.time() + seconds, callback = callback })
                table.sort(tasks, function(a, b) return a.epoch < b.epoch end)
            end,
            removeTasks = function(_, epoch, callback)
                for i = #tasks, 1, -1 do
                    if (not epoch or tasks[i].epoch == epoch) and (not callback or tasks[i].callback == callback) then
                        table.remove(tasks, i)
                    end
                end
            end,
        }
        local function alarms()
            local out = {}
            for __, task in ipairs(tasks) do table.insert(out, task.epoch) end
            return #out > 0 and out or nil
        end
        plugin:onSuspend()
        local armed = alarms()
        if args.fire and tasks[1] then
            tasks[1].callback()
            table.remove(tasks, 1)
            stepUI()
        end
        return { alarms_armed = armed, alarms_after = alarms(), wifi_turned_on = turned_on, wifi_left_on = NetworkMgr:isWifiOn() }
    end)
end

-- plugin-open opens a book in the reader with the plugin loaded, online or
-- not: opening pulls its position. Then it can go to a place, and tap Sync
-- now.
commands["plugin-open"] = function()
    stubNetwork(args.online)
    runWithReader(function(ui)
        local plugin = ui.shisho
        if not plugin then fail("the KOReader plugin didn't load") end
        stepUI()
        local out = { prompted = confirm_box and confirm_box.text or nil }
        if confirm_box and args.confirm then
            confirm_box.ok_callback()
            stepUI()
        end
        if args.page or args.xpointer or args.percent then
            gotoCommand(ui)
            stepUI()
        end
        if args.sync_now then
            plugin:syncAll(true)
            stepUI()
        end
        out.plugin = pluginReport(plugin)
        return out
    end)
end

local command = commands[args.command]
if not command then fail("unknown command: " .. tostring(args.command)) end
local ok, err = xpcall(command, debug.traceback)
if not ok then fail(err) end
