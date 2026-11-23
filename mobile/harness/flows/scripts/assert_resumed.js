// The app reopens the audiobook where it last saved its place before it was
// killed: at most one save interval (5 s, plus tap latency) before the place
// seen then, and no later than that place plus the time it kept playing until
// the kill. The server has the place too.
// It polls the server until the check passes or 25 s pass, since the sync
// can land late on a stalled CI simulator, and records "ok", or what's still
// wrong, in output.serverCheck for the flow's assertTrue.

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

// Where the app resumed, against what the flow saw before the kill. It
// can't change, so it's checked once, before polling the server.
function checkResumed() {
  const before = seconds(output.beforeKill);
  const after = seconds(output.afterRelaunch);
  // Seconds the player kept going between the flow seeing the place and the
  // kill, which a slow simulator can stretch past a save.
  const untilKill = Math.ceil((output.killedAt - output.seenAt) / 1000);
  if (after < before - 8 || after > before + untilKill + 2) {
    throw new Error(
      "resumed at " +
        output.afterRelaunch +
        ", but the place before the kill was " +
        output.beforeKill,
    );
  }
}

function checkServer() {
  const after = seconds(output.afterRelaunch);
  const reading = serverReading();
  if (Math.abs(reading.seconds - after) > 2) {
    throw new Error(
      "the server's place is " +
        reading.seconds +
        " s from " +
        reading.source +
        ", the app's " +
        after +
        " s",
    );
  }
}

output.serverCheck = "";
try {
  checkResumed();
} catch (e) {
  output.serverCheck = String(e.message || e);
}
const deadline = Date.now() + 25000;
while (output.serverCheck === "") {
  try {
    checkServer();
    output.serverCheck = "ok";
  } catch (e) {
    if (Date.now() > deadline) output.serverCheck = String(e.message || e);
  }
  if (output.serverCheck === "") http.get(CONTROL + "/wait?ms=500");
}
console.log(JSON.stringify(last));
console.log(
  "before kill " +
    output.beforeKill +
    ", after relaunch " +
    output.afterRelaunch +
    "; server check: " +
    output.serverCheck,
);
