var plugin = (function() {
  return {
    outputGenerator: {
      generate: function(context) {
        var content = shisho.fs.readTextFile(context.sourcePath);
        shisho.fs.writeTextFile(context.destPath, "generated:" + content);
      },
      fingerprint: function(context) {
        return "fp-" + context.book.title + "-" + context.file.fileType;
      }
    }
  };
})();
