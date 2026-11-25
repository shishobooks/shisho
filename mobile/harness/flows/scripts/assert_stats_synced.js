// Once listening done offline has synced, Reading Stats count it once: the
// listening time the app showed (output.shownListening, "Listening time,
// 12m") is the server's, formatted as the app formats it, and the server
// has no place left in TITLE, whose saved place was cleared offline. It
// polls the server until the check passes or 25 s pass, and records "ok",
// or what's still wrong, in output.serverCheck for the flow's assertTrue.

// A length as the app shows it: "6h 43m", "12m", or "<1m".
function duration(seconds) {
  if (seconds > 0 && seconds < 60) return "<1m";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return hours > 0 ? hours + "h " + minutes + "m" : minutes + "m";
}

let last;
function check() {
  const response = http.get(
    CONTROL + "/record?titles=" + encodeURIComponent(TITLE),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  last = json(response.body);
  if (last.positions[TITLE]) {
    throw new Error("the server still has a place in " + TITLE);
  }
  const shown = String(output.shownListening).split(", ").pop();
  const server = duration(last.listening_seconds);
  if (shown !== server) {
    throw new Error(
      "the app shows " + shown + " of listening, the server counts " + server,
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
