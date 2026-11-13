-- HTTP to one Shisho server. Every request carries the plugin's version and
-- protocol, as the server requires.
local JSON = require("json")
local Protocol = require("shisho/protocol")
local Urls = require("shisho/urls")
local http = require("socket.http")
local ltn12 = require("ltn12")
local mime = require("mime")
local socket = require("socket")
local socketutil = require("socketutil")

local Client = {}
Client.__index = Client

-- Timeouts in seconds, per block and in total. A background check-in runs
-- whenever Wi-Fi comes on, and a server out of reach mustn't hold KOReader
-- up for long.
Client.BACKGROUND = { 5, 10 }

-- new returns a client for the server at url, signed in with username and
-- credential when given. plugin_version is the running plugin's.
function Client.new(url, plugin_version, username, credential, timeouts)
    return setmetatable({
        url = Urls.normalize(url),
        plugin_version = plugin_version,
        username = username,
        credential = credential,
        timeouts = timeouts,
    }, Client)
end

-- post sends body as JSON to path and returns the status code, the decoded
-- answer, and the response's headers, by lowercase name. With sink, the
-- answer goes there instead.
function Client:post(path, body, sink)
    local encoded = body and JSON.encode(body) or ""
    local headers = {
        ["Content-Type"] = "application/json",
        ["Content-Length"] = tostring(#encoded),
        ["Accept"] = "application/json",
        ["X-Shisho-Plugin-Version"] = self.plugin_version,
        ["X-Shisho-Plugin-Protocol"] = tostring(Protocol.version),
    }
    if self.username and self.credential then
        headers["Authorization"] = "Basic " .. mime.b64(self.username .. ":" .. self.credential)
    end
    local chunks = {}
    if self.timeouts then
        socketutil:set_timeout(self.timeouts[1], self.timeouts[2])
    elseif sink then
        socketutil:set_timeout(socketutil.FILE_BLOCK_TIMEOUT, socketutil.FILE_TOTAL_TIMEOUT)
    else
        socketutil:set_timeout(socketutil.LARGE_BLOCK_TIMEOUT, socketutil.LARGE_TOTAL_TIMEOUT)
    end
    local code, response_headers = socket.skip(1, http.request{
        url = self.url .. path,
        method = "POST",
        headers = headers,
        source = ltn12.source.string(encoded),
        sink = sink or ltn12.sink.table(chunks),
    })
    socketutil:reset_timeout()
    if type(code) ~= "number" then
        return nil
    end
    local decoded
    if not sink or code ~= 200 then
        local ok, value = pcall(JSON.decode, table.concat(chunks))
        if ok then decoded = value end
    end
    return code, decoded, response_headers
end

function Client:pair(code)
    return self:post("/koreader/plugin/pair", { code = code })
end

function Client:status()
    return self:post("/koreader/plugin/status")
end

-- upload sends one batch of page rows and journal entries (shisho/sync).
function Client:upload(payload)
    return self:post("/koreader/plugin/upload", payload)
end

-- pull asks for the positions of the books documents names, timed on the
-- device's clock.
function Client:pull(documents, clock)
    return self:post("/koreader/plugin/pull", { clock = clock or os.time(), documents = documents })
end

-- pullStates asks for the statuses and Ratings to write to the device
-- (shisho/sync's pullStates makes payload).
function Client:pullStates(payload, clock)
    payload.clock = clock or os.time()
    return self:post("/koreader/plugin/pull", payload)
end

-- download saves the server's plugin zip to path. Returns the status, or
-- nil and an error when path can't be written.
function Client:download(path)
    local file, err = io.open(path, "wb")
    if not file then
        return nil, err or "can't write " .. path
    end
    local code = self:post("/koreader/plugin/download", nil, ltn12.sink.file(file))
    if code ~= 200 then
        os.remove(path)
    end
    return code
end

return Client
