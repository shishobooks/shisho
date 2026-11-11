-- Plugin versions and which server to update from (ADR 0016). No UI.
local Urls = require("shisho/urls")

local Versions = {}

-- parse reads "1.2.3", "1.2.3-dev", or a prerelease such as "1.2.3-rc.1".
-- A dev build carries the release it was built from and counts as newer
-- than that release; a prerelease counts as older than its release.
-- Anything else, such as the plugin's source copied over without a server
-- stamping it, reads as 0.0.0-dev.
function Versions.parse(v)
    local a, b, c, rest
    if type(v) == "string" then
        a, b, c, rest = v:match("^v?(%d+)%.(%d+)%.(%d+)(.*)$")
    end
    if not a then
        return { 0, 0, 0, dev = true }
    end
    local parsed = { tonumber(a), tonumber(b), tonumber(c) }
    if rest == "-dev" then
        parsed.dev = true
    elseif rest ~= "" then
        parsed.pre = rest:gsub("^%-", "")
    end
    return parsed
end

-- rank orders the kinds of build of one release: prerelease, release, dev.
local function rank(parsed)
    if parsed.pre then return 0 end
    if parsed.dev then return 2 end
    return 1
end

-- compare returns -1, 0, or 1 as version x is older than, the same as, or
-- newer than version y.
function Versions.compare(x, y)
    local a, b = Versions.parse(x), Versions.parse(y)
    for i = 1, 3 do
        if a[i] ~= b[i] then
            return a[i] < b[i] and -1 or 1
        end
    end
    if rank(a) ~= rank(b) then
        return rank(a) < rank(b) and -1 or 1
    end
    if a.pre and a.pre ~= b.pre then
        return a.pre < b.pre and -1 or 1
    end
    return 0
end

-- accepts reports whether a server, by the info it last answered with,
-- accepts protocol.
function Versions.accepts(info, protocol)
    return type(info) == "table" and type(protocol) == "number"
        and info.protocol_min ~= nil and info.protocol_max ~= nil
        and info.protocol_min <= protocol and protocol <= info.protocol_max
end

local function offerFrom(server)
    return {
        server = server.url,
        version = server.info.plugin_version,
        hash = server.info.plugin_hash,
    }
end

--[[--
offer decides whether to offer an update, and from which server.

installed is { version, hash, installed_from }, the plugin running now and
the server it was last installed from. servers are the signed-in servers,
each { url, info }, where info is what the server last answered (nil when it
hasn't yet).

- A server offers its plugin only when it's strictly newer than the one
  installed, and every server that has answered accepts its protocol. The
  newest such plugin wins, and between equals the installed-from server.
- A build of the same version with a different hash is offered only by the
  server the plugin was last installed from, so two dev servers never swap
  it back and forth.
- A server that needs a new sign-in offers nothing: its last answer may be
  stale, and its download needs the sign-in. What it last accepted still
  counts.

Returns the offer { server, version, hash }, or nil and, when a newer plugin
exists that some server would refuse, { server, version, upgrade }, naming
the server to upgrade.
]]
function Versions.offer(installed, servers)
    local known = {}
    for __, server in ipairs(servers or {}) do
        if type(server.info) == "table" then
            table.insert(known, server)
        end
    end
    if #known == 0 then return nil end

    local function refuser(protocol)
        for __, server in ipairs(known) do
            if not Versions.accepts(server.info, protocol) then
                return server
            end
        end
    end

    local best, blocked
    for __, server in ipairs(known) do
        local version = server.info.plugin_version
        if not server.needs_login and Versions.compare(version, installed.version) > 0 then
            local refused_by = refuser(server.info.plugin_protocol)
            if not refused_by then
                local cmp = best and Versions.compare(version, best.info.plugin_version) or 1
                if cmp > 0 or (cmp == 0 and Urls.same(server.url, installed.installed_from)) then
                    best = server
                end
            elseif not blocked or Versions.compare(version, blocked.version) > 0 then
                blocked = { server = server.url, version = version, upgrade = refused_by.url }
            end
        end
    end
    if best then
        return offerFrom(best)
    end

    for __, server in ipairs(known) do
        if not server.needs_login and Urls.same(server.url, installed.installed_from)
                and Versions.compare(server.info.plugin_version, installed.version) == 0
                and server.info.plugin_hash ~= installed.hash
                and not refuser(server.info.plugin_protocol) then
            return offerFrom(server)
        end
    end
    return nil, blocked
end

-- reinstallable is whether the server at url gets Reinstall plugin from this
-- server. With several servers, choosing whose plugin to run matters, so
-- each does. With one, only when it serves a build Check for plugin update
-- would never install: an older version, or another build of this version
-- when the plugin wasn't installed from it, as with a release zip.
function Versions.reinstallable(installed, servers, url)
    servers = servers or {}
    if #servers > 1 then return true end
    local server = Versions.choose(servers, url)
    if not server or not server.version then return false end
    if Versions.compare(server.version, installed.version) == 0 and server.hash == installed.hash then
        return false
    end
    local offer = Versions.offer(installed, servers)
    return not (offer and Urls.same(offer.server, url) and offer.hash == server.hash)
end

-- choose is what Reinstall plugin from this server installs: whatever plugin that
-- server serves, older or newer, because the user picked it on purpose. It
-- then becomes the server the plugin was installed from. Returns nil for a
-- server not signed in to.
function Versions.choose(servers, url)
    for __, server in ipairs(servers or {}) do
        if Urls.same(server.url, url) then
            local info = type(server.info) == "table" and server.info or {}
            return { server = server.url, version = info.plugin_version, hash = info.plugin_hash }
        end
    end
end

return Versions
