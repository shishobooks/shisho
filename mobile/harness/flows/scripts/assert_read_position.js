// The server has the reader's place in TITLE as a CFI from the Shisho app,
// as far through the book as the app showed after relaunching.
const response = http.get(
  CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
);
if (response.status !== 200) {
  throw new Error("the harness could not read the server: " + response.status);
}
const reading = json(response.body);
console.log(JSON.stringify(reading));
if (reading.source !== "shisho_app") {
  throw new Error("the place came from " + reading.source);
}
if (!/^epubcfi\(/.test(reading.cfi)) {
  throw new Error("the place isn't a CFI: " + JSON.stringify(reading.cfi));
}
if (!(reading.percentage > 0 && reading.percentage < 1)) {
  throw new Error("the place is " + reading.percentage + " through the book");
}
const shown = /(\d+)%/.exec(String(output.afterRelaunch));
if (!shown) throw new Error("no percentage in " + output.afterRelaunch);
if (Math.abs(reading.percentage * 100 - Number(shown[1])) > 1) {
  throw new Error(
    "the server's place is " +
      Math.round(reading.percentage * 100) +
      "%, the app showed " +
      shown[1] +
      "%",
  );
}
