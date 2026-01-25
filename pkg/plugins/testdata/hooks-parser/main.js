var plugin = (function() {
  return {
    fileParser: {
      parse: function(context) {
        return {
          title: "Test Book",
          subtitle: "A Subtitle",
          authors: [
            { name: "Author One", role: "writer" },
            { name: "Author Two", role: "" }
          ],
          narrators: ["Narrator One", "Narrator Two"],
          series: "Test Series",
          seriesNumber: 2.5,
          genres: ["Fiction", "Fantasy"],
          tags: ["epic", "adventure"],
          description: "A test book description",
          publisher: "Test Publisher",
          imprint: "Test Imprint",
          url: "https://example.com/book",
          releaseDate: "2023-06-15T00:00:00Z",
          coverMimeType: "image/jpeg",
          coverData: new Uint8Array([0xFF, 0xD8, 0xFF, 0xE0]).buffer,
          coverPage: 0,
          duration: 3661.5,
          bitrateBps: 128000,
          pageCount: 42,
          identifiers: [
            { type: "isbn_13", value: "9781234567890" },
            { type: "asin", value: "B01ABCDEFG" }
          ],
          chapters: [
            {
              title: "Chapter 1",
              startPage: 0,
              children: [
                { title: "Section 1.1", startPage: 2 }
              ]
            },
            { title: "Chapter 2", startPage: 10 }
          ]
        };
      }
    }
  };
})();
