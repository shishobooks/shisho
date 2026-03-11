var plugin = (function() {
  return {
    inputConverter: {
      sourceTypes: ["docx"],
      targetType: "epub",
      convert: function(context) { return { success: true, targetPath: "" }; }
    },
    fileParser: {
      types: ["docx"],
      parse: function(context) { return { title: "Test" }; }
    }
  };
})();
