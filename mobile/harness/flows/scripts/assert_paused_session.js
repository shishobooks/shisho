// A pause sends the session so far, so the server has it without waiting
// for the sitting to end: one more than before this flow listened
// (note_app_sessions.js).
const response = http.get(
  CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
);
if (response.status !== 200) {
  throw new Error("the harness could not read the server: " + response.status);
}
const reading = json(response.body);
console.log(JSON.stringify(reading));
const sessions = reading.sessions.filter((s) => s.source === "shisho_app");
if (sessions.length <= output.appSessionsBefore) {
  throw new Error("the server has no session from the pause");
}
