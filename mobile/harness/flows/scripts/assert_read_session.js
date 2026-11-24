// After reading and closing the reader, the server has a Reading Session
// from the Shisho app for TITLE that started after the app was relaunched
// (output.relaunchedAt) and ended after the flow's wait on the second page
// (output.dwellEnded), from the phone's Device, counting the pages read
// (each from 5 seconds), and the place it ended at: the second page
// (output.secondPage), not the one reopened at (output.reopenedCfi).
// Reading past the first page starts a Read-through, as on the web. A session is told by when it
// started, not by counting them: the server takes in each journal event in
// its own transaction, so the session from before the kill can land after
// the place the flow checked first.
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
  // Seconds, as the server gives them, less one for the rounding.
  const relaunched = Math.floor(output.relaunchedAt / 1000) - 1;
  const sessions = reading.sessions.filter(
    (s) => s.source === "shisho_app" && s.started_at >= relaunched,
  );
  if (sessions.length === 0) {
    throw new Error("the server has no session from after the relaunch");
  }
  const session = sessions[sessions.length - 1];
  const dwellEnded = Math.floor(output.dwellEnded / 1000) - 1;
  if (session.ended_at < dwellEnded) {
    throw new Error(
      "the session ended at " +
        session.ended_at +
        ", before the second page's wait ended at " +
        dwellEnded,
    );
  }
  if (!session.device) throw new Error("the session names no Device");
  if (session.duration < 10) {
    throw new Error("the session counted " + session.duration + " s");
  }
  if (!/^epubcfi\(/.test(reading.cfi)) {
    throw new Error("the place isn't a CFI: " + JSON.stringify(reading.cfi));
  }
  if (reading.cfi === output.reopenedCfi) {
    throw new Error("the place is still the page reopened at");
  }
  const shown = /(\d+)%/.exec(String(output.secondPage));
  if (!shown) throw new Error("no percentage in " + output.secondPage);
  if (Math.abs(reading.percentage * 100 - Number(shown[1])) > 1) {
    throw new Error(
      "the server's place is " +
        Math.round(reading.percentage * 100) +
        "%, the second page showed " +
        shown[1] +
        "%",
    );
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
