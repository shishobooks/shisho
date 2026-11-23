// Notes where listening offline should reach: at least 12 s past the place
// the player paused at online (output.onlinePaused), as a pattern for the
// position label that any later place matches, so a stall can't skip past it.

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
for (let s = seconds(output.onlinePaused) + 12; s < 120; s++) {
  places.push(Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0"));
}
output.offlineTarget = ".*(" + places.join("|") + ")";
