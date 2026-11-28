// Restarts the Shisho server through the harness's control endpoint,
// accepting only the app protocols in RANGE, such as 2-2 to refuse the app
// as too old or 0-0 to refuse it as too new for the server. go_online.js
// puts the server's own range back.
const response = http.post(
  CONTROL + "/app-protocols?range=" + encodeURIComponent(RANGE),
  { body: "" },
);
if (response.status !== 204) {
  throw new Error(
    "the harness could not change the app protocols the server accepts: " +
      response.status,
  );
}
