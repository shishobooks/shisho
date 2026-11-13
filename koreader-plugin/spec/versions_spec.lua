-- Which plugin to update to, and from which server (ADR 0016).
local Versions = require("shisho/versions")

local function server(url, version, protocol, hash, accepts_min, accepts_max)
    return {
        url = url,
        info = {
            plugin_version = version,
            plugin_hash = hash or (url .. version),
            plugin_protocol = protocol or 1,
            protocol_min = accepts_min or 1,
            protocol_max = accepts_max or 1,
        },
    }
end

local installed = { version = "0.7.0", hash = "installed", installed_from = "http://a" }

return {
    ["versions order by release, then dev after its release"] = function()
        assert_eq(Versions.compare("0.7.0", "0.7.0"), 0)
        assert_eq(Versions.compare("0.7.0", "0.8.0"), -1)
        assert_eq(Versions.compare("0.10.0", "0.9.9"), 1)
        assert_eq(Versions.compare("1.0.0", "0.99.99"), 1)
        assert_eq(Versions.compare("0.7.0-dev", "0.7.0"), 1, "a dev build is newer than its release")
        assert_eq(Versions.compare("0.7.0-dev", "0.8.0"), -1)
        assert_eq(Versions.compare("0.7.0-dev", "0.7.0-dev"), 0)
        assert_eq(Versions.compare("v0.7.0", "0.7.0"), 0)
        assert_eq(Versions.compare(nil, "0.0.0-dev"), 0, "an unstamped plugin is 0.0.0-dev")
        assert_eq(Versions.compare("0.8.0-rc.1", "0.8.0"), -1, "a prerelease is older than its release")
        assert_eq(Versions.compare("0.8.0-rc.1", "0.7.0"), 1)
        assert_eq(Versions.compare("0.8.0-rc.1", "0.8.0-rc.2"), -1)
        assert_eq(Versions.compare("0.8.0-rc.1", "0.8.0-dev"), -1)
    end,

    ["offers only a strictly newer version"] = function()
        assert_eq(Versions.offer(installed, { server("http://a", "0.7.0", 1, "installed") }), nil, "the same build")
        assert_eq(Versions.offer(installed, { server("http://a", "0.6.0") }), nil, "an older version")
        local offer = Versions.offer(installed, { server("http://a", "0.8.0") })
        assert_eq(offer, { server = "http://a", version = "0.8.0", hash = "http://a0.8.0" })
    end,

    ["offers the newest version every server accepts"] = function()
        local offer = Versions.offer(installed, {
            server("http://a", "0.8.0"),
            server("http://b", "0.9.0"),
            server("http://c", "0.7.0"),
        })
        assert_eq(offer.server, "http://b")
        assert_eq(offer.version, "0.9.0")
    end,

    ["never offers a version some server refuses"] = function()
        -- b serves a plugin speaking protocol 2, which a doesn't accept.
        local offer, blocked = Versions.offer(installed, {
            server("http://a", "0.7.0", 1, "installed", 1, 1),
            server("http://b", "0.9.0", 2, nil, 1, 2),
        })
        assert_eq(offer, nil)
        assert_eq(blocked, { server = "http://b", version = "0.9.0", upgrade = "http://a" }, "names the server to upgrade")
    end,

    ["falls back to an older newer version every server accepts"] = function()
        local offer = Versions.offer(installed, {
            server("http://a", "0.8.0", 1, nil, 1, 1),
            server("http://b", "0.9.0", 2, nil, 1, 2),
        })
        assert_eq(offer.server, "http://a")
        assert_eq(offer.version, "0.8.0")
    end,

    ["offers a same-version build only from the installed-from server"] = function()
        local servers = {
            server("http://a", "0.7.0", 1, "a-build"),
            server("http://b", "0.7.0", 1, "b-build"),
        }
        local offer = Versions.offer(installed, servers)
        assert_eq(offer, { server = "http://a", version = "0.7.0", hash = "a-build" })

        local from_b = { version = "0.7.0", hash = "b-build", installed_from = "http://b/" }
        assert_eq(Versions.offer(from_b, servers), nil, "b's build is installed, and a never offers its own")

        local from_nowhere = { version = "0.7.0", hash = "release" }
        assert_eq(Versions.offer(from_nowhere, servers), nil, "a release zip installed by hand")
    end,

    ["checking yourself offers another server's build of the same version"] = function()
        local servers = {
            server("http://a", "0.7.0", 1, "a-build"),
            server("http://b", "0.7.0", 1, "b-build"),
        }
        local checking = { any_build = true }
        -- The installed-from server's build comes first.
        assert_eq(Versions.offer(installed, servers, checking), { server = "http://a", version = "0.7.0", hash = "a-build" })

        -- Up to date with it, another server's build is offered when asked.
        local from_b = { version = "0.7.0", hash = "b-build", installed_from = "http://b/" }
        assert_eq(Versions.offer(from_b, servers, checking), { server = "http://a", version = "0.7.0", hash = "a-build" })
        local from_nowhere = { version = "0.7.0", hash = "release" }
        assert_eq(Versions.offer(from_nowhere, servers, checking).version, "0.7.0", "a release zip installed by hand")

        -- Nothing different anywhere is up to date.
        local same = { server("http://a", "0.7.0", 1, "x"), server("http://b", "0.7.0", 1, "x") }
        assert_eq(Versions.offer({ version = "0.7.0", hash = "x", installed_from = "http://a" }, same, checking), nil)
        -- A server that needs a new sign-in offers nothing.
        local signed_out = server("http://b", "0.7.0", 1, "b-build")
        signed_out.needs_login = true
        assert_eq(Versions.offer(from_nowhere, { signed_out }, checking), nil)
    end,

    ["prefers the installed-from server between equal newer versions"] = function()
        local offer = Versions.offer(installed, {
            server("http://b", "0.8.0"),
            server("http://a", "0.8.0"),
        })
        assert_eq(offer.server, "http://a")
    end,

    ["offers the update a server that refused the plugin serves"] = function()
        -- The server raised its protocol and serves a plugin speaking it;
        -- its refusal said so (responses_spec covers reading that).
        local Responses = require("shisho/responses")
        local refusing = server("http://a", "0.7.0", 1, "installed")
        Responses.checkIn(refusing, 426, { error = { code = "plugin_update_required" } }, {
            ["x-shisho-protocol-min"] = "2", ["x-shisho-protocol-max"] = "2",
            ["x-shisho-server-plugin-version"] = "0.9.0", ["x-shisho-server-plugin-hash"] = "new",
            ["x-shisho-server-plugin-protocol"] = "2",
        })
        assert_eq(refusing.update_required, true)
        assert_eq(Versions.offer(installed, { refusing }), { server = "http://a", version = "0.9.0", hash = "new" })
    end,

    ["never offers from a server that needs a new sign-in"] = function()
        -- Its last answer may be stale, and its download needs a sign-in.
        local signed_out = server("http://a", "0.9.0")
        signed_out.needs_login = true
        assert_eq(Versions.offer(installed, { signed_out }), nil)

        local same_build = server("http://a", "0.7.0", 1, "a-build")
        same_build.needs_login = true
        assert_eq(Versions.offer(installed, { same_build }), nil)

        -- What it last accepted still counts against another server's offer.
        local strict = server("http://a", "0.7.0", 1, "installed", 1, 1)
        strict.needs_login = true
        local offer, blocked = Versions.offer(installed, { strict, server("http://b", "0.9.0", 2, nil, 1, 2) })
        assert_eq(offer, nil)
        assert_eq(blocked.upgrade, "http://a")
    end,

    ["reinstalling is offered with several servers, or when nothing else would install a server's build"] = function()
        local one = { server("http://a", "0.7.0", 1, "installed") }
        assert_eq(Versions.reinstallable(installed, one, "http://a"), false, "the build installed")

        assert_eq(Versions.reinstallable(installed, { server("http://a", "0.6.0") }, "http://a"), true, "an older version")

        local by_hand = { version = "0.7.0", hash = "release" }
        assert_eq(Versions.reinstallable(by_hand, { server("http://a", "0.7.0", 1, "a-build") }, "http://a"), true,
            "another build of the version, with no server it was installed from")

        assert_eq(Versions.reinstallable(installed, { server("http://a", "0.8.0") }, "http://a"), false,
            "Check for plugin update offers it")
        assert_eq(Versions.reinstallable(installed, { { url = "http://a" } }, "http://a"), false, "nothing known yet")

        local two = { server("http://a", "0.7.0", 1, "installed"), server("http://b", "0.7.0", 1, "installed") }
        assert_eq(Versions.reinstallable(installed, two, "http://a"), true)
        assert_eq(Versions.reinstallable(installed, two, "http://b"), true)
    end,

    ["offers nothing before any server has answered"] = function()
        assert_eq(Versions.offer(installed, {}), nil)
        assert_eq(Versions.offer(installed, { { url = "http://a" } }), nil)
    end,

    ["manual install picks the chosen server, whatever it serves"] = function()
        local servers = { server("http://a", "0.6.0"), server("http://b", "0.9.0") }
        assert_eq(Versions.choose(servers, "HTTP://A/"), { server = "http://a", version = "0.6.0", hash = "http://a0.6.0" })
        assert_eq(Versions.choose(servers, "http://c"), nil, "a server not signed in to")

        -- Once installed from b, b's same-version builds are the ones offered.
        local from_b = { version = "0.9.0", hash = "old", installed_from = "http://b" }
        local next_servers = { server("http://a", "0.9.0", 1, "a-build"), server("http://b", "0.9.0", 1, "b-build") }
        assert_eq(Versions.offer(from_b, next_servers).server, "http://b")
    end,
}
