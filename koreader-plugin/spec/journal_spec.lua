-- The journal of positions not yet acknowledged by every server.
local Journal = require("shisho/journal")
local State = require("shisho/state")

local function stateWith(...)
    local state = State.open(scratch_dir() .. "/shisho.lua")
    for __, url in ipairs({ ... }) do
        state:signIn{ url = url, username = "reader", credential = "C" }
    end
    return state
end

local function ids(events)
    local out = {}
    for __, e in ipairs(events) do table.insert(out, e.id) end
    return out
end

return {
    ["a newer position replaces the pending one for its book, under a new id"] = function()
        local state = stateWith("https://a.example")
        local journal = Journal.new(state)
        journal:recordPosition("doc1", "/body/p[1]", 0.1, 1000)
        local first = journal:pending("https://a.example")[1]
        journal:recordPosition("doc1", "/body/p[9]", 0.4, 1100)
        journal:recordPosition("doc2", "5", 0.2, 1200)

        local pending = journal:pending("https://a.example")
        assert_eq(#pending, 2)
        assert_eq(pending[1].document, "doc1")
        assert_eq(pending[1].progress, "/body/p[9]")
        assert_eq(pending[1].percentage, 0.4)
        assert_eq(pending[1].at, 1100)
        assert_eq(pending[1].type, "position")
        assert(pending[1].id ~= first.id, "a new id")
    end,

    ["the same place isn't recorded again"] = function()
        local state = stateWith("https://a.example")
        local journal = Journal.new(state)
        journal:recordPosition("doc1", "/body/p[1]", 0.1, 1000)
        local id = journal:pending("https://a.example")[1].id
        journal:acknowledge("https://a.example", { id })
        journal:prune()
        assert_eq(journal:recordPosition("doc1", "/body/p[1]", 0.1, 2000), false)
        assert_eq(#journal:pending("https://a.example"), 0)
        assert_eq(journal:lastPositionAt("doc1"), 1000)
    end,

    ["each server acknowledges for itself, and an entry stays until all have"] = function()
        local state = stateWith("https://a.example", "https://b.example")
        local journal = Journal.new(state)
        journal:recordPosition("doc1", "3", 0.3, 1000)
        local id = journal:pending("https://a.example")[1].id

        journal:acknowledge("https://a.example", { id })
        journal:prune()
        assert_eq(#journal:pending("https://a.example"), 0)
        assert_eq(ids(journal:pending("https://b.example")), { id })

        -- A server that needs a new sign-in keeps it queued.
        state:find("https://b.example").needs_login = true
        journal:prune()
        assert_eq(ids(journal:pending("https://b.example")), { id })

        journal:acknowledge("https://b.example", { id })
        journal:prune()
        assert_eq(#state:get("journal").entries, 0)
    end,

    ["a disabled server keeps nothing waiting"] = function()
        local state = stateWith("https://a.example", "https://b.example")
        local journal = Journal.new(state)
        journal:recordPosition("doc1", "3", 0.3, 1000)
        local id = journal:pending("https://a.example")[1].id
        state:setEnabled("https://b.example", false)

        journal:acknowledge("https://a.example", { id })
        journal:prune()

        assert_eq(#state:get("journal").entries, 0)
    end,

    ["pending is bounded"] = function()
        local state = stateWith("https://a.example")
        local journal = Journal.new(state)
        for i = 1, 5 do journal:recordPosition("doc" .. i, "1", 0, 1000 + i) end
        assert_eq(#journal:pending("https://a.example", 2), 2)
        assert_eq(journal:pending("https://a.example", 2)[1].document, "doc1")
    end,

    ["remembers when each book was last at its place, through the state file"] = function()
        local state = stateWith("https://a.example")
        local journal = Journal.new(state)
        journal:recordPosition("doc1", "3", 0.3, 1000)
        journal:recordPosition("doc1", "4", 0.4, 1500)
        assert_eq(state:flush(), true)

        local reopened = Journal.new(State.open(state.path))
        assert_eq(reopened:lastPositionAt("doc1"), 1500)
        assert_eq(reopened:lastPositionAt("doc2"), nil)
        assert_eq(#reopened:pending("https://a.example"), 1)
    end,
}
