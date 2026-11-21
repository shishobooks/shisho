// After listening to the end, the server has the place at the end, a
// Reading Session from the Shisho app attributed to the phone's Device, and
// a finished Read-through.

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
    throw new Error("the harness could not read the server: " + response.status);
  }
  return json(response.body);
}
const reading = serverReading();
console.log(JSON.stringify(reading));
if (reading.seconds < 7) {
  throw new Error("the server's place is " + reading.seconds + " s, not the end");
}
if (reading.source !== "shisho_app") {
  throw new Error("the place came from " + reading.source);
}
const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
if (sessions.length === 0) throw new Error("the server has no session from the app");
if (!sessions[0].device) throw new Error("the session names no Device");
if (sessions[0].duration < 5) {
  throw new Error("the session lasted " + sessions[0].duration + " s");
}
if (reading.read_through !== "finished") {
  throw new Error("the Read-through is " + JSON.stringify(reading.read_through));
}
