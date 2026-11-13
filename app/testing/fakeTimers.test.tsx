import { render, screen } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

// vitest.setup.ts fakes the timers for every test and clears them after each
// one. Whatever one test leaves on the clock must not reach the next, in any
// order the tests run.
const ShowsLater = ({ text }: { text: string }) => {
  const [shown, setShown] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setShown(text), 0);
    return () => clearTimeout(timer);
  }, [text]);
  return <p>{shown}</p>;
};

describe("the fake timers every test shares", () => {
  // React queues its non-urgent renders on its scheduler, which never queues
  // again once its pending callback is cleared. A test that ended with a
  // render queued would stall every such render in the tests after it, such
  // as a navigation from a debounced search.
  it("can end a test with a render still queued", () => {
    render(<ShowsLater text="first" />);
    // Fires the timer outside act, so React queues the render on its
    // scheduler, and the test ends before it runs.
    vi.advanceTimersByTime(0);
  });

  it("still run renders queued in the test after it", async () => {
    render(<ShowsLater text="second" />);
    expect(await screen.findByText("second")).toBeInTheDocument();
  });

  it("can set the date a test runs on", () => {
    vi.setSystemTime(new Date("2001-02-03T04:05:06Z"));
  });

  it("are back on today's date in the test after it", () => {
    expect(Math.abs(Date.now() - vi.getRealSystemTime())).toBeLessThan(1000);
  });
});
