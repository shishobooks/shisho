var plugin = (function() {
  var metadataEnricher = {
    name: "Simple Enricher",
    fileTypes: ["epub"],
    search: function(context) {
      return { results: [] };
    },
    enrich: function(context) {
      return { modified: false };
    }
  };
  return { metadataEnricher: metadataEnricher };
})();
