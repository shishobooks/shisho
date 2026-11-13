-- Daily sync: its settings and when it next runs.
local Daily = require("shisho/daily")

-- at is a local time today, as os.time reads one.
local function at(hour, min, sec, day_offset)
    local t = os.date("*t", os.time())
    t.hour, t.min, t.sec = hour, min, sec or 0
    t.day = t.day + (day_offset or 0)
    return os.time(t)
end

return {
    ["settings default to off at 4:00"] = function()
        assert_eq(Daily.settings(nil), { enabled = false, hour = 4, min = 0 })
        assert_eq(Daily.settings({ daily_sync = true, daily_sync_hour = 2, daily_sync_min = 30 }),
            { enabled = true, hour = 2, min = 30 })
        assert_eq(Daily.settings({ daily_sync_hour = 25, daily_sync_min = -1 }), { enabled = false, hour = 4, min = 0 },
            "a time that can't be falls back to 4:00")
    end,

    ["runs later today, or tomorrow once today's time has passed"] = function()
        assert_eq(Daily.nextRun(at(1, 0), 4, 0), at(4, 0))
        assert_eq(Daily.nextRun(at(4, 0), 4, 0), at(4, 0, 0, 1), "a run due now is tomorrow's")
        assert_eq(Daily.nextRun(at(23, 59), 4, 0), at(4, 0, 0, 1))
        assert_eq(Daily.nextRun(at(3, 59, 59), 4, 0), at(4, 0))
    end,

    ["labels the time on a 24-hour clock"] = function()
        assert_eq(Daily.label(4, 0), "04:00")
        assert_eq(Daily.label(23, 5), "23:05")
    end,
}
