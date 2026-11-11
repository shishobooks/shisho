--[[--
The KOReader plugin for Shisho (ADR 0016). This file is the thin layer:
KOReader's events, menus, dialogs, Wi-Fi, and calls to the server. Decisions
live in the shisho/ modules, which have no UI and are unit tested.

This version signs in to Shisho servers, adds their catalogs to KOReader's
OPDS catalog list, and keeps itself up to date from them.
]]
local Client = require("shisho/client")
local ConfirmBox = require("ui/widget/confirmbox")
local DataStorage = require("datastorage")
local InfoMessage = require("ui/widget/infomessage")
local Installer = require("shisho/installer")
local LuaSettings = require("luasettings")
local MultiInputDialog = require("ui/widget/multiinputdialog")
local NetworkMgr = require("ui/network/manager")
local Opds = require("shisho/opds")
local Responses = require("shisho/responses")
local Stamps = require("shisho/stamps")
local State = require("shisho/state")
local UIManager = require("ui/uimanager")
local Urls = require("shisho/urls")
local Versions = require("shisho/versions")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local logger = require("logger")
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

local Shisho = WidgetContainer:extend{
    name = "shisho",
    is_doc_only = false,
    state_file = DataStorage:getSettingsDir() .. "/shisho.lua",
}

function Shisho:init()
    self.stamps = Stamps.load(self.path)
    self.state = State.open(self.state_file)
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
    local server = self.state:signIn{
        url = url,
        username = username,
        credential = credential,
        device_id = answer.device_id,
        device_name = answer.device_name,
        info = answer.server,
    }
    -- A zip from this server's Device page was installed from it, however
    -- the sign-in went once its code expired.
    local pairing = self.stamps.pairing
    if not self.state:get("installed_from") and pairing and Urls.same(pairing.server_url, server.url) then
        self.state:set("installed_from", server.url)
    end
    self.state:flush()
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
    for __, server in ipairs(self.state:servers()) do
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
    self.checked_at = os.time()
    self:checkIn(not interactive)
    local offer, blocked = Versions.offer(self:installed(), self.state:servers())
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
        for __, server in ipairs(self.state:servers()) do
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
    UIManager:scheduleIn(1, function()
        if self:pairingAction() == "sign_in" then
            self:pairFromZip()
        end
        -- A clock set back since the check doesn't count as recent.
        local since = self.checked_at and os.time() - self.checked_at
        local recent = since and since >= 0 and since < RECHECK_AFTER
        if #self.state:servers() > 0 and not recent and not self.awaiting_restart then
            self:checkForUpdate(false)
        end
    end)
end

-- Menu -------------------------------------------------------------------------

function Shisho:serverMenu(server)
    local items = {
        {
            text = _("Sign in again"),
            keep_menu_open = true,
            callback = function() self:showSignIn(server.url) end,
        },
    }
    if Versions.reinstallable(self:installed(), self.state:servers(), server.url) then
        table.insert(items, {
            text = _("Reinstall plugin from this server"),
            keep_menu_open = true,
            callback = function()
                NetworkMgr:runWhenOnline(function() self:installFrom(server.url) end)
            end,
        })
    end
    table.insert(items,
        {
            text = _("Remove server"),
            keep_menu_open = true,
            callback = function(touchmenu_instance)
                UIManager:show(ConfirmBox:new{
                    text = T(_("Stop using %1? This removes only the plugin's own sign-in for it. KOReader's books, statistics, and OPDS catalog stay."), Urls.label(server.url)),
                    ok_text = _("Remove"),
                    ok_callback = function()
                        self.state:remove(server.url)
                        self.state:flush()
                        if touchmenu_instance then touchmenu_instance:backToUpperMenu() end
                    end,
                })
            end,
        })
    return items
end

function Shisho:addToMainMenu(menu_items)
    menu_items.shisho = {
        text = _("Shisho"),
        sorting_hint = "tools",
        sub_item_table_func = function()
            local items = {}
            for __, server in ipairs(self.state:servers()) do
                local label = Urls.label(server.url)
                local status = Responses.serverStatus(server)
                table.insert(items, {
                    text = status and T("%1 (%2)", label, status) or label,
                    sub_item_table = self:serverMenu(server),
                })
            end
            table.insert(items, {
                text = _("Add a server"),
                keep_menu_open = true,
                separator = true,
                callback = function()
                    self:showSignIn(self.stamps.pairing and self.stamps.pairing.server_url or "")
                end,
            })
            table.insert(items, {
                text = _("Check for plugin update"),
                keep_menu_open = true,
                callback = function()
                    NetworkMgr:runWhenOnline(function() self:checkForUpdate(true) end)
                end,
            })
            table.insert(items, {
                text_func = function()
                    return T(_("Plugin version %1"), self.stamps.version)
                end,
                keep_menu_open = true,
                callback = function()
                    self:notify(T(_("Shisho KOReader plugin %1\nBuild %2"), self.stamps.version, self.stamps.hash:sub(1, 12)))
                end,
            })
            return items
        end,
    }
end

return Shisho
