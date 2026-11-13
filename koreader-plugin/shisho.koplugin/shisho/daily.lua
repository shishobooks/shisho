--[[--
Daily sync: an opt-in sync at a set time each day, which wakes a sleeping
e-reader that can be woken on a timer (ADR 0016's "never turns Wi-Fi on by
itself" holds until the user turns it on). Times are the device's local
time. No UI.
]]
local Daily = {}

Daily.DEFAULT_HOUR = 4
Daily.DEFAULT_MIN = 0

local function inRange(value, low, high)
    return type(value) == "number" and value == math.floor(value) and value >= low and value <= high
end

-- settings reads daily sync's saved settings with their defaults: off, at
-- 4:00.
function Daily.settings(saved)
    saved = type(saved) == "table" and saved or {}
    local hour, min = saved.daily_sync_hour, saved.daily_sync_min
    if not (inRange(hour, 0, 23) and inRange(min, 0, 59)) then
        hour, min = Daily.DEFAULT_HOUR, Daily.DEFAULT_MIN
    end
    return { enabled = saved.daily_sync == true, hour = hour, min = min }
end

-- nextRun is the epoch of the next hour:min after now, local time: later
-- today, or tomorrow once today's has come.
function Daily.nextRun(now, hour, min)
    local t = os.date("*t", now)
    t.hour, t.min, t.sec = hour, min, 0
    -- Left set, today's daylight saving flag would put a run across a
    -- change an hour off; unset, os.time works it out for the day.
    t.isdst = nil
    local run = os.time(t)
    if run <= now then
        t.day = t.day + 1
        t.isdst = nil
        run = os.time(t)
    end
    return run
end

-- label is hour:min on a 24-hour clock.
function Daily.label(hour, min)
    return string.format("%02d:%02d", hour, min)
end

return Daily
