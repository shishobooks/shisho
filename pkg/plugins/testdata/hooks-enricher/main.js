var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(context) {
        return {
          results: [
            {
              title: "Search: " + context.query,
              authors: [{ name: "Search Author", role: "writer" }],
              description: "Found result for " + context.query,
              publisher: "Search Publisher",
              subtitle: "A Search Subtitle",
              series: "Search Series",
              seriesNumber: 2.5,
              genres: ["Fiction", "Fantasy"],
              tags: ["epic", "adventure"],
              narrators: ["Narrator One", "Narrator Two"],
              identifiers: [
                { type: "goodreads", value: "12345" }
              ],
              imprint: "Search Imprint",
              url: "https://example.com/book",
              coverUrl: "https://example.com/cover.jpg"
            }
          ]
        };
      }
    }
  };
})();
