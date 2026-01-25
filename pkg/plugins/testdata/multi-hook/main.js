var plugin = (function() {
  return {
    inputConverter: {
      sourceTypes: ["pdf"],
      targetType: "epub",
      convert: function(context) { return { success: true, targetPath: "" }; }
    },
    fileParser: {
      types: ["pdf"],
      parse: function(context) { return { title: "Test" }; }
    }
  };
})();
