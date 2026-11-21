import type {
  FullConfig,
  Suite,
  TestCase,
  TestResult,
} from "@playwright/test/reporter";
import { describe, expect, it, vi } from "vitest";

import FlakyReporter, { annotation, flakyTests, summary } from "./flaky";

const result = (status: TestResult["status"], message?: string) =>
  ({ status, error: message ? { message } : undefined }) as TestResult;

const testCase = (
  outcome: ReturnType<TestCase["outcome"]>,
  results: TestResult[],
  title = "resuming › a comic reopens on the page turned to",
) =>
  ({
    outcome: () => outcome,
    titlePath: () => [
      "",
      "chromium",
      "page-reader.spec.ts",
      ...title.split(" › "),
    ],
    location: { file: "/repo/e2e/page-reader.spec.ts", line: 680, column: 5 },
    parent: { project: () => ({ name: "chromium" }) },
    results,
  }) as unknown as TestCase;

describe("flakyTests", () => {
  it("lists only tests that failed and then passed on a retry", () => {
    const flaky = testCase("flaky", [
      result(
        "failed",
        "\u001b[31mError: expect(received).toBe(expected)\u001b[39m\n\nExpected: 6",
      ),
      result("passed"),
    ]);
    const tests = [
      testCase("expected", [result("passed")], "passes"),
      testCase("unexpected", [result("failed"), result("failed")], "fails"),
      testCase("skipped", [result("skipped")], "skipped"),
      flaky,
    ];

    expect(flakyTests(tests, "/repo")).toEqual([
      {
        project: "chromium",
        title: "resuming › a comic reopens on the page turned to",
        file: "e2e/page-reader.spec.ts",
        line: 680,
        error: "Error: expect(received).toBe(expected)",
      },
    ]);
  });

  it("leaves out a test whose retry was skipped rather than passed", () => {
    expect(
      flakyTests(
        [testCase("flaky", [result("failed", "Error: x"), result("skipped")])],
        "/repo",
      ),
    ).toEqual([]);
  });

  it("says a failed attempt timed out when it has no error message", () => {
    const [flaky] = flakyTests(
      [testCase("flaky", [result("timedOut"), result("passed")])],
      "/repo",
    );
    expect(flaky.error).toBe("timedOut");
  });
});

describe("annotation", () => {
  it("is a GitHub warning at the test, escaping what workflow commands reserve", () => {
    expect(
      annotation({
        project: "chromium",
        title: "a, b: 100%",
        file: "e2e/x.spec.ts",
        line: 3,
        error: "Error: one\ntwo",
      }),
    ).toBe(
      "::warning file=e2e/x.spec.ts,line=3,title=Flaky test (chromium)::a, b: 100%25 failed, then passed on retry: Error: one%0Atwo",
    );
  });
});

describe("summary", () => {
  it("is a Markdown table of the flaky tests", () => {
    expect(
      summary([
        {
          project: "firefox",
          title: "Alias workflows › adds | removes",
          file: "e2e/alias.spec.ts",
          line: 68,
          error: "Error: timeout",
        },
      ]),
    ).toBe(
      [
        "### Flaky E2E tests",
        "",
        "These failed, then passed on retry, so they didn't fail the job.",
        "",
        "| Test | Project | First failure |",
        "| --- | --- | --- |",
        "| `e2e/alias.spec.ts:68` Alias workflows › adds \\| removes | firefox | Error: timeout |",
        "",
      ].join("\n"),
    );
  });
});

describe("FlakyReporter", () => {
  // A summary that can't be written mustn't fail a run whose tests passed.
  it("still reports when the job summary can't be written", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const before = process.env.GITHUB_STEP_SUMMARY;
    process.env.GITHUB_STEP_SUMMARY = "/nonexistent/dir/summary.md";
    try {
      const reporter = new FlakyReporter();
      const flaky = testCase("flaky", [
        result("failed", "Error: x"),
        result("passed"),
      ]);
      reporter.onBegin(
        {} as FullConfig,
        {
          allTests: () => [flaky],
        } as unknown as Suite,
      );

      expect(() => reporter.onEnd()).not.toThrow();
      expect(log).toHaveBeenCalledWith(expect.stringMatching(/^::warning /));
    } finally {
      if (before === undefined) delete process.env.GITHUB_STEP_SUMMARY;
      else process.env.GITHUB_STEP_SUMMARY = before;
      log.mockRestore();
    }
  });
});
