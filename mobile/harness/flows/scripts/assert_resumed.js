// The app reopens the audiobook where it last saved its place before it was
// killed: no later than the place seen then, and at most one save interval
// (5 s, plus tap latency) earlier. The server has the place too.

// Seconds in the last clock time ("0:12" or "1:02:03") of a label such as
// "Elapsed 0:12", which the player's position shows.
function seconds(label) {
  const match = /(\d+:)?\d+:\d+(?!.*\d+:\d+)/.exec(String(label));
  if (!match) throw new Error("no time in " + JSON.stringify(label));
  const parts = match[0].split(":").map(Number);
  return parts.length === 3
    ? parts[0] * 3600 + parts[1] * 60 + parts[2]
    : parts[0] * 60 + parts[1];
}

// What the server holds of listening to TITLE.
function serverReading() {
  const response = http.get(
    CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  return json(response.body);
}
const before = seconds(output.beforeKill);
const after = seconds(output.afterRelaunch);
console.log("before kill " + before + " s, after relaunch " + after + " s");
if (after < before - 8 || after > before + 2) {
  throw new Error(
    "resumed at " +
      output.afterRelaunch +
      ", but the place before the kill was " +
      output.beforeKill,
  );
}
const reading = serverReading();
console.log("server place " + reading.seconds + " s from " + reading.source);
if (Math.abs(reading.seconds - after) > 2) {
  throw new Error(
    "the server's place is " +
      reading.seconds +
      " s, the app's " +
      after +
      " s",
  );
}
