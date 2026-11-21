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
}
