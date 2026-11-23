// After listening to the end, the server has the place at the end, a
// Reading Session from the Shisho app attributed to the phone's Device, and
// a finished Read-through.
// It polls the server until the check passes or 25 s pass, since the sync
// can land late on a stalled CI simulator, and records "ok", or what's still
// wrong, in output.serverCheck for the flow's assertTrue.

// What the server holds of listening to TITLE, kept in last to log.
let last;
function serverReading() {
  const response = http.get(
    CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  last = json(response.body);
  return last;
}

function check() {
  const reading = serverReading();
  if (reading.seconds < 7) {
    throw new Error(
      "the server's place is " + reading.seconds + " s, not the end",
    );
  }
  if (reading.source !== "shisho_app") {
    throw new Error("the place came from " + reading.source);
  }
  const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
  if (sessions.length === 0)
    throw new Error("the server has no session from the app");
  if (!sessions[0].device) throw new Error("the session names no Device");
  if (sessions[0].duration < 5) {
    throw new Error("the session lasted " + sessions[0].duration + " s");
  }
  if (reading.read_through !== "finished") {
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
console.log(JSON.stringify(last));
console.log("server check: " + output.serverCheck);
