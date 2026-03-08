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
              identifiers: [
                { type: "goodreads", value: "12345" }
              ],
              providerData: { internalId: 42, query: context.query }
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
            url: "https://enriched.example.com"
          }
        };
      }
    }
  };
})();
