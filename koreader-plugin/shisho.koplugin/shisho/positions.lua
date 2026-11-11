--[[--
What to do with a position pulled from a server (ADR 0016): go there, ask
first, or leave it, by whether it's newer than this device's own place and
the person's settings, which default to asking about newer positions and
ignoring older ones, as KOReader's progress sync does. No UI.
]]
local Positions = {}

local ACTIONS = { prompt = true, jump = true, ignore = true }

-- settings reads the plugin's position settings with their defaults.
function Positions.settings(saved)
    saved = type(saved) == "table" and saved or {}
    return {
        newer = ACTIONS[saved.newer] and saved.newer or "prompt",
        older = ACTIONS[saved.older] and saved.older or "ignore",
        sync_on_sleep = saved.sync_on_sleep == true,
    }
end

local function samePercent(a, b)
    return type(a) == "number" and type(b) == "number" and math.floor(a * 10000 + 0.5) == math.floor(b * 10000 + 0.5)
end

--[[--
decide returns "prompt", "jump", or "ignore" for remote, a pulled position
{ progress, percentage, timestamp, source, own }, given here, this device's
place { progress, percentage, at } where at is when it was last somewhere
new in the book (nil if never), and the saved settings. Its second result
is "newer" or "older".
]]
function Positions.decide(remote, here, saved)
    if type(remote) ~= "table" or remote.own or not remote.progress then
        return "ignore"
    end
    here = here or {}
    if remote.progress == here.progress or samePercent(remote.percentage, here.percentage) then
        return "ignore"
    end
    local settings = Positions.settings(saved)
    local newer = here.at == nil or (tonumber(remote.timestamp) or 0) > here.at
    if newer then
        return settings.newer, "newer"
    end
    return settings.older, "older"
end

-- localInstant is when this device was last somewhere new in a book: the
-- later of the journal's last position (journal_at) and a page turn not yet
-- journaled (last_page_turn). Nil when neither is known.
function Positions.localInstant(journal_at, last_page_turn)
    if journal_at and last_page_turn then
        return math.max(journal_at, last_page_turn)
    end
    return journal_at or last_page_turn
end

-- newest is the latest of several servers' positions for one book.
function Positions.newest(list)
    local best
    for __, position in ipairs(list or {}) do
        if not best or (tonumber(position.timestamp) or 0) > (tonumber(best.timestamp) or 0) then
            best = position
        end
    end
    return best
end

return Positions
