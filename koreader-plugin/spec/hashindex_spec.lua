-- The index of document hashes of the books under KOReader's home folder.
local HashIndex = require("shisho/hashindex")
local lfs = require("libs/libkoreader-lfs")
local util = require("util")

local function write(path, content)
    local file = assert(io.open(path, "wb"))
    file:write(content)
    file:close()
end

-- counting wraps KOReader's own partial MD5, counting the files it hashes.
local function counting()
    local hashed = {}
    return function(path)
        table.insert(hashed, path)
        return util.partialMD5(path)
    end, hashed
end


return {
    ["scans the books under a folder, leaving out sidecars and hidden folders"] = function()
        local root = scratch_dir()
        lfs.mkdir(root .. "/Author")
        lfs.mkdir(root .. "/Author/one.sdr")
        lfs.mkdir(root .. "/.hidden")
        write(root .. "/Author/one.epub", "one")
        write(root .. "/Author/one.sdr/metadata.epub.lua", "return {}")
        write(root .. "/.hidden/two.epub", "two")
        write(root .. "/notes.txt", "notes")
        local files = HashIndex.scan(root .. "/")
        local paths = {}
        for path in pairs(files) do table.insert(paths, path) end
        assert_eq(paths, { root .. "/Author/one.epub" })
        assert_eq(files[root .. "/Author/one.epub"].size, 3)
    end,

    ["hashes a file again only when its size or modification time changes"] = function()
        local root = scratch_dir()
        write(root .. "/one.epub", "one")
        write(root .. "/two.epub", "two")
        local index = HashIndex.open(scratch_dir() .. "/shisho_hashes.lua")
        local hash, hashed = counting()

        assert_eq(index:refresh(HashIndex.scan(root), hash, 100), 2)
        assert_eq(index:files()[root .. "/one.epub"].hash, util.partialMD5(root .. "/one.epub"))

        -- Nothing changed: nothing is hashed.
        assert_eq(index:refresh(HashIndex.scan(root), hash, 200), 0)

        -- A new size is a new file.
        write(root .. "/one.epub", "one, longer")
        assert_eq(index:refresh(HashIndex.scan(root), hash, 300), 1)
        assert_eq(hashed[#hashed], root .. "/one.epub")
        assert_eq(index:files()[root .. "/one.epub"].hash, util.partialMD5(root .. "/one.epub"))

        -- So is a new modification time with the same size.
        lfs.touch(root .. "/two.epub", 1000, 1000)
        assert_eq(index:refresh(HashIndex.scan(root), hash, 400), 1)
        assert_eq(hashed[#hashed], root .. "/two.epub")
        assert_eq(#hashed, 4)
    end,

    ["drops files that are gone, and keeps what it knows across a restart"] = function()
        local root = scratch_dir()
        write(root .. "/one.epub", "one")
        write(root .. "/two.epub", "two")
        local path = scratch_dir() .. "/shisho_hashes.lua"
        local index = HashIndex.open(path)
        local hash, hashed = counting()
        index:refresh(HashIndex.scan(root), hash, 100)
        assert_eq(index:flush(), true)

        os.remove(root .. "/two.epub")
        local again = HashIndex.open(path)
        assert_eq(again:scannedAt(), 100)
        assert_eq(again:refresh(HashIndex.scan(root), hash, 200), 0)
        assert_eq(#hashed, 2)
        assert_eq(again:files()[root .. "/two.epub"], nil)
        assert(again:files()[root .. "/one.epub"], "the file still there is kept")
    end,

    ["indexes only the kinds of book Shisho delivers, and doesn't follow links"] = function()
        local root = scratch_dir()
        for __, name in ipairs({ "a.epub", "b.kepub.epub", "c.PDF", "d.cbz", "e.mobi", "f.azw3", "cover.jpg", "notes.txt", "page.html" }) do
            write(root .. "/" .. name, name)
        end
        lfs.mkdir(root .. "/inner")
        write(root .. "/inner/g.epub", "g")
        os.execute(string.format("ln -s %q %q", root, root .. "/inner/up"))
        local paths = {}
        for path in pairs(HashIndex.scan(root)) do table.insert(paths, path:sub(#root + 2)) end
        table.sort(paths)
        assert_eq(paths, { "a.epub", "b.kepub.epub", "c.PDF", "d.cbz", "e.mobi", "f.azw3", "inner/g.epub" })
    end,

    ["a large library is hashed over several scans"] = function()
        local root = scratch_dir()
        for i = 1, 5 do write(root .. "/" .. i .. ".epub", tostring(i)) end
        local index = HashIndex.open(scratch_dir() .. "/shisho_hashes.lua")
        local hash, hashed = counting()
        assert_eq(index:refresh(HashIndex.scan(root), hash, 100, 3), 3)
        assert_eq(index:scannedAt(), nil, "not done, so the next sync carries on")
        assert_eq(index:refresh(HashIndex.scan(root), hash, 200, 3), 2)
        assert_eq(index:scannedAt(), 200)
        assert_eq(#hashed, 5)
    end,

    ["a sync asked for scans; otherwise every few minutes"] = function()
        assert_eq(HashIndex.due(nil, 1000, false), true)
        assert_eq(HashIndex.due(1000, 1000 + 60, true), true)
        assert_eq(HashIndex.due(1000, 1000 + 60, false), false)
        assert_eq(HashIndex.due(1000, 1000 + HashIndex.SCAN_EVERY, false), true)
        -- A clock set back can't hold it off.
        assert_eq(HashIndex.due(1000, 500, false), true)
    end,
}
