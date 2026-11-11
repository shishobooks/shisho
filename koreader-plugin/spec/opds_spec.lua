-- A Shisho server's entry in KOReader's OPDS catalog list.
local Opds = require("shisho/opds")

local entry = {
    title = "Shisho (books.example.com)",
    url = "https://books.example.com/opds/v1/catalog",
    username = "reader",
    password = "NEWCREDENTIAL",
}

return {
    ["the catalog URL is the server's OPDS catalog"] = function()
        assert_eq(Opds.catalogURL("https://Books.Example.com/"), "https://books.example.com/opds/v1/catalog")
    end,

    ["adds the catalog when the list doesn't have it"] = function()
        local servers = { { title = "Project Gutenberg", url = "https://m.gutenberg.org/ebooks.opds/?format=opds" } }
        local added, is_new = Opds.upsert(servers, entry)
        assert_eq(is_new, true)
        assert_eq(#servers, 2)
        assert_eq(servers[2], entry)
        assert_eq(added, entry)
    end,

    ["updates an entry for the same URL instead of adding another"] = function()
        local servers = {
            { title = "My library", url = "HTTPS://books.example.com/opds/v1/catalog/", username = "old", password = "OLD" },
        }
        local updated, is_new = Opds.upsert(servers, entry)
        assert_eq(is_new, false)
        assert_eq(#servers, 1)
        assert_eq(updated.title, "My library", "a title the user chose stays")
        assert_eq(updated.username, "reader")
        assert_eq(updated.password, "NEWCREDENTIAL")
    end,

    ["leaves other catalogs alone"] = function()
        local other = { title = "Other", url = "https://other.example.com/opds/v1/catalog", username = "x", password = "y" }
        local servers = { other }
        Opds.upsert(servers, entry)
        assert_eq(servers[1], { title = "Other", url = "https://other.example.com/opds/v1/catalog", username = "x", password = "y" })
    end,
}
