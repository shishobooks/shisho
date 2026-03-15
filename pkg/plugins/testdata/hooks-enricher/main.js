var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(context) {
        return {
          results: [
            {
              title: "Search: " + context.query,
              authors: ["Search Author"],
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
              providerData: { internalId: 42, query: context.query },
              metadata: {
                title: "Search: " + context.query,
                subtitle: "A Search Subtitle",
                series: "Search Series",
                seriesNumber: 2.5,
                genres: ["Fiction", "Fantasy"],
                tags: ["epic", "adventure"],
                narrators: ["Narrator One", "Narrator Two"],
                authors: [{ name: "Search Author", role: "writer" }],
                description: "Found result for " + context.query,
                publisher: "Search Publisher",
                coverUrl: "https://example.com/cover.jpg",
                identifiers: [
                  { type: "goodreads", value: "12345" }
                ]
              }
            }
          ]
        };
      },
      enrich: function(context) {
        var title = "";
        if (context.selectedResult && context.selectedResult.query) {
          title = context.selectedResult.query;
        } else if (context.book && context.book.title) {
          title = context.book.title;
        }
        return {
          modified: true,
          metadata: {
            description: "Enriched: " + title,
            genres: ["Enriched Genre"],
            tags: ["enriched-tag"],
            identifiers: [
              { type: "goodreads", value: "12345" }
            ],
            series: "Enriched Series",
            seriesNumber: 3,
            publisher: "Enriched Publisher",
            url: "https://enriched.example.com",
            coverUrl: "https://example.com/enriched-cover.jpg"
          }
        };
      }
    }
  };
})();
