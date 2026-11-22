// Gives the second account access to the harness's Library, or takes it
// away when ACCESS is "none", through the harness's control endpoint.
const response = http.post(CONTROL + "/reader-access?access=" + ACCESS, {
  body: "",
});
if (response.status !== 204) {
  throw new Error(
    "the harness could not change the reader's access: " + response.status,
  );
}
