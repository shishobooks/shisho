-- Server URLs, as a person means them.
local Urls = require("shisho/urls")

return {
    ["normalizes scheme, case, and trailing slashes"] = function()
        assert_eq(Urls.normalize(" HTTPS://Books.Example.com:8080/shisho/ "), "https://books.example.com:8080/shisho")
        assert_eq(Urls.normalize("books.example.com"), "http://books.example.com")
        assert_eq(Urls.normalize(""), nil)
        assert_eq(Urls.normalize(nil), nil)
    end,

    ["the path keeps its case"] = function()
        assert_eq(Urls.normalize("http://host/Shisho"), "http://host/Shisho")
        assert_eq(Urls.same("http://host/Shisho", "http://HOST/Shisho/"), true)
        assert_eq(Urls.same("http://host/a", "http://host/b"), false)
        assert_eq(Urls.same(nil, nil), false)
    end,

    ["tries https first for an address typed without a scheme"] = function()
        assert_eq(Urls.candidates("Books.Example.com/"), { "https://books.example.com", "http://books.example.com" })
        assert_eq(Urls.candidates("http://books.example.com"), { "http://books.example.com" })
        assert_eq(Urls.candidates(" https://books.example.com "), { "https://books.example.com" })
        assert_eq(Urls.candidates(""), {})
    end,

    ["labels a server by its host"] = function()
        assert_eq(Urls.label("https://books.example.com/"), "books.example.com")
    end,
}
