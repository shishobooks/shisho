-- What a server's answer means to the plugin.
local Responses = require("shisho/responses")

local function errorBody(code, message)
    return { error = { code = code, message = message, status_code = 0 } }
end

return {
    ["classifies the server's answers"] = function()
        assert_eq(Responses.classify(200, {}), "ok")
        assert_eq(Responses.classify(nil), "unreachable")
        assert_eq(Responses.classify(426, errorBody("plugin_update_required")), "update_required")
        assert_eq(Responses.classify(401, errorBody("unauthorized")), "needs_login")
        assert_eq(Responses.classify(404, errorBody("not_found")), "not_found")
        assert_eq(Responses.classify(429, errorBody("too_many_requests")), "rate_limited")
        assert_eq(Responses.classify(403, errorBody("demo_mode", "This action is unavailable in the demo.")), "demo")
        assert_eq(Responses.classify(500, nil), "error")
    end,

    ["a protocol the server no longer accepts shows update required"] = function()
        local outcome = Responses.classify(426, errorBody("plugin_update_required"))
        assert_eq(Responses.serverStatus({ update_required = outcome == "update_required" }), "Update required")
        assert_eq(Responses.message(outcome), "Update required: this server needs a newer KOReader plugin.")
    end,

    ["a redirect asks for the server's own address"] = function()
        assert_eq(Responses.classify(301, nil), "redirect")
        assert_eq(Responses.message("redirect"):find("https://", 1, true) ~= nil, true)
    end,

    ["names which side is out of date"] = function()
        assert_eq(Responses.message("update_required", nil, { protocol_max = 0 }),
            "Update required: this plugin is newer than the server. Upgrade Shisho on it, or install the plugin it serves.")
        assert_eq(Responses.message("update_required", nil, { protocol_max = 5 }),
            "Update required: this server needs a newer KOReader plugin.")
    end,

    ["a check-in records what the server said"] = function()
        local server = { url = "http://a", needs_login = true, update_required = true, info = { plugin_version = "0.7.0" } }
        assert_eq(Responses.checkIn(server, 200, { device_name = "Libra", server = { plugin_version = "0.8.0" } }), "ok")
        assert_eq(server, { url = "http://a", device_name = "Libra", info = { plugin_version = "0.8.0" } })

        assert_eq(Responses.checkIn(server, 401, errorBody("unauthorized")), "needs_login")
        assert_eq(server.needs_login, true)

        assert_eq(Responses.checkIn(server, nil), "unreachable")
        assert_eq(server.needs_login, true, "an unreachable server keeps what it last said")
        assert_eq(server.info, { plugin_version = "0.8.0" })

        assert_eq(Responses.checkIn(server, 426, errorBody("plugin_update_required"), {}), "update_required")
        assert_eq(server.update_required, true)
        assert_eq(server.info, { plugin_version = "0.8.0" }, "no headers keeps the last info")
    end,

    ["reads a refusal's server info from its headers"] = function()
        assert_eq(Responses.infoFromHeaders({
            ["x-shisho-protocol-min"] = "1", ["x-shisho-protocol-max"] = "2",
            ["x-shisho-server-plugin-version"] = "0.9.0", ["x-shisho-server-plugin-hash"] = "abc",
            ["x-shisho-server-plugin-protocol"] = "2",
        }), { protocol_min = 1, protocol_max = 2, plugin_version = "0.9.0", plugin_hash = "abc", plugin_protocol = 2 })
        assert_eq(Responses.infoFromHeaders({}), nil)
        assert_eq(Responses.infoFromHeaders(nil), nil)
    end,

    ["a refused sign-in shows needs login"] = function()
        assert_eq(Responses.serverStatus({ needs_login = true }), "Needs login")
        assert_eq(Responses.serverStatus({}), nil)
        assert_eq(Responses.serverStatus({ disabled = true, needs_login = true }), "Disabled")
    end,

    ["uses the server's own message"] = function()
        local body = errorBody("demo_mode", "This action is unavailable in the demo.")
        assert_eq(Responses.message("demo", body), "This action is unavailable in the demo.")
    end,
}
