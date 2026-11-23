// Listening done offline reaches the server placed at the time it happened,
// not when it synced: the session ended before the server came back, and
// the server counts the time listened offline. The server may add that time
// to the session the pause sent, so it's counted across all of them.
// It records "ok", or what's wrong, in output.serverCheck rather than
// throwing: the flow checks that with assertTrue inside a retry, which
// retries Maestro's own failures but not a script's error.

output.serverCheck = "";
try {
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
  const counted =
    sessions.reduce((total, s) => total + s.duration, 0) -
    output.appSecondsBeforeOffline;
  if (counted < 12) {
    throw new Error("the server counts " + counted + " s of listening offline");
  }
  // The player kept playing offline: the flow waited for the place to move
  // this far between its two pauses.
  const offline = seconds(output.paused) - seconds(output.onlinePaused);
  if (offline < 12) {
    throw new Error(
      "the place moved " +
        offline +
        " s offline, from " +
        output.onlinePaused +
        " to " +
        output.paused,
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
  output.serverCheck = "ok";
} catch (e) {
  output.serverCheck = String(e.message || e);
  console.log("server check: " + output.serverCheck);
}
