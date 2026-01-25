var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(context) {
        return {
          modified: true,
          metadata: {
            description: "Enriched: " + context.book.title,
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
