-- Writing the plugin's files and books' sidecars.
local DocSettings = require("docsettings")
local Files = require("shisho/files")
local lfs = require("libs/libkoreader-lfs")

local function write(path, content)
    local file = assert(io.open(path, "wb"))
    file:write(content)
    file:close()
end

-- withLocation runs fn with KOReader's sidecars kept in location ("doc",
-- "dir", or "hash"), putting the setting back after.
local function withLocation(location, fn)
    local saved = G_reader_settings:readSetting("document_metadata_folder")
    G_reader_settings:saveSetting("document_metadata_folder", location)
    local ok, err = pcall(fn)
    G_reader_settings:saveSetting("document_metadata_folder", saved)
    if not ok then error(err, 0) end
end

-- openedForWriting runs fn and returns the paths it opened for writing.
local function openedForWriting(fn)
    local opened = {}
    local real_open = io.open
    io.open = function(path, mode, ...)
        if mode and mode:find("[wa]") then table.insert(opened, path) end
        return real_open(path, mode, ...)
    end
    local ok, err = pcall(fn)
    io.open = real_open
    if not ok then error(err, 0) end
    return opened
end

return {
    -- KOReader writes a sidecar in place, and DocSettings:open deletes one
    -- it finds empty or cut short, which the file browser's cover
    -- extraction, in a process of its own, can do mid-write.
    ["writes a sidecar without ever truncating it in place"] = function()
        withLocation("dir", function()
            local book = scratch_dir() .. "/book.epub"
            write(book, "book")
            local first = DocSettings:open(book)
            first:saveSetting("summary", { status = "complete" })
            first:flush()
            local sidecar = DocSettings:findSidecarFile(book)

            local doc_settings = DocSettings:open(book)
            doc_settings:saveSetting("summary", { status = "reading" })
            local opened = openedForWriting(function() Files.writeSidecar(doc_settings) end)

            for __, path in ipairs(opened) do
                assert(path ~= sidecar, "the sidecar was opened for writing in place")
            end
            assert_eq(DocSettings:findSidecarFile(book), sidecar)
            assert_eq(DocSettings:open(book):readSetting("summary"), { status = "reading" })
            assert_eq(lfs.attributes(sidecar .. ".tmp", "mode"), nil, "no temporary file is left")
        end)
    end,

    ["writes a sidecar where KOReader keeps them, removing one kept elsewhere"] = function()
        local book = scratch_dir() .. "/book.epub"
        write(book, "book")
        local beside
        withLocation("doc", function()
            local doc_settings = DocSettings:open(book)
            doc_settings:saveSetting("summary", { status = "complete" })
            doc_settings:flush()
            beside = DocSettings:findSidecarFile(book)
        end)
        withLocation("dir", function()
            local doc_settings = DocSettings:open(book)
            doc_settings:saveSetting("summary", { status = "reading" })
            Files.writeSidecar(doc_settings)
            local sidecar, location = DocSettings:findSidecarFile(book)
            assert_eq(location, "dir")
            assert(sidecar ~= beside)
            assert_eq(lfs.attributes(beside, "mode"), nil, "the copy beside the book is gone")
            assert_eq(DocSettings:open(book):readSetting("summary"), { status = "reading" })
        end)
    end,

    -- writeSidecar reads DocSettings fields that aren't KOReader's API. A
    -- KOReader without them still gets the sidecar saved, by its own write.
    ["falls back to KOReader's own write without the fields it reads"] = function()
        withLocation("doc", function()
            local flushed = false
            local doc_settings = { data = {}, flush = function() flushed = true end }
            Files.writeSidecar(doc_settings)
            assert(flushed, "flush was not called")
        end)
    end,

    -- A full disk takes the buffered write, then fails it when flushed, which
    -- must not replace a good file with an empty one.
    ["keeps the old file when the disk is full"] = function()
        local path = scratch_dir() .. "/state.lua"
        assert(Files.write(path, { kept = true }, "state"))
        -- The temporary file lands on a device that is always full.
        assert(os.execute("ln -s /dev/full '" .. path .. ".tmp'") == 0)

        assert_eq(Files.write(path, { kept = false }, "state"), nil)
        assert_eq(lfs.attributes(path, "mode"), "file")
        assert_eq(dofile(path), { kept = true })
        assert_eq(lfs.symlinkattributes(path .. ".tmp", "mode"), nil, "no temporary file is left")
    end,

    -- KOReader keeps a custom title or cover beside the sidecar, and moves it
    -- when it saves the sidecar elsewhere. One left behind would shadow a
    -- newer one KOReader writes in the new place.
    ["moves a custom metadata file with the sidecar, as KOReader does"] = function()
        local book = scratch_dir() .. "/book.epub"
        write(book, "book")
        local beside
        withLocation("doc", function()
            local doc_settings = DocSettings:open(book)
            doc_settings:saveSetting("summary", { status = "complete" })
            doc_settings:flush()
            beside = DocSettings:findSidecarFile(book)
            write(beside:gsub("[^/]+$", "custom_metadata.lua"), "return { custom_props = { title = \"Mine\" } }")
        end)
        withLocation("dir", function()
            local doc_settings = DocSettings:open(book)
            doc_settings:saveSetting("summary", { status = "reading" })
            local dir = Files.writeSidecar(doc_settings)
            assert_eq(DocSettings:findCustomMetadataFile(book), dir .. "/custom_metadata.lua")
            assert_eq(lfs.attributes(beside:gsub("[^/]+$", "custom_metadata.lua"), "mode"), nil, "the copy beside the book is gone")
        end)
    end,
}
