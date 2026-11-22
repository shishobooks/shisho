// Listening done offline reaches the server placed at the time it happened,
// not when it synced: the session ended before the server came back, and
// counts the time listened offline.

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
const reading = serverReading();
console.log(JSON.stringify(reading));
const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
if (sessions.length === 0)
  throw new Error("the server has no session from the app");
const session = sessions[sessions.length - 1];
if (session.ended_at > reading.online_at) {
  throw new Error(
    "the session ended at " +
      session.ended_at +
      ", after the server came back at " +
      reading.online_at +
      ": it was placed when it synced",
  );
}
if (session.ended_at < reading.offline_at) {
  throw new Error("the session ended before the server went offline");
}
if (session.duration < 12) {
  throw new Error(
    "the session counts " + session.duration + " s, not the time offline",
  );
}
if (Math.abs(reading.seconds - seconds(output.paused)) > 6) {
  throw new Error(
    "the server's place is " +
      reading.seconds +
      " s, the app paused at " +
      output.paused,
  );
}
if (reading.occurred_at > reading.online_at) {
  throw new Error("the place was placed when it synced");
}
