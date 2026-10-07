import { ESLint, type Linter } from "eslint";
import { describe, expect, it } from "vitest";

// The permission, query, URL, mutate and UI rules in eslint.config.js are
// selectors, so a typo in one would silently match nothing, and the local
// mutateAsync rule in eslint-rules/ reads syntax in ways a refactor could
// quietly change. This lints fixtures as if they sat in the app to prove each
// rule still fires where it should.

const eslint = new ESLint();

const FIXTURE = `
import { useQueryClient } from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";

export const useFixture = () => {
  const queryClient = useQueryClient();
  void queryClient.fetchQuery({ queryKey: ["x"], queryFn: () => 1 });
  void queryClient.prefetchQuery({ queryKey: ["y"], queryFn: () => 1 });
  void queryClient.ensureQueryData({ queryKey: ["z"], queryFn: () => 1 });
  const { hasPermission } = useAuth() as unknown as {
    hasPermission: (resource: string, operation: string) => boolean;
  };
  const resource = String(Math.random());
  return hasPermission("books", "read") || hasPermission(resource, "write");
};
`;

const restrictedSyntax = async (filePath: string, fixture = FIXTURE) => {
  const [result] = await eslint.lintText(fixture, { filePath });
  return result.messages
    .filter((message) => message.ruleId === "no-restricted-syntax")
    .map((message) => message.message);
};

const isQueryMessage = (message: string) => message.includes("query hook");
const isPermissionMessage = (message: string) => message.includes("useCan");

describe("ESLint permission rules", () => {
  it("rejects imperative queries and literal permission checks in components", async () => {
    const messages = await restrictedSyntax("app/components/Fixture.tsx");

    expect(messages.filter(isQueryMessage)).toHaveLength(3);
    // Both the all-literal call and the literal operation after a variable.
    expect(messages.filter(isPermissionMessage)).toHaveLength(2);
  });

  it("allows imperative queries in query hooks but still rejects literal permission checks", async () => {
    const messages = await restrictedSyntax("app/hooks/queries/fixture.ts");

    expect(messages.filter(isQueryMessage)).toHaveLength(0);
    expect(messages.filter(isPermissionMessage)).toHaveLength(2);
  });

  it("allows both in tests", async () => {
    expect(await restrictedSyntax("app/components/Fixture.test.tsx")).toEqual(
      [],
    );
  });
});

// Every hand-built cover, download, stream and page URL, plus the ones that
// are fine: API.request paths, which carry no /api prefix.
const URL_FIXTURE = `
const id = Number(Math.random());
const token = String(Math.random());
export const urls = [
  \`/api/books/\${id}/cover?v=1\`,
  \`/api/series/\${id}/cover\`,
  \`/api/books/files/\${id}/cover?v=\${id}\`,
  \`/api/books/files/\${id}/download\`,
  \`/api/books/files/\${id}/download/kepub\`,
  \`/api/books/files/\${id}/stream\`,
  \`/api/jobs/\${id}/download\`,
  \`/api/share/\${token}/files/\${id}/download\`,
  "/api/books/1/cover",
  "/api/books/" + id + "/cover?v=" + id,
  \`/api/books/files/\${id}/page/\${id}\`,
  \`/books/files/\${id}/cover-page\`,
  \`/books/\${id}/cover\`,
  "/books/" + id + "/cover",
  \`/api/books/files/\${id}/cover-page\`,
  \`/api/books?page=\${id}\`,
];
`;

const isUrlMessage = (message: string) => message.includes("app/utils");

describe("ESLint URL rule", () => {
  it("rejects literal cover, download, stream and page URLs outside app/utils", async () => {
    const messages = await restrictedSyntax(
      "app/components/Fixture.tsx",
      URL_FIXTURE,
    );

    expect(messages.filter(isUrlMessage)).toHaveLength(11);
  });

  it("rejects them in query hooks too", async () => {
    const messages = await restrictedSyntax(
      "app/hooks/queries/fixture.ts",
      URL_FIXTURE,
    );

    expect(messages.filter(isUrlMessage)).toHaveLength(11);
  });

  it("allows them in app/utils, where the helpers live", async () => {
    const messages = await restrictedSyntax(
      "app/utils/fixture.ts",
      URL_FIXTURE,
    );

    expect(messages.filter(isUrlMessage)).toHaveLength(0);
  });

  it("still bans raw query hooks in app/utils", async () => {
    const [result] = await eslint.lintText(
      'import { useQuery } from "@tanstack/react-query";\nexport const q = useQuery;\n',
      { filePath: "app/utils/fixture.ts" },
    );

    expect(
      result.messages.filter(
        (message) => message.ruleId === "no-restricted-imports",
      ),
    ).toHaveLength(1);
  });
});

const MUTATE_FIXTURE = `
declare const mutation: {
  mutate: (vars: unknown, options?: Record<string, unknown>) => void;
};
export const run = () => {
  mutation.mutate(1);
  mutation.mutate(2, { onSuccess: () => undefined });
  mutation.mutate(3, { onError: () => undefined });
  mutation.mutate(4, { onSuccess: () => undefined, onError: () => undefined });
  // The onError belongs to the nested call, so the outer one is still silent.
  mutation.mutate(5, {
    onSuccess: () => mutation.mutate(6, { onError: () => undefined }),
  });
  // The rule cannot see into options held in a variable.
  const options = { onError: () => undefined };
  mutation.mutate(7, options);
  // A destructured mutate is not a member call, so it is banned outright.
  const { mutate } = mutation; mutate(8);
  // An explicit undefined is no handler.
  mutation.mutate(9, { onError: undefined });
};
// An uncalled mutate passes the click event as its variables and no options.
export const Button = () => <button onClick={mutation.mutate} type="button" />; // (10)
`;

const isMutateMessage = (message: string) => message.includes("onError");

// The number of each fixture call a filter's messages flag: the first
// integer opening a call's arguments, like (5, or (5), on the line each
// message points at.
const flaggedCalls = async (
  fixture: string,
  filePath: string,
  keep: (message: Linter.LintMessage) => boolean,
) => {
  const [result] = await eslint.lintText(fixture, { filePath });
  const lines = fixture.split("\n");
  return result.messages
    .filter(keep)
    .map((message) => Number(/\((\d+)[,)]/.exec(lines[message.line - 1])?.[1]));
};

describe("ESLint mutation rule", () => {
  it("rejects mutate calls that do not handle a rejection", async () => {
    // The outer 5, the destructured mutate (8), and the uncalled
    // onClick={mutation.mutate} (10) included.
    expect(
      await flaggedCalls(
        MUTATE_FIXTURE,
        "app/components/Fixture.tsx",
        (message) =>
          message.ruleId === "no-restricted-syntax" &&
          isMutateMessage(message.message),
      ),
    ).toEqual([1, 2, 5, 7, 8, 9, 10]);
  });

  it("allows them in tests", async () => {
    const messages = await restrictedSyntax(
      "app/components/Fixture.test.tsx",
      MUTATE_FIXTURE,
    );

    expect(messages.filter(isMutateMessage)).toHaveLength(0);
  });
});

// Each mutateAsync call carries its number as the first parenthesized
// integer on its line, which is how the test below names the flagged ones.
const MUTATE_ASYNC_FIXTURE = `
import { useCallback } from "react";

declare const mutation: { mutateAsync: (vars: unknown) => Promise<unknown> };
declare const report: (error: unknown) => void;
export const run = async () => {
  await mutation.mutateAsync(1);
  void mutation.mutateAsync(2);
  try {
    await mutation.mutateAsync(3);
  } catch (error) {
    report(error);
    // A retry inside the catch clause is not covered by the try.
    await mutation.mutateAsync(4);
  } finally {
    await mutation.mutateAsync(5);
  }
  await mutation.mutateAsync(6).catch(report);
  await mutation
    .mutateAsync(7)
    .then(() => undefined)
    .catch(report);
  // A .then chain without a .catch still drops the rejection.
  await mutation.mutateAsync(8).then(() => undefined);
  // A try with only a finally clause does not catch.
  try {
    await mutation.mutateAsync(10);
  } finally {
    report(null);
  }
  // A concise arrow passed to a call hands its promise to that call, which
  // nothing here catches.
  await Promise.all([1].map(() => mutation.mutateAsync(11)));
  // A try does not cover a callback defined in it that runs later.
  try {
    [1].forEach(async () => {
      await mutation.mutateAsync(12);
    });
  } catch (error) {
    report(error);
  }
  // A destructured, renamed, aliased, or computed mutateAsync hides the call.
  const { mutateAsync } = mutation; await mutateAsync(13);
  const { mutateAsync: save } = mutation; await save(20);
  const alias = mutation.mutateAsync; await alias(21);
  await mutation["mutateAsync"](22);
  // An empty catch swallows the failure, and a catch that only logs and
  // rethrows reports nothing.
  try {
    await mutation.mutateAsync(23);
  } catch {
    // ignored
  }
  try {
    await mutation.mutateAsync(24);
  } catch (error) {
    console.error(error);
    throw error;
  }
  // The second .then argument handles the rejection.
  await mutation.mutateAsync(25).then(() => undefined, report);
  // So does a try around a concise arrow passed to a call.
  try {
    await Promise.all([1].map(() => mutation.mutateAsync(26)));
  } catch (error) {
    report(error);
  }
  // A try catches only a promise it awaits.
  try {
    void mutation.mutateAsync(30);
  } catch (error) {
    report(error);
  }
};
// Returning the promise hands the rejection to the caller.
export const handOff = (id: number) => {
  return mutation.mutateAsync(id);
};
export const handOffArrow = (id: number) => mutation.mutateAsync(id);
// A component callback prop such as onSave may catch what it is handed, so
// a handler passed only there is a hand-off too.
const handleSave = async () => {
  return mutation.mutateAsync(27);
};
// Known gaps, pinned so a change to them is deliberate: a handler that
// reaches an event prop only through a wrapper or a factory.
const handleWrapped = async () => {
  return mutation.mutateAsync(28);
};
const makeHandler = () => () => mutation.mutateAsync(33);
// A try does not catch a callback that runs later, since it awaits nothing.
export const later = () => {
  try {
    setTimeout(() => mutation.mutateAsync(29), 0);
  } catch (error) {
    report(error);
  }
};
// An event handler never catches what it is handed, whether it is inline,
// held in a variable, or wrapped in useCallback.
const handleClick = () => mutation.mutateAsync(15);
const handleSubmit = async () => {
  return mutation.mutateAsync(17);
};
export const Button = () => {
  const handleSelect = useCallback(() => mutation.mutateAsync(16), []);
  return (
    <div>
      <button onClick={() => mutation.mutateAsync(9)} type="button" />
      <button onClick={() => { return mutation.mutateAsync(14); }} type="button" />
      <button onClick={handleClick} type="button" />
      <button onClick={handleSelect} type="button" />
      <form onSubmit={handleSubmit} />
      <form action={() => mutation.mutateAsync(18)} />
      <Dialog onSave={handleSave} />
      <button onClick={() => void handleWrapped()} type="button" />
      <button onClick={makeHandler()} type="button" />
      <button onClick={async () => { try { return mutation.mutateAsync(31); } catch (error) { report(error); } }} type="button" />
      <button onClick={flag ? () => mutation.mutateAsync(32) : undefined} type="button" />
    </div>
  );
};
declare const Dialog: (props: { onSave: () => Promise<unknown> }) => null;
declare const flag: boolean;
`;

const isMutateAsyncMessage = (message: string) =>
  message.includes("mutateAsync");

// A message from the mutateAsync checks: the local rule or the reference
// bans in no-restricted-syntax.
const isMutateAsyncCheck = (message: Linter.LintMessage) =>
  message.ruleId === "shisho/mutate-async-handled" ||
  (message.ruleId === "no-restricted-syntax" &&
    isMutateAsyncMessage(message.message));

describe("ESLint mutateAsync rule", () => {
  it("rejects mutateAsync calls whose rejection nothing handles", async () => {
    // In fixture line order. 3, 6, 7, 25, and 26 are caught; the returned
    // calls in handOff, handOffArrow, and handleSave (27) are hand-offs; the
    // known gaps (28, 33) are not flagged.
    expect(
      await flaggedCalls(
        MUTATE_ASYNC_FIXTURE,
        "app/components/Fixture.tsx",
        isMutateAsyncCheck,
      ),
    ).toEqual([
      1, 2, 4, 5, 8, 10, 11, 12, 13, 20, 21, 22, 23, 24, 30, 29, 15, 17, 16, 9,
      14, 18, 31, 32,
    ]);
  });

  it("allows them in tests", async () => {
    expect(
      await flaggedCalls(
        MUTATE_ASYNC_FIXTURE,
        "app/components/Fixture.test.tsx",
        isMutateAsyncCheck,
      ),
    ).toEqual([]);
  });
});

// Each UI convention rule flags its line; the lines after them are the
// accepted forms and must stay clean.
const UI_FIXTURE = `
import { Tabs } from "@/components/ui/tabs";
import { cn } from "@/libraries/utils";

declare const error: { status: number };
declare const c: string;
export const Fixture = ({ n }: { n: number }) => {
  void navigator.clipboard.writeText("x");
  const { clipboard } = navigator;
  const a = error.status === 404;
  const b = 404 !== error.status;
  const t = \`\${n} minutes ago\`;
  const forbidden = error.status === 403;
  return (
    <div className={\`p-2 \${c}\`}>
      <span>{n} days ago</span>
      <Tabs defaultValue="x" />
      <Tabs onValueChange={() => undefined} value="x" />
      <span className={cn("p-2", c)}>{String(a && b && forbidden) + t}</span>
      <span>{String(clipboard)} Chicago</span>
    </div>
  );
};
`;

const uiMessages = async (filePath: string) => {
  const [result] = await eslint.lintText(UI_FIXTURE, { filePath });
  return result.messages
    .filter(
      (message) =>
        message.ruleId === "no-restricted-syntax" ||
        message.ruleId === "no-restricted-properties",
    )
    .map((message) => message.message);
};

describe("ESLint UI convention rules", () => {
  it("rejects each convention once per violation", async () => {
    const messages = await uiMessages("app/components/Fixture.tsx");
    const count = (needle: string) =>
      messages.filter((message) => message.includes(needle)).length;

    expect(count("copyText")).toBe(2);
    expect(count("isNotFoundError")).toBe(2);
    expect(count("formatDistanceToNow")).toBe(2);
    expect(count("cn()")).toBe(1);
    expect(count("Deep-link tabs")).toBe(1);
    expect(messages).toHaveLength(8);
  });

  it("allows them in tests", async () => {
    expect(await uiMessages("app/components/Fixture.test.tsx")).toEqual([]);
  });

  it("lets copyText use the async clipboard", async () => {
    const [result] = await eslint.lintText(
      'export const copy = () => navigator.clipboard.writeText("x");\n',
      { filePath: "app/utils/clipboard.ts" },
    );

    expect(
      result.messages.filter(
        (message) => message.ruleId === "no-restricted-properties",
      ),
    ).toEqual([]);
  });
});
