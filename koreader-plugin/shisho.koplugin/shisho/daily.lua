--[[--
Daily sync: an opt-in sync at a set time each day, which wakes a sleeping
e-reader that can be woken on a timer. It's off until the user turns it on,
like everything that turns Wi-Fi on without being asked. Times are the
device's local time. No UI.
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

--[[--
secondsToNext is how long from now until daily sync next runs, or nil when
it's off or no server is enabled. It counts from the last run when that's
later than now, as when an alarm woke the device a little early and ran
today's: from then, today's time would still be to come.
]]
function Daily.secondsToNext(saved, enabled_count, now, last_run)
    local daily = Daily.settings(saved)
    if not daily.enabled or (enabled_count or 0) == 0 then return nil end
    return Daily.nextRun(math.max(now, last_run or now), daily.hour, daily.min) - now
end

-- alarmIn is how long from now to set the wake-up alarm for, or nil when
-- waking couldn't sync: when daily sync wouldn't run, or when KOReader asks
-- before turning Wi-Fi on, which no one is there to answer.
function Daily.alarmIn(saved, enabled_count, wifi_turns_on, now, last_run)
    if not wifi_turns_on then return nil end
    return Daily.secondsToNext(saved, enabled_count, now, last_run)
end

-- claim reports whether the run due at target is still to do, the last
-- having been for last_run. The timer and the alarm both aim at the same
-- time, and whichever goes first claims it.
function Daily.claim(target, last_run)
    return last_run == nil or target > last_run
end

-- label is hour:min on a 24-hour clock.
function Daily.label(hour, min)
    return string.format("%02d:%02d", hour, min)
end

return Daily
