// Takes the Shisho server offline through the harness's control endpoint,
// so the app can't reach it, as in airplane mode.
const response = http.post(CONTROL + "/offline", { body: "" });
if (response.status !== 204) {
  throw new Error(
    "the harness could not take the server offline: " + response.status,
  );
}
