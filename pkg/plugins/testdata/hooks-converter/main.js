var plugin = (function() {
  return {
    inputConverter: {
      convert: function(context) {
        var content = shisho.fs.readTextFile(context.sourcePath);
        var targetPath = context.targetDir + "/output.epub";
        shisho.fs.writeTextFile(targetPath, "converted:" + content);
        return { success: true, targetPath: targetPath };
      }
    }
  };
})();
