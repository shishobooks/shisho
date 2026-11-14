--[[--
The KOReader plugin for Shisho (ADR 0016). This file is the thin layer:
KOReader's events, menus, dialogs, Wi-Fi, and calls to the server. Decisions
live in the shisho/ modules, which have no UI and are unit tested.

It signs in to Shisho servers, adds their catalogs to KOReader's OPDS
catalog list, keeps itself up to date from them, syncs reading time from
KOReader's statistics and positions to each of them, and syncs each book's
status and Rating both ways.
]]
local Client = require("shisho/client")
local ConfirmBox = require("ui/widget/confirmbox")
local DataStorage = require("datastorage")
local DateTimeWidget = require("ui/widget/datetimewidget")
local Daily = require("shisho/daily")
local Device = require("device")
local DocSettings = require("docsettings")
local Event = require("ui/event")
local HashIndex = require("shisho/hashindex")
local InfoMessage = require("ui/widget/infomessage")
local Installer = require("shisho/installer")
local Journal = require("shisho/journal")
local LuaSettings = require("luasettings")
local Math = require("optmath")
local MultiInputDialog = require("ui/widget/multiinputdialog")
local NetworkMgr = require("ui/network/manager")
local Opds = require("shisho/opds")
local Positions = require("shisho/positions")
local Responses = require("shisho/responses")
local Stamps = require("shisho/stamps")
local State = require("shisho/state")
local Statistics = require("shisho/statistics")
local Summaries = require("shisho/summaries")
local Sync = require("shisho/sync")
local UIManager = require("ui/uimanager")
local Urls = require("shisho/urls")
local Versions = require("shisho/versions")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local lfs = require("libs/libkoreader-lfs")
local logger = require("logger")
local util = require("util")
local T = require("ffi/util").template
local _ = require("gettext")

-- KOReader lists a menu item it has no place for after More tools, which on
-- an e-reader is usually the Tools tab's second page. So the plugin adds
-- itself to the top of Tools in both menus' order before either is built.
for __, module in ipairs({ "ui/elements/filemanager_menu_order", "ui/elements/reader_menu_order" }) do
    local ok, order = pcall(require, module)
    if ok and type(order) == "table" and type(order.tools) == "table" then
        local listed = false
        for __, id in ipairs(order.tools) do
            if id == "shisho" then listed = true end
        end
        if not listed then
            table.insert(order.tools, 1, "shisho")
        end
    end
end

local NONE_ENABLED = _("No Shisho server is enabled. Turn one on with Enabled under it in the Shisho menu.")

local Shisho = WidgetContainer:extend{
    name = "shisho",
    is_doc_only = false,
    state_file = DataStorage:getSettingsDir() .. "/shisho.lua",
    summaries_file = DataStorage:getSettingsDir() .. "/shisho_summaries.lua",
    hashes_file = DataStorage:getSettingsDir() .. "/shisho_hashes.lua",
}

function Shisho:init()
    self.stamps = Stamps.load(self.path)
    self.state = State.open(self.state_file)
    self.journal = Journal.new(self.state)
    self.summaries = Summaries.open(self.summaries_file)
    self.hashes = HashIndex.open(self.hashes_file)
    self:scheduleDaily()
    self.ui.menu:registerToMainMenu(self)

    -- A zip from a Device's page signs itself in on first launch. Once its
    -- code has expired, sign-in opens with its server filled in.
    local action = self:pairingAction()
    if action == "ask" then
        UIManager:nextTick(function()
            self:markPairingHandled()
            self:showSignIn(self.stamps.pairing.server_url, _("This plugin's pairing code has expired. Press Pair on the Device's page in Shisho and enter the new code."))
        end)
    elseif action == "sign_in" and NetworkMgr:isOnline() then
        UIManager:nextTick(function() self:pairFromZip() end)
    end
end

function Shisho:installed()
    return {
        version = self.stamps.version,
        hash = self.stamps.hash,
        installed_from = self.state:get("installed_from"),
    }
end

function Shisho:client(server)
    return Client.new(server.url, self.stamps.version, server.username, server.credential)
end

function Shisho:notify(text, timeout)
    UIManager:show(InfoMessage:new{ text = text, timeout = timeout })
end

-- Pairing from the zip --------------------------------------------------

function Shisho:pairingAction()
    local pairing = self.stamps.pairing
    local signed_in = pairing and self.state:find(pairing.server_url) ~= nil
    return Stamps.pairingState(pairing, self.state:get("pairing_handled"), signed_in, os.time())
end

function Shisho:markPairingHandled()
    self.state:set("pairing_handled", self.stamps.pairing.short_code or true)
    self.state:flush()
end

function Shisho:pairFromZip()
    if self:pairingAction() ~= "sign_in" then return end
    local pairing = self.stamps.pairing
    local ok, outcome, message = self:signInWithCode(pairing.server_url, pairing.short_code)
    if outcome == "unreachable" then
        -- Try again the next time Wi-Fi comes on.
        return
    end
    self:markPairingHandled()
    if ok then
        -- A zip from a Device's page was installed from its server.
        self.state:set("installed_from", Urls.normalize(pairing.server_url))
        self.state:flush()
    else
        self:showSignIn(pairing.server_url, message)
    end
end

-- Signing in --------------------------------------------------------------

-- signedIn records a sign-in, adds the server's catalog to KOReader's OPDS
-- list, and tells the user.
function Shisho:signedIn(url, answer, username, credential)
    local server, other_device = self.state:signIn{
        -- A server signed in to for the first time takes reading from now on.
        stats_cursor = Statistics.currentCursor(self:statisticsPath()),
        url = url,
        username = username,
        credential = credential,
        device_id = answer.device_id,
        device_name = answer.device_name,
        info = answer.server,
    }
    if other_device then
        -- Another Device's statuses and Ratings say nothing about this one's.
        self.summaries:remove(server.url)
        self.summaries:flush()
    end
    -- A zip from this server's Device page was installed from it, however
    -- the sign-in went once its code expired.
    local pairing = self.stamps.pairing
    if not self.state:get("installed_from") and pairing and Urls.same(pairing.server_url, server.url) then
        self.state:set("installed_from", server.url)
    end
    self.state:flush()
    self:scheduleDaily()
    self:writeCatalog(server)
    self:notify(T(_("Signed in to %1 as %2."), Urls.label(server.url), server.device_name or username), 3)
    return server
end

-- trySignIn calls request with each URL what the person typed may mean, and
-- stops at the first that answers. Returns the URL, the outcome, and the
-- status and answer.
local function trySignIn(typed, request)
    local url, outcome, status, body
    for __, candidate in ipairs(Urls.candidates(typed)) do
        url = candidate
        status, body = request(candidate)
        outcome = Responses.classify(status, body)
        if outcome ~= "unreachable" then break end
    end
    return url, outcome, body
end

-- signInWithCode exchanges a Device short code for the Device's sign-in.
-- Returns ok, the outcome, and a message for the user.
function Shisho:signInWithCode(url, code)
    if not Urls.normalize(url) or not code or code == "" then
        return false, "error", _("Enter the server's URL and the pairing code.")
    end
    local server_url, outcome, body = trySignIn(url, function(candidate)
        return Client.new(candidate, self.stamps.version):pair(code)
    end)
    if outcome ~= "ok" then
        return false, outcome, Responses.message(outcome, body)
    end
    self:signedIn(server_url, body, body.username, body.credential)
    return true, outcome
end

-- signInWithPassword signs in with a username and the Device's password.
function Shisho:signInWithPassword(url, username, password)
    if not Urls.normalize(url) or not username or username == "" or not password or password == "" then
        return false, "error", _("Enter the server's URL, your username, and the Device's password.")
    end
    local server_url, outcome, body = trySignIn(url, function(candidate)
        return Client.new(candidate, self.stamps.version, username, password):status()
    end)
    if outcome ~= "ok" then
        return false, outcome, Responses.message(outcome, body)
    end
    self:signedIn(server_url, body, username, password)
    return true, outcome
end

function Shisho:showSignIn(url, message)
    local dialog
    dialog = MultiInputDialog:new{
        title = _("Sign in to Shisho"),
        description = message,
        fields = {
            { description = _("Server URL"), text = url or "", hint = "https://books.example.com" },
            { description = _("Pairing code"), text = "", hint = _("From Pair on the Device's page") },
        },
        buttons = {{
            {
                text = _("Cancel"),
                id = "close",
                callback = function() UIManager:close(dialog) end,
            },
            {
                text = _("Use password"),
                callback = function()
                    local fields = dialog:getFields()
                    UIManager:close(dialog)
                    self:showPasswordSignIn(fields[1])
                end,
            },
            {
                text = _("Sign in"),
                is_enter_default = true,
                callback = function()
                    local fields = dialog:getFields()
                    UIManager:close(dialog)
                    NetworkMgr:runWhenOnline(function()
                        local ok, _outcome, err = self:signInWithCode(fields[1], fields[2])
                        if not ok then self:showSignIn(fields[1], err) end
                    end)
                end,
            },
        }},
    }
    dialog.shisho_prefilled_url = url
    self.sign_in_dialog = dialog
    UIManager:show(dialog)
    dialog:onShowKeyboard()
end

function Shisho:showPasswordSignIn(url, message)
    local dialog
    dialog = MultiInputDialog:new{
        title = _("Sign in to Shisho"),
        description = message or _("Use your Shisho username and the password shown on the Device's page."),
        fields = {
            { description = _("Server URL"), text = url or "", hint = "https://books.example.com" },
            { description = _("Username"), text = "" },
            { description = _("Device password"), text = "", text_type = "password" },
        },
        buttons = {{
            {
                text = _("Cancel"),
                id = "close",
                callback = function() UIManager:close(dialog) end,
            },
            {
                text = _("Sign in"),
                is_enter_default = true,
                callback = function()
                    local fields = dialog:getFields()
                    UIManager:close(dialog)
                    NetworkMgr:runWhenOnline(function()
                        local ok, _outcome, err = self:signInWithPassword(fields[1], fields[2], fields[3])
                        if not ok then self:showPasswordSignIn(fields[1], err) end
                    end)
                end,
            },
        }},
    }
    self.sign_in_dialog = dialog
    UIManager:show(dialog)
    dialog:onShowKeyboard()
end

-- The OPDS catalog list -----------------------------------------------------

-- writeCatalog adds the server's catalog to KOReader's OPDS list, or updates
-- the entry already there for its URL. The list is the OPDS plugin's own, so
-- it goes through that plugin when it's loaded, and its file otherwise.
function Shisho:writeCatalog(server)
    local entry = {
        title = T(_("Shisho (%1)"), Urls.label(server.url)),
        url = Opds.catalogURL(server.url),
        username = server.username,
        password = server.credential,
    }
    local opds = self.ui and self.ui.opds
    if opds and opds.loadSettings then
        opds:loadSettings()
        Opds.upsert(opds.servers, entry)
        opds.updated = true
        opds:onFlushSettings()
        return
    end
    -- With the OPDS plugin off, its list is changed only once it exists:
    -- writing one now would replace the catalogs KOReader starts it with.
    local settings = LuaSettings:open(DataStorage:getSettingsDir() .. "/opds.lua")
    local servers = settings:readSetting("servers")
    if type(servers) ~= "table" then
        logger.info("Shisho: KOReader's OPDS catalog list doesn't exist yet; not adding", entry.url)
        return
    end
    Opds.upsert(servers, entry)
    settings:saveSetting("servers", servers)
    settings:flush()
end

-- Checking in and updating --------------------------------------------------

-- checkIn asks every signed-in server about itself, noting which need a new
-- sign-in or a newer plugin. In the background it gives up on a server
-- quickly.
function Shisho:checkIn(background)
    for __, server in ipairs(self.state:enabledServers()) do
        local client = Client.new(server.url, self.stamps.version, server.username, server.credential,
            background and Client.BACKGROUND or nil)
        local status, body, headers = client:status()
        Responses.checkIn(server, status, body, headers)
    end
    self.state:flush()
end

-- checkForUpdate offers an update when a server has one for every server
-- signed in. Before any sign-in there's nothing to check.
function Shisho:checkForUpdate(interactive)
    -- Check for plugin update can turn Wi-Fi on and run before a paired
    -- zip's own sign-in, which waits for Wi-Fi's event, so it signs in first.
    local dialog_before = self.sign_in_dialog
    if self:pairingAction() == "sign_in" then
        self:pairFromZip()
    end
    if #self.state:servers() == 0 then
        -- A failed sign-in from the zip already opened sign-in.
        if interactive and self.sign_in_dialog == dialog_before then
            self:showSignIn(self.stamps.pairing and self.stamps.pairing.server_url or "",
                _("Sign in to a Shisho server to check for plugin updates."))
        end
        return nil
    end
    if #self.state:enabledServers() == 0 then
        if interactive then self:notify(NONE_ENABLED) end
        return nil
    end
    self.checked_at = os.time()
    self:checkIn(not interactive)
    -- Checking yourself also offers another server's build of this version.
    local offer, blocked = Versions.offer(self:installed(), self.state:enabledServers(), { any_build = interactive })
    self.offer = offer
    -- An offer declined once isn't pressed again until something newer
    -- comes along, or the person checks themselves.
    if offer and not interactive and offer.hash == self.state:get("declined_offer") then
        return offer, blocked
    end
    if offer then
        local text = T(_("Shisho KOReader plugin %1 is available from %2. Install it and restart KOReader?"), offer.version, Urls.label(offer.server))
        if Versions.compare(offer.version, self.stamps.version) == 0 then
            text = T(_("A new build of the Shisho KOReader plugin %1 is available from %2. Install it and restart KOReader?"), offer.version, Urls.label(offer.server))
        end
        UIManager:show(ConfirmBox:new{
            text = text,
            ok_text = _("Install"),
            ok_callback = function() self:installFrom(offer.server) end,
            cancel_callback = function()
                self.state:set("declined_offer", offer.hash)
                self.state:flush()
            end,
        })
    elseif blocked and interactive then
        self:notify(T(_("Plugin %1 from %2 doesn't work with %3. Upgrade Shisho on %3 first."), blocked.version, Urls.label(blocked.server), Urls.label(blocked.upgrade)))
    elseif interactive then
        -- A server that needs a new sign-in offers nothing, so it can't be
        -- called up to date.
        local signed_out = {}
        for __, server in ipairs(self.state:enabledServers()) do
            if server.needs_login then table.insert(signed_out, Urls.label(server.url)) end
        end
        if #signed_out > 0 then
            self:notify(T(_("Sign in again to %1 to check it for plugin updates."), table.concat(signed_out, ", ")))
        else
            self:notify(_("The Shisho KOReader plugin is up to date."), 3)
        end
    end
    return offer, blocked
end

-- installFrom installs the plugin the server at url serves, makes it the
-- server the plugin was installed from, and asks KOReader to restart.
function Shisho:installFrom(url)
    local choice = Versions.choose(self.state:servers(), url)
    if not choice then
        return false, _("Not signed in to that server.")
    end
    local server = self.state:find(choice.server)
    local zip = DataStorage:getDataDir() .. "/cache/shisho-plugin.zip"
    require("util").makePath(DataStorage:getDataDir() .. "/cache")
    local status, download_err = self:client(server):download(zip)
    if status ~= 200 then
        local message = download_err and T(_("Couldn't save the plugin: %1"), tostring(download_err))
            or Responses.message(Responses.classify(status), nil, server.info)
        self:notify(message)
        return false, message
    end
    local ok, err = Installer.install(zip, self.path)
    os.remove(zip)
    if not ok then
        logger.warn("Shisho: installing the plugin failed:", err)
        local message = T(_("Couldn't install the plugin: %1"), tostring(err))
        self:notify(message)
        return false, message
    end
    self.state:set("installed_from", server.url)
    self.state:flush()
    -- Until KOReader restarts, the old plugin runs and would offer the
    -- build just installed again.
    self.awaiting_restart = true
    UIManager:askForRestart(_("The Shisho KOReader plugin is updated. Restart KOReader to use it."))
    return true
end

-- Events ----------------------------------------------------------------------

-- onNetworkConnected runs a moment after the event, so the rest of
-- KOReader handles Wi-Fi coming on first.
-- A check that ran within RECHECK_AFTER seconds, such as the one Check for
-- plugin update just ran after turning Wi-Fi on, isn't repeated.
local RECHECK_AFTER = 60

function Shisho:onNetworkConnected()
    -- Wi-Fi a sync while away turned on is known now, before that sync turns
    -- it off again.
    local background = self:inBackgroundSync()
    UIManager:scheduleIn(1, function()
        if self:pairingAction() == "sign_in" then
            self:pairFromZip()
        end
        -- A clock set back since the check doesn't count as recent.
        local since = self.checked_at and os.time() - self.checked_at
        local actions = Sync.onConnected{
            servers = #self.state:enabledServers(),
            background = background or self:inBackgroundSync(),
            screen_saver = Device.screen_saver_mode and true or false,
            recent_check = since and since >= 0 and since < RECHECK_AFTER or false,
            ran_recently = Sync.ranRecently(self.synced_at, os.time()),
            awaiting_restart = self.awaiting_restart and true or false,
        }
        if actions.check then
            self:checkForUpdate(false)
        end
        -- Whatever turned Wi-Fi on, what's waiting goes out.
        if actions.sync then
            self:syncAll(false)
        end
    end)
end

-- inBackgroundSync reports whether a sync while away turned Wi-Fi on a
-- moment ago. KOReader reports the connection up to a few seconds later.
function Shisho:inBackgroundSync()
    return self.background_until ~= nil and os.time() <= self.background_until
end

-- The reader -------------------------------------------------------------------

-- How long after the last page turn the place is written to the journal.
local RECORD_AFTER = 10

function Shisho:onReaderReady()
    -- Opening a book lands on a page, which isn't moving: only page turns
    -- from here on are.
    self.reader_ready = true
    self.last_page = self.ui.getCurrentPage and self.ui:getCurrentPage()
    self.last_page_turn = nil
    -- The status as the book opens, so a change while it's open is seen.
    self.open_status = self:openSummary()
    -- Opening a book never waits for Wi-Fi.
    if #self.state:enabledServers() > 0 and NetworkMgr:isOnline() then
        UIManager:nextTick(function() self:pullOpenBook(false) end)
    end
end

function Shisho:onPageUpdate(page)
    if not self.reader_ready or page == nil or page == self.last_page then return end
    self:observeStatus()
    self.last_page = page
    self.last_page_turn = os.time()
    self.pages_since_sync = (self.pages_since_sync or 0) + 1
    -- Whether Wi-Fi is connected is cheap to ask; whether it reaches the
    -- internet is a lookup, which waits only once there's a connection.
    local action = Sync.afterPage(self.pages_since_sync, Sync.dueAfterPages(self.pages_since_sync) and NetworkMgr:isConnected())
    if action == "reset" then
        self.pages_since_sync = 0
    elseif action == "sync" then
        -- After the page is drawn, so the turn itself isn't held up.
        UIManager:nextTick(function() self:syncWhileOnline() end)
    end
    if not self.record_task then
        self.record_task = function() self:recordPosition(true) end
    end
    UIManager:unschedule(self.record_task)
    UIManager:scheduleIn(RECORD_AFTER, self.record_task)
end

function Shisho:onCloseDocument()
    if self.record_task then UIManager:unschedule(self.record_task) end
    self:observeStatus()
    self:recordPosition(true)
    self.reader_ready = nil
    self:syncWhileOnline()
end

--[[--
syncWhileOnline syncs on Wi-Fi that's already on, as a book closes, the
device sleeps, or every few pages, so a device that stays online keeps
Shisho current. It never turns Wi-Fi on, doesn't repeat a sync that just ran,
and pulls no position: a question mid-read, or as a book closes, would only
get in the way.
]]
function Shisho:syncWhileOnline()
    if #self.state:enabledServers() == 0 then return end
    if Sync.ranRecently(self.synced_at, os.time()) then return end
    if not (NetworkMgr:isConnected() and NetworkMgr:isOnline()) then return end
    -- A server that just failed is left out for a while, so one that's down
    -- doesn't hold up reading every few pages.
    self:syncAll(false, true, true)
end

function Shisho:onSuspend()
    if self.record_task then UIManager:unschedule(self.record_task) end
    self:observeStatus()
    self:recordPosition(true)
    self:armDailyWakeup()
    local servers = self.state:enabledServers()
    local now = os.time()
    -- With Wi-Fi left on, as on Android, going to sleep syncs at no cost. (A
    -- Kobo's Wi-Fi is already off by now: KOReader turns it off first.)
    -- Otherwise, sync on sleep turns Wi-Fi on, at most once an hour.
    local action = Sync.onSleep{
        online = #servers > 0 and NetworkMgr:isConnected() and NetworkMgr:isOnline() or false,
        syncable = #Sync.syncable(servers),
        sync_on_sleep = self:settings().sync_on_sleep,
        last_synced = Sync.lastSynced(servers),
        last_attempt = self.state:get("sleep_sync_attempted"),
        wifi_turns_on = self:wifiTurnsOnByItself(),
        now = now,
    }
    if action == "sync" then
        self:syncWhileOnline()
    elseif action == "wifi" then
        -- Counted when tried, so a server that's down doesn't turn Wi-Fi on
        -- at every sleep. It pulls no position: a question would wait on a
        -- sleeping screen. Wi-Fi it turned on goes off again before the
        -- device sleeps: a Kobo can fail to sleep, or hang, with Wi-Fi on.
        self.state:set("sleep_sync_attempted", now)
        self.state:flush()
        self:syncInBackground(true)
    end
end

-- WIFI_ASKS is what to tell someone turning on a sync that happens while
-- they're away, when KOReader would ask before turning Wi-Fi on.
local WIFI_ASKS = _("KOReader asks before turning Wi-Fi on, so this sync only happens when Wi-Fi is already on. To let it turn Wi-Fi on by itself, set Settings > Network > Action when Wi-Fi is off to Turn on.")

-- wifiTurnsOnByItself reports whether KOReader turns Wi-Fi on without
-- asking. A sync while the person is away, as the device goes to sleep or at
-- the daily time, never leaves a question on the screen.
function Shisho:wifiTurnsOnByItself()
    return G_reader_settings:readSetting("wifi_enable_action") == "turn_on"
end

--[[--
syncInBackground syncs while the person is away: on Wi-Fi already on, or on
Wi-Fi it turns on and off again, when KOReader turns it on without asking.
Going to sleep, it pulls no position, since a question would wait on a
sleeping screen.
]]
function Shisho:syncInBackground(going_to_sleep)
    if NetworkMgr:isConnected() and NetworkMgr:isOnline() then
        self:syncAll(false, going_to_sleep)
        return
    end
    if not self:wifiTurnsOnByItself() then return end
    local was_on = self:wifiWasOn()
    self.background_until = os.time() + RECHECK_AFTER
    NetworkMgr:willRerunWhenOnline(function()
        self:syncAll(false, going_to_sleep)
        self:turnWifiOff(was_on)
    end)
end

-- wifiWasOn is KOReader's note that Wi-Fi was on, which it uses to turn
-- Wi-Fi back on as the device wakes. Turning Wi-Fi on sets it.
function Shisho:wifiWasOn()
    return { setting = G_reader_settings:isTrue("wifi_was_on"), manager = NetworkMgr.wifi_was_on }
end

-- turnWifiOff turns off Wi-Fi a sync while away turned on, and puts back
-- KOReader's note of whether Wi-Fi was on as it was before, so someone who
-- keeps Wi-Fi off doesn't find it turned on at every wake afterwards.
function Shisho:turnWifiOff(was_on)
    local function restore()
        if was_on.setting then
            G_reader_settings:makeTrue("wifi_was_on")
        else
            G_reader_settings:makeFalse("wifi_was_on")
        end
        NetworkMgr.wifi_was_on = was_on.manager
    end
    NetworkMgr:disableWifi(restore)
    restore()
end

-- documentHash is the open book's document hash, as KOReader keeps it.
function Shisho:documentHash()
    local doc_settings = self.ui and self.ui.document and self.ui.doc_settings
    return doc_settings and doc_settings:readSetting("partial_md5_checksum")
end

-- currentPlace is where the reader is in the open book, in the form
-- KOReader's progress sync sends: an XPointer, or the 1-based page.
function Shisho:currentPlace()
    local ui = self.ui
    if not (ui and ui.document) then return nil end
    local view = ui.document.info.has_pages and ui.paging or ui.rolling
    if not view then return nil end
    return {
        progress = tostring(view:getLastProgress()),
        percentage = Math.roundPercent(view:getLastPercent()),
    }
end

-- recordPosition journals the open book's place, timed by the last page
-- turn, for every signed-in server.
function Shisho:recordPosition(flush)
    if #self.state:enabledServers() == 0 or not self.last_page_turn then return end
    local document, place = self:documentHash(), self:currentPlace()
    if not document or not place then return end
    if self.journal:recordPosition(document, place.progress, place.percentage, self.last_page_turn) and flush then
        self.state:flush()
    end
end

function Shisho:goToProgress(progress)
    if self.ui.document.info.has_pages then
        self.ui:handleEvent(Event:new("GotoPage", tonumber(progress)))
    else
        self.ui:handleEvent(Event:new("GotoXPointer", progress))
    end
end

-- pullOpenBook asks every signed-in server for the open book's position,
-- and goes to the newest, asks, or leaves it, by the settings.
function Shisho:pullOpenBook(interactive)
    local document = self:documentHash()
    local here = self:currentPlace()
    if not document or not here then return end
    local candidates = {}
    for __, server in ipairs(self.state:enabledServers()) do
        if not server.needs_login then
            local client = Client.new(server.url, self.stamps.version, server.username, server.credential,
                not interactive and Client.BACKGROUND or nil)
            local status, body = client:pull({ document })
            if status == 200 and type(body) == "table" and type(body.positions) == "table" then
                for __, position in ipairs(body.positions) do
                    if position.document == document then table.insert(candidates, position) end
                end
            end
        end
    end
    local remote = Positions.newest(candidates)
    here.at = Positions.localInstant(self.journal:lastPositionAt(document), self.last_page_turn)
    local action, direction = Positions.decide(remote, here, self.state:get("settings"))
    if action == "ignore" then return action end
    local percent = Math.round((tonumber(remote.percentage) or 0) * 100)
    local source = remote.source or _("another reader")
    if action == "jump" then
        self:goToProgress(remote.progress)
        self:notify(T(_("Went to %1% from %2."), percent, source), 3)
        return action
    end
    local text = T(_("Go to %1% from %2? It's newer than your place on this device."), percent, source)
    if direction == "older" then
        text = T(_("Go back to %1% from %2? It's older than your place on this device."), percent, source)
    end
    UIManager:show(ConfirmBox:new{
        text = text,
        ok_text = _("Go"),
        ok_callback = function() self:goToProgress(remote.progress) end,
    })
    return action
end

-- Statuses and Ratings ---------------------------------------------------------
-- KOReader fires no event when a book's status or Rating changes, so each
-- sync compares the books' sidecars with what was last synced
-- (shisho/summaries). While a book is open, the plugin also watches its
-- status and stars, so a book finished at the end-of-book prompt or in Book
-- status goes with the moment it happened, not just the day.

-- openSummary is the open book's summary, as KOReader holds it in memory.
function Shisho:openSummary()
    local doc_settings = self.ui and self.ui.document and self.ui.doc_settings
    return doc_settings and Summaries.read(doc_settings:readSetting("summary"))
end

-- observeStatus notes when the open book's status or stars changed since it
-- opened, or since it was last looked at.
function Shisho:observeStatus()
    if not self.reader_ready then return end
    local document, now = self:documentHash(), self:openSummary()
    if not document or not now then return end
    local before = self.open_status
    self.open_status = now
    if Summaries.observe(self.summaries:seen(), document, before, now, os.time()) then
        self.summaries:flush()
    end
end

-- onEndOfBook looks at the status once KOReader has handled the end of the
-- book, which marks it finished when set to.
function Shisho:onEndOfBook()
    UIManager:nextTick(function() self:observeStatus() end)
end

-- hashOf is a book file's document hash: the one its sidecar records, as
-- KOReader computed it when the book was first opened, or else its own.
local function hashOf(path)
    if DocSettings:hasSidecarFile(path) then
        local recorded = DocSettings:open(path):readSetting("partial_md5_checksum")
        if recorded then return recorded end
    end
    return util.partialMD5(path)
end

-- refreshHashes brings the index of book files on the device up to date
-- when it's due (HashIndex.due), and reports whether it scanned.
function Shisho:refreshHashes(interactive)
    local now = os.time()
    if not HashIndex.due(self.hashes:scannedAt(), now, interactive) then return false end
    local roots = HashIndex.roots{
        storage = Device.home_dir,
        home = G_reader_settings:readSetting("home_dir"),
        download = G_reader_settings:readSetting("download_dir"),
        ereader = Device:isKobo() or Device:isKindle() or Device:isPocketBook() or Device:isCervantes() or Device:isRemarkable(),
    }
    if #roots == 0 then return false end
    local ok, err = pcall(function()
        self.hashes:refresh(HashIndex.scan(roots), hashOf, now)
    end)
    if not ok then
        logger.warn("Shisho: indexing books failed:", err)
        return false
    end
    self.hashes:flush()
    return true
end

-- readSidecar returns the document hash a book's sidecar records and its
-- summary, reading a sidecar again only once it changed on disk.
function Shisho:readSidecar(path)
    local sidecar = DocSettings:findSidecarFile(path)
    if not sidecar then return nil, nil end
    local mtime = lfs.attributes(sidecar, "modification")
    self.sidecars = self.sidecars or {}
    local cached = self.sidecars[path]
    if not cached or cached.sidecar ~= sidecar or cached.mtime ~= mtime then
        local doc_settings = DocSettings:open(path)
        cached = {
            sidecar = sidecar,
            mtime = mtime,
            recorded = doc_settings:readSetting("partial_md5_checksum"),
            summary = doc_settings:readSetting("summary"),
        }
        self.sidecars[path] = cached
    end
    return cached.recorded, cached.summary
end

--[[--
localBooks is every book whose status and Rating sync, by document hash
(Summaries.addBook): the open book, the books in KOReader's history, and
those in the hash index. With read_index false, the indexed books' sidecars
are left unread: syncs while reading look only at what was read lately.
]]
function Shisho:localBooks(read_index)
    local books, paths = {}, {}
    local index = self.hashes:files()
    local function indexed(path) return index[path] and index[path].hash end
    local open = self.ui and self.ui.document
    if open and self.ui.doc_settings then
        paths[open.file] = true
        Summaries.addBook(books, open.file, self.ui.doc_settings:readSetting("partial_md5_checksum"),
            indexed(open.file), self.ui.doc_settings:readSetting("summary"))
    end
    for __, item in ipairs(require("readhistory").hist) do
        local path = item.file
        if path and not paths[path] and not item.dim then
            paths[path] = true
            local recorded, summary = self:readSidecar(path)
            if recorded or indexed(path) then
                Summaries.addBook(books, path, recorded, indexed(path), summary)
            end
        end
    end
    for path, entry in pairs(index) do
        if not paths[path] then
            if read_index then
                local recorded, summary = self:readSidecar(path)
                Summaries.addBook(books, path, recorded, entry.hash, summary)
            else
                Summaries.addBook(books, path, nil, entry.hash, nil, true)
            end
        end
    end
    return books
end

-- writeSummary writes what a server sent into a book's sidecar, as
-- KOReader's file manager does when it marks a book: a book never opened
-- gets a sidecar.
function Shisho:writeSummary(book, write)
    local open = self.ui and self.ui.document and self.ui.document.file == book.path
    local doc_settings = open and self.ui.doc_settings or DocSettings:open(book.path)
    local summary = Summaries.written(doc_settings:readSetting("summary"), write, os.date("%Y-%m-%d"))
    doc_settings:saveSetting("summary", summary)
    doc_settings:flush()
    book.current = Summaries.read(summary)
    self.summaries_written = true
    if open then
        -- Written by the server, not changed by the reader.
        self.open_status = book.current
    end
    local ok, BookList = pcall(require, "ui/widget/booklist")
    if ok and BookList.setBookInfoCacheProperty then
        if write.status then BookList.setBookInfoCacheProperty(book.path, "status", summary.status) end
        if write.rating then BookList.setBookInfoCacheProperty(book.path, "rating", summary.rating or 0) end
    end
end

--[[--
syncStates sends server the status and Rating changes of books, through
run(changes, store), then pulls what the device should hold and writes it.
Returns the outcome.
]]
function Shisho:syncStates(server, client, books, run)
    local store = self.summaries:server(server.url)
    local changes = Summaries.collect(store, books, {
        signed_in_day = State.signedInDay(server, os.date("%Y-%m-%d")),
        today = os.date("%Y-%m-%d"),
        seen = self.summaries:seen(),
    })
    local outcome = run(changes, store)
    if outcome == "ok" then
        outcome = Sync.pullStates{
            server = server,
            store = store,
            documents = Summaries.documents(books),
            post = function(payload) return client:pullStates(payload) end,
            write = function(pulled, outright)
                local book = books[pulled.document]
                if not book then return end
                if book.unread then
                    local _recorded, summary = self:readSidecar(book.path)
                    book.current, book.unread = Summaries.read(summary), nil
                end
                local write = Summaries.apply(store.synced, pulled, book.current, outright)
                if write then self:writeSummary(book, write) end
            end,
        }
    end
    self.summaries:flush()
    return outcome
end

-- Daily sync ------------------------------------------------------------------
-- While KOReader is awake, a timer runs daily sync. While the device sleeps,
-- a wake-up alarm does, on a device that has them (Kobo, some Kindles): it
-- wakes, the plugin syncs, and KOReader puts it back to sleep.

function Shisho:dailySettings()
    return Daily.settings(self.state:get("settings"))
end

-- dailyLastRun is the time daily sync last ran for, kept in the state so a
-- restart can't run one day's sync twice.
function Shisho:dailyLastRun()
    return self.state:get("daily_last_run")
end

-- scheduleDaily sets the timer for daily sync's next run. It keeps running
-- while the device shows its sleep screen without sleeping, as some Kobos
-- do while charging, where the wake-up alarm never goes off.
function Shisho:scheduleDaily()
    if self.daily_task then UIManager:unschedule(self.daily_task) end
    local now = os.time()
    local seconds = Daily.secondsToNext(self.state:get("settings"), #self.state:enabledServers(), now, self:dailyLastRun())
    if not seconds then return end
    local target = now + seconds
    self.daily_task = function() self:runDailySync(target) end
    UIManager:scheduleIn(seconds, self.daily_task)
end

-- armDailyWakeup sets the wake-up alarm for daily sync's next run as the
-- device goes to sleep, since a timer doesn't run while it's asleep. It sets
-- none when waking couldn't sync.
function Shisho:armDailyWakeup()
    self:removeDailyWakeup()
    if not Device.wakeup_mgr then return end
    local now = os.time()
    local seconds = Daily.alarmIn(self.state:get("settings"), #self.state:enabledServers(),
        self:wifiTurnsOnByItself(), now, self:dailyLastRun())
    if not seconds then return end
    local target = now + seconds
    self.daily_wakeup = function() self:runDailySync(target) end
    Device.wakeup_mgr:addTask(seconds, self.daily_wakeup)
end

function Shisho:removeDailyWakeup()
    if Device.wakeup_mgr and self.daily_wakeup then
        Device.wakeup_mgr:removeTasks(nil, self.daily_wakeup)
    end
end

function Shisho:onResume()
    self:removeDailyWakeup()
    self:scheduleDaily()
end

function Shisho:onCloseWidget()
    if self.daily_task then UIManager:unschedule(self.daily_task) end
end

--[[--
runDailySync runs the daily sync due at target, once: the timer and the
wake-up alarm both aim at it, and whichever goes first claims it. Behind the
sleep screen, woken by the alarm or on a device that didn't sleep, it has
the moment before KOReader puts the device back to sleep, so it waits for
Wi-Fi and syncs before returning, turns Wi-Fi off, and leaves nothing on the
screen. Awake, it syncs as any sync while away does. Neither pulls a
position: no one is there to answer a question. Then it sets the next day's.
]]
function Shisho:runDailySync(target)
    if Daily.claim(target, self:dailyLastRun()) then
        self.state:set("daily_last_run", target)
        self.state:flush()
        if Device.screen_saver_mode then
            self:syncBehindSleepScreen()
        else
            self:syncInBackground(true)
        end
    end
    -- After KOReader removes the alarm that ran this, so the new one stays.
    UIManager:nextTick(function()
        if Device.screen_saver_mode then self:armDailyWakeup() end
        self:scheduleDaily()
    end)
end

-- syncBehindSleepScreen syncs while the sleep screen shows, waiting for
-- Wi-Fi, and turns Wi-Fi off again. A failure to connect leaves no message
-- on the sleep screen, where it would stay until someone woke the device.
function Shisho:syncBehindSleepScreen()
    if NetworkMgr:isConnected() and NetworkMgr:isOnline() then
        self:syncAll(false, true)
        return
    end
    if not self:wifiTurnsOnByItself() then return end
    local was_on = self:wifiWasOn()
    local top = UIManager:getTopmostVisibleWidget()
    self.background_until = os.time() + RECHECK_AFTER
    local ok, err = pcall(function()
        NetworkMgr:goOnlineToRun(function() self:syncAll(false, true) end)
    end)
    if not ok then logger.warn("Shisho: daily sync failed:", err) end
    local left = UIManager:getTopmostVisibleWidget()
    if left and left ~= top and getmetatable(left) and getmetatable(left).__index == InfoMessage then
        UIManager:close(left)
    end
    -- A Kobo can't sleep with Wi-Fi on.
    self:turnWifiOff(was_on)
end

function Shisho:showDailyTime()
    local daily = self:dailySettings()
    UIManager:show(DateTimeWidget:new{
        hour = daily.hour,
        min = daily.min,
        title_text = _("Daily sync time"),
        ok_text = _("Set time"),
        callback = function(time)
            self:saveSettings{ daily_sync_hour = time.hour, daily_sync_min = time.min }
        end,
    })
end

-- Syncing -----------------------------------------------------------------------

local STATISTICS_OFF = _("Reading statistics are off in KOReader, so reading time won't sync. Positions still do.")

function Shisho:settings()
    return Positions.settings(self.state:get("settings"))
end

function Shisho:saveSetting(key, value)
    self:saveSettings{ [key] = value }
end

-- saveSettings saves the settings in values and sets daily sync's timer
-- from them.
function Shisho:saveSettings(values)
    local saved = self.state:get("settings")
    if type(saved) ~= "table" then
        saved = {}
        self.state:set("settings", saved)
    end
    for key, value in pairs(values) do saved[key] = value end
    self.state:flush()
    self:scheduleDaily()
end

-- flushStatistics asks the statistics plugin to write the pages it still
-- holds in memory, since the plugin's sleep handler can run before its own.
function Shisho:flushStatistics()
    local statistics = self.ui and self.ui.statistics
    if statistics and statistics.insertDB then
        local ok, err = pcall(statistics.insertDB, statistics)
        if not ok then logger.warn("Shisho: flushing statistics failed:", err) end
    end
end

function Shisho:statisticsPath()
    return DataStorage:getSettingsDir() .. "/statistics.sqlite3"
end

function Shisho:statisticsEnabled()
    return Statistics.enabled(G_reader_settings:readSetting("statistics"), G_reader_settings:readSetting("plugins_disabled"))
end

--[[--
syncAll sends every signed-in server what it hasn't acknowledged, then pulls
the open book's position, except when going to sleep. One server failing
doesn't stop the others. A server that needs a new sign-in keeps everything
queued until it gets one. Interactive, from Sync now, it says how it went.
]]
function Shisho:syncAll(interactive, going_to_sleep, skip_failing)
    if self.syncing then return end
    if #self.state:servers() == 0 then
        if interactive then
            self:showSignIn(self.stamps.pairing and self.stamps.pairing.server_url or "",
                _("Sign in to a Shisho server to sync."))
        end
        return
    end
    if #self.state:enabledServers() == 0 then
        if interactive then self:notify(NONE_ENABLED) end
        return
    end
    self.syncing = true
    self.pages_since_sync = 0
    local ok, problems, statistics_on = pcall(self.syncServers, self, interactive, skip_failing)
    self.syncing = false
    self.synced_at = os.time()
    if self.summaries_written then
        -- The file browser, History, and Collections show the statuses and
        -- stars written, as they do after KOReader's own Book information.
        self.summaries_written = nil
        UIManager:broadcastEvent(Event:new("BookMetadataChanged"))
    end
    if not ok then
        logger.warn("Shisho: syncing failed:", problems)
        problems, statistics_on = { _("Couldn't sync. Try again later.") }, true
    end
    if not going_to_sleep then
        local pulled, err = pcall(self.pullOpenBook, self, interactive)
        if not pulled then logger.warn("Shisho: pulling the position failed:", err) end
    end
    if interactive then
        local lines = {}
        if #problems == 0 then
            table.insert(lines, _("Synced with Shisho."))
        else
            for __, problem in ipairs(problems) do table.insert(lines, problem) end
        end
        if not statistics_on then
            table.insert(lines, STATISTICS_OFF)
        end
        self:notify(table.concat(lines, "\n"), #problems == 0 and statistics_on and 3 or nil)
    end
    return problems
end

-- syncServers runs the sync for each signed-in server, returning what went
-- wrong and whether statistics is on. With skip_failing, a server that just
-- failed is left out.
function Shisho:syncServers(interactive, skip_failing)
    self:observeStatus()
    self:recordPosition(false)
    -- A sync while reading doesn't walk the device's folders, or read the sidecars
    -- of books only it found.
    local scanned = not skip_failing and self:refreshHashes(interactive)
    local books = self:localBooks(scanned)
    local statistics_on = self:statisticsEnabled()
    local read_pages
    if statistics_on then
        self:flushStatistics()
        local path = self:statisticsPath()
        read_pages = function(cursor, limit) return Statistics.read(path, cursor, limit) end
    end
    local problems = {}
    for __, server in ipairs(self.state:enabledServers()) do
        local label = Urls.label(server.url)
        if server.needs_login then
            table.insert(problems, T(_("%1 needs a new sign-in."), label))
        elseif skip_failing and Sync.backingOff(server, os.time()) then
            logger.dbg("Shisho: leaving out a server that just failed:", server.url)
        else
            local client = Client.new(server.url, self.stamps.version, server.username, server.credential,
                not interactive and Client.BACKGROUND or nil)
            local ok, outcome = pcall(self.syncStates, self, server, client, books, function(changes, store)
                return Sync.run{
                    state = self.state,
                    journal = self.journal,
                    server = server,
                    read_pages = read_pages,
                    post = function(payload) return client:upload(payload) end,
                    changes = changes,
                    store = store,
                }
            end)
            if not ok then
                logger.warn("Shisho: syncing failed:", outcome)
                outcome = "error"
            end
            Sync.noteOutcome(server, outcome, os.time())
            if outcome == "needs_login" then
                table.insert(problems, T(_("%1 needs a new sign-in."), label))
            elseif outcome ~= "ok" then
                table.insert(problems, T(_("Couldn't sync with %1: %2"), label, Responses.message(outcome, nil, server.info)))
            end
        end
    end
    self.state:flush()
    return problems, statistics_on
end

-- Menu -------------------------------------------------------------------------

function Shisho:serverMenu(server)
    local items = {
        {
            text = _("Enabled"),
            help_text = _("Sync with this server. Turn it off for a server you can't always reach, such as one on your own computer: the plugin then doesn't contact it at all, so it never waits on it, doesn't offer its plugin updates, and keeps nothing waiting for it. Turned on again, it gets the reading time KOReader recorded in the meantime, but not places from while it was off."),
            checked_func = function() return self.state:isEnabled(server) end,
            keep_menu_open = true,
            callback = function()
                self.state:setEnabled(server.url, not self.state:isEnabled(server))
                self.journal:prune()
                self.state:flush()
                self:scheduleDaily()
            end,
        },
    }
    if server.last_synced then
        table.insert(items, {
            text = T(_("Last synced %1"), os.date("%Y-%m-%d %H:%M", server.last_synced)),
            enabled = false,
            help_text = _("When the plugin last sent this server your reading time, places, statuses, and Ratings."),
        })
    end
    table.insert(items, {
        text = _("Sign in again"),
        help_text = _("Sign in to this server again with a new pairing code from the Device's page in Shisho, or with the Device's password. Use it after regenerating the Device's password. Reading waiting to sync is kept and sent once you're signed in."),
        keep_menu_open = true,
        callback = function() self:showSignIn(server.url) end,
    })
    if not server.disabled and Versions.reinstallable(self:installed(), self.state:servers(), server.url) then
        table.insert(items, {
            text = _("Reinstall plugin from this server"),
            help_text = _("Install the plugin this server serves, even an older version, and restart KOReader. Automatic updates then come from this server for builds of the same version."),
            keep_menu_open = true,
            callback = function()
                NetworkMgr:runWhenOnline(function() self:installFrom(server.url) end)
            end,
        })
    end
    table.insert(items, {
        text = _("Remove server"),
        help_text = _("Stop syncing with this server. This removes only the plugin's sign-in for it and what was waiting to go to it. KOReader's books, statistics, and OPDS catalog stay."),
        keep_menu_open = true,
        callback = function(touchmenu_instance)
            UIManager:show(ConfirmBox:new{
                text = T(_("Stop using %1? This removes only the plugin's own sign-in for it. KOReader's books, statistics, and OPDS catalog stay."), Urls.label(server.url)),
                ok_text = _("Remove"),
                ok_callback = function()
                    self.state:remove(server.url)
                    self.state:flush()
                    self.summaries:remove(server.url)
                    self.summaries:flush()
                    self:scheduleDaily()
                    if touchmenu_instance then touchmenu_instance:backToUpperMenu() end
                end,
            })
        end,
    })
    return items
end

-- serverHelp says which Device a server's entry signs in as, when it last
-- synced, and what its status means.
function Shisho:serverHelp(server)
    local lines = { T(_("Signed in to %1 as the Device %2."), server.url, server.device_name or server.username or "?") }
    if server.last_synced then
        table.insert(lines, T(_("Last synced %1."), os.date("%Y-%m-%d %H:%M", server.last_synced)))
    else
        table.insert(lines, _("Not synced yet."))
    end
    if server.disabled then
        table.insert(lines, _("Disabled: the plugin doesn't sync with this server or check it for updates until you turn on Enabled under it."))
    elseif server.needs_login then
        table.insert(lines, _("Needs login: the server no longer accepts this sign-in, such as after the Device's password was regenerated. Your reading waits on this device until you sign in again."))
    elseif server.update_required then
        table.insert(lines, _("Update required: the server no longer accepts this plugin. Install the update it offers."))
    end
    return table.concat(lines, "\n\n")
end

local POSITION_CHOICES = {
    { "prompt", _("Ask"), _("Ask before going to the other reader's place, naming where it came from.") },
    { "jump", _("Go there"), _("Go to the other reader's place without asking, and say so briefly.") },
    { "ignore", _("Ignore"), _("Stay where you are on this device.") },
}

-- positionMenu chooses what to do with a newer or older position.
function Shisho:positionMenu(key)
    local items = {}
    for __, choice in ipairs(POSITION_CHOICES) do
        table.insert(items, {
            text = choice[2],
            help_text = choice[3],
            radio = true,
            checked_func = function() return self:settings()[key] == choice[1] end,
            callback = function() self:saveSetting(key, choice[1]) end,
        })
    end
    return items
end

function Shisho:settingsMenu()
    return {
        {
            text = _("Sync on sleep"),
            help_text = _("The plugin syncs on its own whenever Wi-Fi comes on, but never turns Wi-Fi on by itself. With this on, it also turns Wi-Fi on briefly when the device goes to sleep, syncs, and turns Wi-Fi off again, so Shisho stays current even if you rarely use Wi-Fi. It does this at most once an hour, so short sittings don't turn Wi-Fi on every time. It uses some battery. It turns Wi-Fi on only if Settings > Network > Action when Wi-Fi is off is set to Turn on."),
            checked_func = function() return self:settings().sync_on_sleep end,
            callback = function()
                local on = not self:settings().sync_on_sleep
                self:saveSetting("sync_on_sleep", on)
                if on and not self:wifiTurnsOnByItself() then self:notify(WIFI_ASKS) end
            end,
        },
        {
            text = _("Daily sync"),
            help_text_func = function()
                local text = _("Sync once a day at the time below, turning Wi-Fi on for it and off again, so Shisho is current each morning without you doing anything. It uses a little battery once a day. It turns Wi-Fi on only if Settings > Network > Action when Wi-Fi is off is set to Turn on.")
                if Device.wakeup_mgr then
                    return text .. "\n\n" .. _("If the device is asleep then, it wakes up to sync and goes back to sleep. A device that's turned off doesn't sync.")
                end
                return text .. "\n\n" .. _("This device can't wake itself, so the sync happens only if KOReader is open and awake then.")
            end,
            checked_func = function() return self:dailySettings().enabled end,
            callback = function()
                local on = not self:dailySettings().enabled
                self:saveSetting("daily_sync", on)
                if on and not self:wifiTurnsOnByItself() then self:notify(WIFI_ASKS) end
            end,
        },
        {
            text_func = function()
                local daily = self:dailySettings()
                return T(_("Daily sync time: %1"), Daily.label(daily.hour, daily.min))
            end,
            help_text = _("When daily sync runs each day, on this device's clock. 04:00 unless you change it."),
            enabled_func = function() return self:dailySettings().enabled end,
            keep_menu_open = true,
            callback = function() self:showDailyTime() end,
        },
        {
            text = _("Newer position from another reader"),
            help_text = _("What to do when a book is opened with Wi-Fi on, or after a sync, and another reader, such as the Shisho web reader or another device, was in the book more recently than this device: ask before going there, go there without asking, or ignore it. Asks by default."),
            sub_item_table = self:positionMenu("newer"),
        },
        {
            text = _("Older position from another reader"),
            help_text = _("What to do when another reader's place in the book is from before this device's: ask before going there, go there without asking, or ignore it. Ignored by default, so an old place never moves you back."),
            sub_item_table = self:positionMenu("older"),
            separator = true,
        },
        {
            text = _("Check for plugin update"),
            help_text = _("Ask your servers for a newer plugin now, turning Wi-Fi on if needed. The plugin also checks each time Wi-Fi comes on."),
            keep_menu_open = true,
            callback = function()
                if self:noneEnabled() then return end
                NetworkMgr:runWhenOnline(function() self:checkForUpdate(true) end)
            end,
        },
        {
            text_func = function()
                return T(_("Plugin version %1"), self.stamps.version)
            end,
            help_text = _("The version of the Shisho plugin installed. Tap it to see its build."),
            keep_menu_open = true,
            callback = function()
                self:notify(T(_("Shisho KOReader plugin %1\nBuild %2"), self.stamps.version, self.stamps.hash:sub(1, 12)))
            end,
        },
    }
end

-- noneEnabled tells someone who turned every server off, before anything
-- turns Wi-Fi on for nothing.
function Shisho:noneEnabled()
    if #self.state:servers() > 0 and #self.state:enabledServers() == 0 then
        self:notify(NONE_ENABLED)
        return true
    end
    return false
end

function Shisho:addToMainMenu(menu_items)
    menu_items.shisho = {
        text = _("Shisho"),
        sorting_hint = "tools",
        sub_item_table_func = function()
            local items = {}
            if not self:statisticsEnabled() then
                -- One line of the menu holds only the gist; tapping it says the rest.
                table.insert(items, {
                    text = _("Reading time won't sync: statistics off"),
                    help_text = STATISTICS_OFF .. "\n\n" .. _("Turn them on under Tools > Reading statistics > Enabled."),
                    keep_menu_open = true,
                    separator = true,
                    callback = function() self:notify(STATISTICS_OFF) end,
                })
            end
            for __, server in ipairs(self.state:servers()) do
                local label = Urls.label(server.url)
                local status = Responses.serverStatus(server)
                table.insert(items, {
                    text = status and T("%1 (%2)", label, status) or label,
                    help_text_func = function() return self:serverHelp(server) end,
                    sub_item_table = self:serverMenu(server),
                })
            end
            table.insert(items, {
                text = _("Add a server"),
                help_text = _("Sign in to another Shisho server with a pairing code from a KOReader Device's page there. Each server gets the reading for the books it has."),
                keep_menu_open = true,
                separator = true,
                callback = function()
                    self:showSignIn(self.stamps.pairing and self.stamps.pairing.server_url or "")
                end,
            })
            table.insert(items, {
                text = _("Sync now"),
                help_text = T(_("Send your reading time, places, and books' statuses and Ratings to every enabled server now, and bring back statuses and Ratings changed in Shisho, turning Wi-Fi on if it's off, and check the open book for a newer place. Use it before switching to another device. The plugin also syncs on its own whenever Wi-Fi comes on, and while Wi-Fi is on, every %1 pages, when a book closes, and when the device goes to sleep."), Sync.PAGES_BETWEEN),
                keep_menu_open = true,
                callback = function()
                    if #self.state:servers() == 0 then
                        self:syncAll(true)
                        return
                    end
                    if self:noneEnabled() then return end
                    NetworkMgr:runWhenOnline(function() self:syncAll(true) end)
                end,
            })
            table.insert(items, {
                text = _("Settings"),
                help_text = _("Syncing on sleep and daily sync, what to do with places from other readers, and plugin updates."),
                sub_item_table = self:settingsMenu(),
            })
            return items
        end,
    }
end

return Shisho
