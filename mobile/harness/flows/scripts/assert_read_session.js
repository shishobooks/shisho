// After reading and closing the reader, the server has one more Reading
// Session from the Shisho app for TITLE, from the phone's Device, counting
// the pages read (each from 5 seconds), and the place it ended at. Reading
// past the first page starts a Read-through, as on the web.
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
  output.serverCheck = "ok";
} catch (e) {
  output.serverCheck = String(e.message || e);
  console.log("server check: " + output.serverCheck);
}
