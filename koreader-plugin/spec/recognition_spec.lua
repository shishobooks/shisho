-- Which server recognizes which book, and where each book's data goes.
local Recognition = require("shisho/recognition")

local A, B = "https://a.example", "https://b.example"

local function open()
    return Recognition.open(scratch_dir() .. "/shisho_recognition.lua")
end

local function servers(...)
    local out = {}
    for __, url in ipairs({ ... }) do table.insert(out, { url = url }) end
    return out
end

local function answer(r, url, document, state, at)
    r:record(url, { { document = document, state = state } }, at)
end

return {
    ["a book no server recognizes goes as unrecognized once every server has answered"] = function()
        local r = open()
        local both = servers(A, B)
        r:noteSeen({ "old" }, 100)
        answer(r, A, "old", "unknown", 100)
        assert_eq(r:decide(both, A, "old"), "wait", "B hasn't answered")
        answer(r, B, "old", "unknown", 101)
        assert_eq(r:decide(both, A, "old"), "unrecognized")
        assert_eq(r:decide(both, B, "old"), "unrecognized")
    end,

    ["an answer from before the book first appeared doesn't count"] = function()
        local r = open()
        local both = servers(A, B)
        answer(r, A, "old", "unknown", 50)
        answer(r, B, "old", "unknown", 50)
        r:noteSeen({ "old" }, 100)
        assert_eq(r:decide(both, A, "old"), "wait")
        r:noteSeen({ "old" }, 200)
        answer(r, A, "old", "unknown", 150)
        answer(r, B, "old", "unknown", 150)
        assert_eq(r:decide(both, A, "old"), "unrecognized", "the first time seen is kept")
    end,

    ["a server that's down keeps a book no other server recognizes waiting"] = function()
        local r = open()
        local both = servers(A, B)
        r:noteSeen({ "b-book", "old" }, 100)
        -- A answered; B is down, so it never did.
        answer(r, A, "b-book", "unknown", 100)
        answer(r, A, "old", "unknown", 100)
        assert_eq(r:decide(both, A, "b-book"), "wait", "B's book isn't listed as unrecognized on A")
        assert_eq(r:decide(both, A, "old"), "wait")
    end,

    ["a server that needs a new sign-in doesn't count as answering"] = function()
        local r = open()
        local both = servers(A, B)
        both[2].needs_login = true
        r:noteSeen({ "old" }, 100)
        answer(r, A, "old", "unknown", 100)
        answer(r, B, "old", "unknown", 100)
        assert_eq(r:decide(both, A, "old"), "wait")
        both[2].needs_login = nil
        assert_eq(r:decide(both, A, "old"), "unrecognized")
    end,

    ["each server gets the books it recognizes, and nothing of another's"] = function()
        local r = open()
        local both = servers(A, B)
        r:noteSeen({ "a-book", "both" }, 100)
        answer(r, A, "a-book", "recognized", 100)
        answer(r, B, "a-book", "unknown", 100)
        answer(r, A, "both", "recognized", 100)
        answer(r, B, "both", "recognized", 100)
        assert_eq(r:decide(both, A, "a-book"), "send")
        assert_eq(r:decide(both, B, "a-book"), "skip")
        assert_eq(r:decide(both, A, "both"), "send")
        assert_eq(r:decide(both, B, "both"), "send")
    end,

    ["a book linked on one server is claimed from another that held it"] = function()
        local r = open()
        local both = servers(A, B)
        r:noteSeen({ "old" }, 100)
        answer(r, A, "old", "recognized", 200)
        answer(r, B, "old", "held", 200)
        assert_eq(r:decide(both, B, "old"), "claim")
        assert_eq(r:claims(both, B, { "old", "other" }), { "old" })
        r:claimed(B, { "old" }, 210)
        assert_eq(r:decide(both, B, "old"), "skip", "claimed once")
    end,

    ["a book a server holds or dismissed goes to it as it is"] = function()
        local r = open()
        local both = servers(A, B)
        r:noteSeen({ "old", "gone" }, 100)
        answer(r, A, "old", "held", 100)
        answer(r, A, "gone", "dismissed", 100)
        assert_eq(r:decide(both, A, "old"), "send", "even before B answers")
        assert_eq(r:decide(both, A, "gone"), "send", "the server drops it")
        assert_eq(r:held(), { "old" })
    end,

    ["a disabled server isn't waited for"] = function()
        local r = open()
        r:noteSeen({ "old" }, 100)
        answer(r, A, "old", "unknown", 100)
        -- Only enabled servers are passed in.
        assert_eq(r:decide(servers(A), A, "old"), "unrecognized")
    end,

    ["deferred books are kept by server, and removing a server forgets only its own"] = function()
        local r = open()
        r:defer(A, "old")
        r:defer(B, "old")
        r:defer(B, "other")
        answer(r, A, "old", "held", 100)
        answer(r, B, "old", "held", 100)
        r:remove(B)
        assert_eq(r:deferred(B), {})
        assert_eq(r:answer(B, "old"), nil)
        assert_eq(r:deferred(A), { "old" })
        assert_eq(r:answer(A, "old").state, "held")
        r:undefer(A, "old")
        assert_eq(r:deferred(A), {})
        assert_eq(r:flush(), true)
        assert_eq(Recognition.open(r.path):answer(A, "old").state, "held")
    end,

    ["unknown answers are ignored"] = function()
        local r = open()
        r:record(A, { { document = "old", state = "maybe" }, "junk", { state = "held" } }, 100)
        assert_eq(r:answer(A, "old"), nil)
    end,

    ["KOReader's one-per-line values become lists"] = function()
        assert_eq(Recognition.lines("isbn:9780441478125\n uuid:abc \n\n"), { "isbn:9780441478125", "uuid:abc" })
        assert_eq(Recognition.lines({ "a", " b " }), { "a", "b" })
        assert_eq(Recognition.lines(nil), {})
    end,

    ["rounds count syncs, so a clock set back can't hold a book up"] = function()
        local r = open()
        local first = r:beginRound()
        assert_eq(r:current(), first)
        r:noteSeen({ "old" }, first)
        local second = r:beginRound()
        assert_eq(second, first + 1)
        r:record(A, { { document = "old", state = "unknown" } }, second)
        assert_eq(r:decide(servers(A), A, "old"), "unrecognized")
    end,

    ["books waiting come from every source and each server's held books"] = function()
        local r = open()
        r:defer(A, "deferred")
        r:record(B, { { document = "held", state = "held" } }, 1)
        local plans = {
            { server = { url = A }, changes = { { document = "changed" } }, history = { { document = "history" } } },
        }
        local documents = r:waiting(plans, {
            statistics = function(server) return server.url == A and { "read", "changed" } or {} end,
            journal = function() return { { document = "position" } } end,
        })
        assert_eq(documents, { "changed", "deferred", "held", "history", "position", "read" })
    end,

    ["what KOReader knows of a book takes statistics first, then the sidecar, cut to what a server takes"] = function()
        local about = Recognition.about(
            { title = "From statistics", authors = {}, reading_seconds = 60, first_read = 1, last_read = 2 },
            "/books/Old Copy.kepub.epub",
            { title = "From the sidecar", authors = "A. Author\nB. Author", language = "en", identifiers = "isbn:1\nuuid:2" })
        assert_eq(about, {
            title = "From statistics", authors = { "A. Author", "B. Author" }, identifiers = { "isbn:1", "uuid:2" },
            language = "en", file_type = "kepub", reading_seconds = 60, first_read = 1, last_read = 2,
        })
        local long = string.rep("x", 2000)
        local many = {}
        for i = 1, 60 do table.insert(many, "Author " .. i) end
        local clipped = Recognition.about({ title = long, authors = many }, "/books/b.something-very-long-ext", nil)
        assert_eq(#clipped.title, Recognition.LIMITS.text)
        assert_eq(#clipped.authors, Recognition.LIMITS.list)
        assert_eq(clipped.file_type, nil)
        assert_eq(clipped.identifiers, nil, "an empty list is left out")
        assert_eq(Recognition.about(nil, nil, nil).title, nil)
    end,

    ["history sent again asks about the books whose reading history a server deleted"] = function()
        local r = open()
        r:record(A, { { document = "deleted", state = "dismissed" } }, 1)
        r:record(B, { { document = "elsewhere", state = "dismissed" } }, 1)
        local sources = { statistics = function() return {} end }
        assert_eq(r:waiting({ { server = { url = A, history = { through = 0 } } } }, sources), {})
        assert_eq(r:waiting({ { server = { url = A, history = { through = 0, resend = true } } } }, sources), { "deleted" },
            "only the server being sent history again, which may have restored it")
    end,

    ["a copy a server says it holds bare is described, unless this round it recognized it"] = function()
        local r = open()
        local round = r:beginRound()
        r:record(A, { { document = "now-known", state = "recognized" } }, round)
        r:undescribed(A, { "bare", "now-known" }, round)
        assert_eq(r:answer(A, "now-known").state, "recognized")
        assert_eq(r:needingDetails(A), { "bare" })
    end,

    ["a copy KOReader knows nothing of isn't asked about again until a while later"] = function()
        local r = open()
        local round = r:beginRound()
        r:undescribed(A, { "gone" }, round)
        r:cannotDescribe(A, "gone", round)
        assert_eq(r:needingDetails(A), {})
        -- Asked again at the next lookup, it's still left alone.
        round = r:beginRound()
        r:undescribed(A, { "gone" }, round)
        assert_eq(r:needingDetails(A), {})
        for _ = 1, Recognition.DESCRIBE_RETRY do r:beginRound() end
        assert_eq(r:needingDetails(A), { "gone" }, "tried again, as when it's back on the device")
    end,

    ["a title of only spaces is no title"] = function()
        assert_eq(Recognition.about({ title = "   " }, nil, nil).title, nil)
    end,
}

