-- Deciding what to do with a position pulled from a server.
local Positions = require("shisho/positions")

local function remote(fields)
    local r = { progress = "/body/p[50]", percentage = 0.5, timestamp = 2000, source = "Shisho web reader", own = false }
    for k, v in pairs(fields or {}) do r[k] = v end
    return r
end

local here = { progress = "/body/p[10]", percentage = 0.1, at = 1000 }

return {
    ["a newer position prompts by default"] = function()
        assert_eq(Positions.decide(remote(), here, {}), "prompt")
        assert_eq(select(2, Positions.decide(remote(), here, {})), "newer")
    end,

    ["an older position is ignored by default"] = function()
        assert_eq(Positions.decide(remote{ timestamp = 500 }, here, {}), "ignore")
        assert_eq(select(2, Positions.decide(remote{ timestamp = 500 }, here, {})), "older")
    end,

    ["the settings choose for each direction"] = function()
        local settings = { newer = "jump", older = "prompt" }
        assert_eq(Positions.decide(remote(), here, settings), "jump")
        assert_eq(Positions.decide(remote{ timestamp = 500 }, here, settings), "prompt")
        assert_eq(Positions.decide(remote(), here, { newer = "ignore" }), "ignore")
    end,

    ["a book never read here takes any position as newer"] = function()
        assert_eq(Positions.decide(remote{ timestamp = 1 }, { progress = "/body/p[1]", percentage = 0 }, {}), "prompt")
    end,

    ["the device's own position and the current place are ignored"] = function()
        assert_eq(Positions.decide(remote{ own = true }, here, { newer = "jump" }), "ignore")
        assert_eq(Positions.decide(remote{ progress = here.progress }, here, { newer = "jump" }), "ignore")
        assert_eq(Positions.decide(remote{ percentage = 0.1 }, here, { newer = "jump" }), "ignore")
        assert_eq(Positions.decide(nil, here, {}), "ignore")
    end,

    ["of several servers' positions, the newest wins"] = function()
        local best = Positions.newest({ remote{ timestamp = 10, source = "a" }, remote{ timestamp = 30, source = "b" }, remote{ timestamp = 20 } })
        assert_eq(best.source, "b")
        assert_eq(Positions.newest({}), nil)
    end,

    ["this device's instant is its latest of the journal's and the last page turn"] = function()
        assert_eq(Positions.localInstant(nil, nil), nil)
        assert_eq(Positions.localInstant(1000, nil), 1000)
        assert_eq(Positions.localInstant(nil, 1200), 1200)
        assert_eq(Positions.localInstant(1000, 1200), 1200)
        assert_eq(Positions.localInstant(1500, 1200), 1500)
    end,

    ["settings default to prompting for newer and ignoring older"] = function()
        assert_eq(Positions.settings(nil), { newer = "prompt", older = "ignore", sync_on_sleep = false })
        assert_eq(Positions.settings({ newer = "jump", sync_on_sleep = true }), { newer = "jump", older = "ignore", sync_on_sleep = true })
    end,
}
