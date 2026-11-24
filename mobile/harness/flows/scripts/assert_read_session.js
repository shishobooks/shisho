// After reading and closing the reader, the server has one more Reading
// Session from the Shisho app for TITLE, from the phone's Device, counting
// the pages read (each from 5 seconds), and the place it ended at. Reading
// past the first page starts a Read-through, as on the web.
// It polls the server until the check passes or 25 s pass, since the sync
// can land late on a stalled CI simulator, and records "ok", or what's still
// wrong, in output.serverCheck for the flow's assertTrue.

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
  console.log(JSON.stringify(reading));
  const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
  if (sessions.length <= output.appSessionsBefore) {
    throw new Error(
      "the server has " +
        sessions.length +
        " sessions from the app, as many as before reading",
    );
  }
  const session = sessions[sessions.length - 1];
  if (!session.device) throw new Error("the session names no Device");
  if (session.duration < 10) {
    throw new Error("the session counted " + session.duration + " s");
  }
  if (!/^epubcfi\(/.test(reading.cfi)) {
    throw new Error("the place isn't a CFI: " + JSON.stringify(reading.cfi));
  }
  if (
    reading.read_through !== "underway" &&
    reading.read_through !== "finished"
  ) {
    throw new Error(
      "the Read-through is " + JSON.stringify(reading.read_through),
    );
  }
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
console.log("server check: " + output.serverCheck);
