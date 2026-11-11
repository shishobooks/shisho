-- What the server stamped into the plugin's zip.
local Stamps = require("shisho/stamps")

local function write(path, content)
    local file = assert(io.open(path, "w"))
    file:write(content)
    file:close()
end

local pairing = { server_url = "https://books.example.com", short_code = "abc123", expires_at = 1000 }

return {
    ["a copy of the source reads as an unstamped dev plugin"] = function()
        local stamps = Stamps.load(scratch_dir())
        assert_eq(stamps.version, "0.0.0-dev")
        assert_eq(stamps.hash, "")
        assert_eq(stamps.pairing, nil)
    end,

    ["reads the build and the pairing"] = function()
        local dir = scratch_dir()
        write(dir .. "/build.lua", 'return { version = "0.7.0", hash = "abc" }\n')
        write(dir .. "/pairing.lua", 'return { server_url = "https://books.example.com", short_code = "abc123", expires_at = 1000 }\n')
        local stamps = Stamps.load(dir)
        assert_eq(stamps.version, "0.7.0")
        assert_eq(stamps.hash, "abc")
        assert_eq(stamps.pairing, pairing)
    end,

    ["a broken stamp is ignored"] = function()
        local dir = scratch_dir()
        write(dir .. "/pairing.lua", "this is not lua")
        assert_eq(Stamps.load(dir).pairing, nil)
    end,

    ["a code that hasn't expired signs in"] = function()
        assert_eq(Stamps.pairingState(pairing, nil, false, 999), "sign_in")
    end,

    ["an expired code asks with the server filled in"] = function()
        assert_eq(Stamps.pairingState(pairing, nil, false, 1000), "ask")
        assert_eq(Stamps.pairingState({ server_url = "https://books.example.com" }, nil, false, 0), "ask", "a pairing with no code")
    end,

    ["does nothing once handled, once signed in, or with no pairing"] = function()
        assert_eq(Stamps.pairingState(pairing, "abc123", false, 999), nil)
        assert_eq(Stamps.pairingState(pairing, nil, true, 999), nil)
        assert_eq(Stamps.pairingState(nil, nil, false, 999), nil)
    end,
}
