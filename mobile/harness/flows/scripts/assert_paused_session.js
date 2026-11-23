// A pause sends the session so far, so the server has it without waiting
// for the sitting to end: one more than before this flow listened
// (note_app_sessions.js). It also notes the seconds of listening the server
// counts so far, for assert_offline_session.js.
// It polls the server until the check passes or 25 s pass, since the sync
// can land late on a stalled CI simulator, and records "ok", or what's still
// wrong, in output.serverCheck for the flow's assertTrue.

let last;
function check() {
  const response = http.get(
    CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  const reading = json(response.body);
  last = reading;
  const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
  if (sessions.length <= output.appSessionsBefore) {
    throw new Error("the server has no session from the pause");
  }
  output.appSecondsBeforeOffline = sessions.reduce(
    (total, s) => total + s.duration,
    0,
  );
}

const deadline = Date.now() + 25000;
for (;;) {
  try {
    check();
    output.serverCheck = "ok";
  } catch (e) {
    output.serverCheck = String(e.message || e);
  }
  if (output.serverCheck === "ok" || Date.now() > deadline) break;
  http.get(CONTROL + "/wait?ms=500");
}
console.log(JSON.stringify(last));
console.log("server check: " + output.serverCheck);
