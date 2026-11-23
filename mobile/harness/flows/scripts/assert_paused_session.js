// A pause sends the session so far, so the server has it without waiting
// for the sitting to end: one more than before this flow listened
// (note_app_sessions.js). It also notes the seconds of listening the server
// counts so far, for assert_offline_session.js.
// It records "ok", or what's wrong, in output.serverCheck rather than
// throwing: the flow checks that with assertTrue inside a retry, which
// retries Maestro's own failures but not a script's error.

output.serverCheck = "";
try {
  const response = http.get(
    CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  const reading = json(response.body);
  console.log(JSON.stringify(reading));
  const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
  if (sessions.length <= output.appSessionsBefore) {
    throw new Error("the server has no session from the pause");
  }
  output.appSecondsBeforeOffline = sessions.reduce(
    (total, s) => total + s.duration,
    0,
  );
  output.serverCheck = "ok";
} catch (e) {
  output.serverCheck = String(e.message || e);
  console.log("server check: " + output.serverCheck);
}
