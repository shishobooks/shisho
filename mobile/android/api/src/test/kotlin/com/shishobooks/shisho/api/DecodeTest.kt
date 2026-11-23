package com.shishobooks.shisho.api

import com.shishobooks.shisho.api.models.FileType
import com.shishobooks.shisho.api.shishoapp.FeedEntry
import com.shishobooks.shisho.api.shishoapp.FeedKind
import com.shishobooks.shisho.api.shishoapp.FeedResponse
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

// The generated types decode leniently (ADR 0021), so a newer server can't
// break an older app. These cases match the Swift decode tests.
class DecodeTest {
    private val entry = """
        {
          "kind": "book",
          "id": 7,
          "deleted": false,
          "library_id": 1,
          "a_field_from_a_newer_server": {"anything": [1, 2]},
          "book": {
            "id": 7,
            "library_id": 1,
            "title": "Dune",
            "sort_title": "Dune",
            "authors": null,
            "files": [{"id": 3, "file_type": "djvu", "file_role": "main", "size_bytes": 12345678901}],
            "cover_version": "3-1700000000"
          }
        }
    """.trimIndent()

    @Test
    fun ignoresAnExtraField() {
        val decoded = ShishoJson.decodeFromString<FeedEntry>(entry)
        assertEquals(7L, decoded.id)
        assertEquals(FeedKind.Book, decoded.kind)
    }

    @Test
    fun leavesAMissingOptionalFieldEmpty() {
        val book = ShishoJson.decodeFromString<FeedEntry>(entry).book!!
        // subtitle is a pointer in Go, so it's null when left out.
        assertNull(book.subtitle)
        // series was left out and authors sent as null: both are empty.
        assertEquals(emptyList(), book.series)
        assertEquals(emptyList(), book.authors)
    }

    @Test
    fun keepsAnUnknownEnumValue() {
        val book = ShishoJson.decodeFromString<FeedEntry>(entry).book!!
        assertEquals(FileType.Unknown("djvu"), book.files.single().fileType)
        assertEquals("djvu", book.files.single().fileType.raw)
        assertEquals(12345678901L, book.files.single().sizeBytes)

        val response = ShishoJson.decodeFromString<FeedResponse>("""{"entries":[{"kind":"highlight","id":1}],"cursor":9}""")
        assertEquals(FeedKind.Unknown("highlight"), response.entries.single().kind)
        assertEquals(9L, response.cursor)
    }

    @Test
    fun encodesKnownAndUnknownValuesAsSent() {
        val json = ShishoJson.encodeToString(FeedEntry.serializer(), FeedEntry(kind = FeedKind.Unknown("highlight"), id = 1))
        assertEquals(true, json.contains("\"kind\":\"highlight\""))
    }
}
