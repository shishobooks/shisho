// The server has the reader's place in TITLE as a 0-based page from the
// Shisho app: PAGE, the page the app showed after relaunching, counted from
// 1 as the app shows it.
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
  if (reading.source !== "shisho_app") {
    throw new Error("the place came from " + reading.source);
  }
  if (reading.cfi !== "") {
    throw new Error("the place is a CFI: " + reading.cfi);
  }
  const shown = /Page (\d+) of/.exec(String(PAGE));
  if (!shown) throw new Error("no page in " + PAGE);
  if (reading.page !== Number(shown[1]) - 1) {
    throw new Error(
      "the server's place is page " +
        reading.page +
        " (0-based), the app showed page " +
        shown[1],
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
