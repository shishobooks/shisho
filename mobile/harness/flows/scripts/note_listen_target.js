// Notes where listening on from the place FROM (the player's position label)
// should reach: at least 12 s past it, as a pattern for the position label
// in output.listenTarget that any later place matches, so a stall can't skip
// past it.

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

// Every place from the target to the end of Harbor Lights' first chapter.
const places = [];
for (let s = seconds(FROM) + 12; s < 120; s++) {
  places.push(Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0"));
}
// None left would make a pattern that matches any place.
if (places.length === 0) {
  throw new Error(FROM + " is too near the end of the chapter to listen on");
}
output.listenTarget = ".*(" + places.join("|") + ")";
