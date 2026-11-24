// The server keeps FLOW as the Page flow for the Book titled TITLE, chosen
// in the app, so the web reads it in that flow too.
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
  if (reading.page_flow !== FLOW) {
    throw new Error(
      "the Book's flow is " +
        JSON.stringify(reading.page_flow) +
        ", not " +
        FLOW,
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
