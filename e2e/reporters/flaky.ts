/**
 * Reports tests that failed and then passed on a retry. CI retries each test
 * once (playwright.config.ts), so such a test passes the job and would go
 * unnoticed: this puts a warning on the test's line in the pull request and
 * lists the tests in the job summary.
 */
import fs from "node:fs";
import path from "node:path";

import type {
  FullConfig,
  Reporter,
  Suite,
  TestCase,
} from "@playwright/test/reporter";

export interface FlakyTest {
  project: string;
  // The test's describe blocks and title, without the project and file.
  title: string;
  // Relative to the repository root, as GitHub annotations need.
  file: string;
  line: number;
  // The first line of the first failed attempt's error.
  error: string;
}

// eslint-disable-next-line no-control-regex
const ansi = /\u001b\[[0-9;]*m/g;

const firstFailure = (test: TestCase) => {
  const failed = test.results.find((result) => result.status !== "passed");
  return (
    failed?.error?.message?.replace(ansi, "").split("\n")[0] ??
    failed?.status ??
    ""
  );
};

export function flakyTests(tests: TestCase[], rootDir: string): FlakyTest[] {
  return tests
    .filter((test) => test.outcome() === "flaky")
    .map((test) => ({
      project: test.parent.project()?.name ?? "",
      title: test.titlePath().slice(3).join(" › "),
      file: path.relative(rootDir, test.location.file),
      line: test.location.line,
      error: firstFailure(test),
    }));
}

// Workflow commands reserve these in messages, and also , and : in
// properties.
const escapeData = (s: string) =>
  s.replace(/%/g, "%25").replace(/\r/g, "%0D").replace(/\n/g, "%0A");
const escapeProperty = (s: string) =>
  escapeData(s).replace(/:/g, "%3A").replace(/,/g, "%2C");

export function annotation(test: FlakyTest): string {
  const properties = [
    `file=${escapeProperty(test.file)}`,
    `line=${test.line}`,
    `title=${escapeProperty(`Flaky test (${test.project})`)}`,
  ].join(",");
  const message = `${test.title} failed, then passed on retry: ${test.error}`;
  return `::warning ${properties}::${escapeData(message)}`;
}

const cell = (s: string) => s.replace(/\|/g, "\\|").replace(/\n/g, " ");

export function summary(tests: FlakyTest[]): string {
  return [
    "### Flaky E2E tests",
    "",
    "These failed, then passed on retry, so they didn't fail the job.",
    "",
    "| Test | Project | First failure |",
    "| --- | --- | --- |",
    ...tests.map(
      (test) =>
        `| \`${test.file}:${test.line}\` ${cell(test.title)} | ${cell(test.project)} | ${cell(test.error)} |`,
    ),
    "",
  ].join("\n");
}

export default class FlakyReporter implements Reporter {
  private suite?: Suite;

  onBegin(_config: FullConfig, suite: Suite) {
    this.suite = suite;
  }

  onEnd() {
    // Playwright runs from the repository root, where the workflow does.
    const flaky = flakyTests(this.suite?.allTests() ?? [], process.cwd());
    if (flaky.length === 0) return;
    for (const test of flaky) console.log(annotation(test));
    const summaryFile = process.env.GITHUB_STEP_SUMMARY;
    if (summaryFile) fs.appendFileSync(summaryFile, summary(flaky));
  }
}
