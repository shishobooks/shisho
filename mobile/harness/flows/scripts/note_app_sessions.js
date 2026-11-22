// How many sessions the server has from the Shisho app for TITLE before
// this flow listens, since other flows listen to the same title.
const response = http.get(
  CONTROL + "/reading?title=" + encodeURIComponent(TITLE),
);
if (response.status !== 200) {
  throw new Error("the harness could not read the server: " + response.status);
}
output.appSessionsBefore = json(response.body).sessions.filter(
  (s) => s.source === "shisho_app",
).length;
