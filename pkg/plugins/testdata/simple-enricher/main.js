var plugin = (function() {
  var metadataEnricher = {
    name: "Simple Enricher",
    fileTypes: ["epub"],
    enrich: function(context) {
      return { modified: false };
    }
  };
  return { metadataEnricher: metadataEnricher };
})();
