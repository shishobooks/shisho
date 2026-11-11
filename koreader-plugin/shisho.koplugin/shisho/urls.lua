-- Server URLs, compared the way a person means them: a missing scheme is
-- http, the scheme and host ignore case, and trailing slashes don't count.
local Urls = {}

function Urls.normalize(url)
    if type(url) ~= "string" then return nil end
    url = url:gsub("^%s+", ""):gsub("%s+$", "")
    if url == "" then return nil end
    if not url:match("^%a[%w+.-]*://") then
        url = "http://" .. url
    end
    local scheme, rest = url:match("^(%a[%w+.-]*)://(.*)$")
    local host, path = rest:match("^([^/]*)(.*)$")
    path = path:gsub("/+$", "")
    return scheme:lower() .. "://" .. host:lower() .. path
end

function Urls.same(a, b)
    local na, nb = Urls.normalize(a), Urls.normalize(b)
    return na ~= nil and na == nb
end

-- candidates are the URLs to try for what a person typed: as typed when it
-- names a scheme, and otherwise https, then http. Trying https first keeps
-- a sign-in off plain http where the server has https, since the plugin's
-- requests are POSTs, which don't follow a redirect to it.
function Urls.candidates(typed)
    local normalized = Urls.normalize(typed)
    if not normalized then return {} end
    if typed:match("^%s*%a[%w+.-]*://") then
        return { normalized }
    end
    local rest = normalized:match("^http://(.*)$")
    return { "https://" .. rest, normalized }
end

-- The host (and port) a person recognizes a server by.
function Urls.label(url)
    local normalized = Urls.normalize(url)
    if not normalized then return "" end
    return normalized:match("^%a[%w+.-]*://(.*)$")
end

return Urls
