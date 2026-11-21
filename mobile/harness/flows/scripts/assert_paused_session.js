// A pause sends the session so far, so the server has it without waiting
// for the sitting to end.
const response = http.get(
  CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
);
if (response.status !== 200) {
  throw new Error("the harness could not read the server: " + response.status);
}
const reading = json(response.body);
console.log(JSON.stringify(reading));
const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
if (sessions.length === 0) {
  throw new Error("the server has no session from the pause");
}
