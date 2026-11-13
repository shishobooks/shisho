-- The plugin's own state file.
local LuaSettings = require("luasettings")
local State = require("shisho/state")
local lfs = require("libs/libkoreader-lfs")

local function signIn(state, url, credential)
    return state:signIn{
        url = url, username = "reader", credential = credential,
        device_id = "dev-1", device_name = "Libra KOReader",
        info = { plugin_version = "0.7.0" },
    }
end

return {
    ["a missing file opens with no servers"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        assert_eq(#state:servers(), 0)
    end,

    ["signing in again replaces the sign-in and clears what needed attention"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        local server = signIn(state, "https://Books.example.com/", "OLD")
        server.needs_login = true
        server.update_required = true

        signIn(state, "https://books.example.com", "NEW")

        assert_eq(#state:servers(), 1)
        assert_eq(state:servers()[1].url, "https://books.example.com")
        assert_eq(state:servers()[1].credential, "NEW")
        assert_eq(state:servers()[1].needs_login, nil)
        assert_eq(state:servers()[1].update_required, nil)
    end,

    ["keeps a list of servers"] = function()
        local state = State.open(scratch_dir() .. "/shisho.lua")
        signIn(state, "https://a.example.com", "A")
        signIn(state, "https://b.example.com", "B")
        assert_eq(#state:servers(), 2)
        assert_eq(state:find("https://B.example.com/").credential, "B")
        assert_eq(state:remove("https://a.example.com"), true)
        assert_eq(#state:servers(), 1)
        assert_eq(state:servers()[1].url, "https://b.example.com")
    end,

    ["a server can be disabled and enabled again, and stays signed in"] = function()
        local path = scratch_dir() .. "/shisho.lua"
        local state = State.open(path)
        signIn(state, "https://a.example", "A")
        signIn(state, "https://b.example", "B")
        assert_eq(state:isEnabled(state:find("https://b.example")), true, "enabled by default")

        state:setEnabled("https://B.example/", false)
        assert_eq(state:flush(), true)
        local reopened = State.open(path)
        assert_eq(#reopened:servers(), 2)
        assert_eq(#reopened:enabledServers(), 1)
        assert_eq(reopened:enabledServers()[1].url, "https://a.example")
        assert_eq(reopened:isEnabled(reopened:find("https://b.example")), false)

        -- Signing in again leaves the choice alone.
        signIn(reopened, "https://b.example", "NEW")
        assert_eq(reopened:isEnabled(reopened:find("https://b.example")), false)
        reopened:setEnabled("https://b.example", true)
        assert_eq(#reopened:enabledServers(), 2)
    end,

    ["a failed write leaves the old state"] = function()
        local dir = scratch_dir()
        local path = dir .. "/shisho.lua"
        local state = State.open(path)
        signIn(state, "https://books.example.com", "GOOD")
        assert_eq(state:flush(), true)

        -- The temporary file can't be written where a directory stands.
        lfs.mkdir(path .. ".tmp")
        signIn(state, "https://books.example.com", "NEW")
        local ok = state:flush()
        assert_eq(ok, nil)
        assert_eq(State.open(path):find("https://books.example.com").credential, "GOOD")
    end,

    ["flushes atomically and reads back through KOReader's settings"] = function()
        local dir = scratch_dir()
        local path = dir .. "/shisho.lua"
        local state = State.open(path)
        signIn(state, "https://books.example.com", "CRED")
        state:set("installed_from", "https://books.example.com")
        assert_eq(state:flush(), true)

        assert_eq(lfs.attributes(path .. ".tmp", "mode"), nil, "no temporary file is left behind")
        local data = LuaSettings:open(path).data
        assert_eq(data.installed_from, "https://books.example.com")
        assert_eq(data.servers[1].credential, "CRED")

        local reopened = State.open(path)
        assert_eq(reopened:find("https://books.example.com").device_name, "Libra KOReader")
    end,
}
