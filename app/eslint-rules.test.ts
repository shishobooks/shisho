import { ESLint, type Linter } from "eslint";
import { describe, expect, it } from "vitest";

// The permission, query, URL, mutate, UI, list envelope and e2e import rules
// in eslint.config.js are selectors or path patterns, so a typo in one would
// silently match nothing, and the local mutateAsync rule in eslint-rules/
// reads syntax in ways a refactor could quietly change. This lints fixtures
// as if they sat in the app or e2e/ to prove each rule still fires where it
// should.

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
import { Button } from "@/components/ui/button";
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
      <button type="button">x</button>
      <Button type="button">x</Button>
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
    expect(count("raw <button>")).toBe(1);
    expect(messages).toHaveLength(9);
  });

  it("allows them in tests", async () => {
    expect(await uiMessages("app/components/Fixture.test.tsx")).toEqual([]);
  });

  it("lets the ui kit render raw buttons", async () => {
    const [result] = await eslint.lintText(
      'export const B = () => <button type="button" />;\n',
      { filePath: "app/components/ui/fixture.tsx" },
    );

    expect(result.messages.filter((message) => message.ruleId)).toEqual([]);
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

const playwrightImports = async (filePath: string) => {
  const [result] = await eslint.lintText(
    `import { test } from "@playwright/test";
import { type Locator } from "@playwright/test";
import type { Page } from "@playwright/test";
export { expect } from "@playwright/test";

export const run = (page: Page, locator: Locator) => test(String(page), () => void locator);
`,
    { filePath },
  );
  return result.messages
    .filter(
      (message) =>
        message.ruleId === "@typescript-eslint/no-restricted-imports",
    )
    .map((message) => message.line);
};

describe("ESLint e2e fixture import rule", () => {
  it("rejects value imports and re-exports of @playwright/test in e2e specs", async () => {
    expect(await playwrightImports("e2e/fixture.spec.ts")).toEqual([1, 4]);
  });

  it("allows them in e2e/fixtures.ts", async () => {
    expect(await playwrightImports("e2e/fixtures.ts")).toEqual([]);
  });
});

const resourceListImports = async (filePath: string, source = "@/types") => {
  const [result] = await eslint.lintText(
    `import type { ResourceListResponse } from "${source}";\nexport type Data = ResourceListResponse<number>;\n`,
    { filePath },
  );
  return result.messages.filter(
    (message) => message.ruleId === "no-restricted-imports",
  );
};

describe("ESLint generated list envelope rule", () => {
  it("rejects ResourceListResponse in query hooks", async () => {
    expect(
      await resourceListImports("app/hooks/queries/fixture.ts"),
    ).toHaveLength(1);
  });

  it("rejects it through the explicit index path too", async () => {
    expect(
      await resourceListImports(
        "app/hooks/queries/fixture.ts",
        "@/types/index",
      ),
    ).toHaveLength(1);
  });

  it("allows it in tests", async () => {
    expect(
      await resourceListImports("app/hooks/queries/fixture.test.ts"),
    ).toHaveLength(0);
  });

  it("allows it in the generic list components", async () => {
    expect(
      await resourceListImports("app/components/library/Fixture.tsx"),
    ).toHaveLength(0);
  });
});

// Each control carries its case number in data-case. Cases 1 to 13 and 28
// to 32 have no name at some width; 14 to 27 and 33 are the accepted forms
// and must stay clean; 34 to 41 are the known gaps (41 a false positive).
const CONTROL_NAME_FIXTURE = `
import { Link } from "react-router-dom";

import { BadgeRemoveButton } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/libraries/utils";

declare const X: () => null;
declare const busy: boolean;
declare const label: string;
declare const item: { icon: React.ReactNode; label: string };
declare const props: Record<string, string>;
declare const statusIcon: React.ReactNode;
declare const iconElement: React.ReactNode;
declare const lexicon: string;
declare const renderIcon: () => React.ReactNode;
declare const IconButton: (props: { children: React.ReactNode }) => null;
export const Fixture = () => (
  <div>
    <Button data-case="1" size="icon"><X /></Button>
    <Button data-case="2" title="Delete"><X /></Button>
    <Button data-case="3"><X /><span className="hidden sm:inline">Add</span></Button>
    <Button data-case="4">{busy ? <X /> : <X />}</Button>
    <Button data-case="5">{busy ? <X /> : "Share"}</Button>
    <Link data-case="6" to="/">{item.icon}{busy && item.label}</Link>
    <Button asChild data-case="7"><Link to="/"><X /></Link></Button>
    <Button asChild={busy} data-case="8">{busy ? <a href="/x"><X /></a> : <span><X /></span>}</Button>
    <BadgeRemoveButton data-case="9" onClick={() => undefined} />
    <a data-case="10" href="/x"><X /></a>
    <Button data-case="11"><span aria-hidden="true">x</span></Button>
    <Button data-case="12"><X /><span className={cn("hidden", "sm:inline")}>Add</span></Button>
    <Button aria-label="" data-case="13"><X /></Button>
    <Button aria-label="Close" data-case="14" size="icon"><X /></Button>
    <Button aria-labelledby="heading" data-case="15"><X /></Button>
    <Link aria-hidden data-case="16" tabIndex={-1} to="/"><X /></Link>
    <Button data-case="17" {...props}><X /></Button>
    <Button data-case="18"><X /><span className="sr-only">Install</span></Button>
    <Button data-case="19"><X />Save</Button>
    <Button data-case="20">{busy ? "Saving" : "Save"}</Button>
    <Button data-case="21">{label}</Button>
    <Button asChild data-case="22"><Link aria-label="Settings" to="/"><X /></Link></Button>
    <Button aria-label="Open" asChild={busy} data-case="23">{busy ? <a href="/x"><X /></a> : <span><X /></span>}</Button>
    <Button aria-label="Add" data-case="24"><X /><span className="hidden sm:inline">Add</span></Button>
    <Link data-case="25" to="/"><img alt="Cover" src="/x" /></Link>
    <Link data-case="26" to="/"><span className={cn("overflow-hidden")}>{item.label}</span></Link>
    <BadgeRemoveButton aria-label="Remove tag" data-case="27" onClick={() => undefined} />
    <Button aria-hidden={false} data-case="28"><X /></Button>
    <Button aria-hidden data-case="29"><X /></Button>
    <Button data-case="30"><X /><span className="sm:hidden">Add</span></Button>
    <Button data-case="31"><X /><span className="invisible sm:visible">Add</span></Button>
    <Button data-case="32">{statusIcon}</Button>
    <Button data-case="33">{lexicon}</Button>
    {/* Known gaps, pinned so a change to them is deliberate: a call or map
        is assumed to render text, an aria-hidden expression counts as
        hidden, a wrapper component around Button is not checked, an
        aria-label expression counts even when undefined, a hidden class
        behind an arbitrary variant counts as visible, and a variable not
        named like an icon counts as text. */}
    <Button data-case="34">{renderIcon()}</Button>
    <Button data-case="35">{[1].map(() => <X />)}</Button>
    <Button aria-hidden={busy} data-case="36" tabIndex={-1}><X /></Button>
    <IconButton data-case="37"><X /></IconButton>
    <Button aria-label={undefined} data-case="38"><X /></Button>
    <Button data-case="39"><X /><span className="data-[state=open]:hidden">Add</span></Button>
    <Button data-case="40">{iconElement}</Button>
    {/* A known false positive: complementary && branches always render
        one label, but the rule sees no text in either. */}
    <Button data-case="41">{busy && "Saving"}{!busy && "Save"}</Button>
  </div>
);
`;

const unnamedControls = async (filePath: string) => {
  const [result] = await eslint.lintText(CONTROL_NAME_FIXTURE, { filePath });
  const lines = CONTROL_NAME_FIXTURE.split("\n");
  return result.messages
    .filter((message) => message.ruleId === "shisho/control-has-name")
    .map((message) =>
      Number(/data-case="(\d+)"/.exec(lines[message.line - 1])?.[1]),
    );
};

describe("ESLint control name rule", () => {
  it("rejects buttons and links with no name at some width", async () => {
    expect(await unnamedControls("app/components/Fixture.tsx")).toEqual([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 28, 29, 30, 31, 32, 41,
    ]);
  });

  it("allows them in tests and the ui kit", async () => {
    expect(await unnamedControls("app/components/Fixture.test.tsx")).toEqual(
      [],
    );
    expect(await unnamedControls("app/components/ui/fixture.tsx")).toEqual([]);
  });
});

// Each control carries its case number in data-case. Cases 1 to 16 and 43
// to 46 have no name; 17 to 37 and 47 to 52 are the accepted forms and must
// stay clean; 38 to 42 and 53 are the known gaps.
const FORM_CONTROL_NAME_FIXTURE = `
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Command, CommandInput } from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { SelectTrigger } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

declare const id: string;
declare const label: string;
declare const props: Record<string, string>;
declare const SearchBox: (props: { placeholder: string }) => null;
export const Fixture = () => (
  <div>
    <Input data-case="1" placeholder="Search..." />
    <Textarea data-case="2" />
    <Checkbox data-case="3" />
    <Switch data-case="4" />
    <RadioGroupItem data-case="5" value="a" />
    <SelectTrigger data-case="6"><span>Value</span></SelectTrigger>
    <input data-case="7" type="text" />
    <select data-case="8"><option>One</option></select>
    <textarea data-case="9" />
    <Input data-case="10" id="unlinked" />
    <Slider aria-label="Volume" data-case="11" />
    <Command><CommandInput aria-label="Search" data-case="12" /></Command>
    <Button data-case="13" role="combobox">Pick an author</Button>
    <Input aria-label="" data-case="14" />
    <Input aria-hidden data-case="15" />
    <Input className="hidden sm:block" data-case="16" />
    <Input aria-label="Search" data-case="17" />
    <Input aria-labelledby="heading" data-case="18" />
    <Label>Title <Input data-case="19" /></Label>
    <label><input data-case="20" type="checkbox" /></label>
    <Label htmlFor="linked">Name</Label>
    <Input data-case="21" id="linked" />
    <Input data-case="22" id={id} />
    <Input data-case="23" {...props} />
    <input data-case="24" type="hidden" />
    <Checkbox aria-hidden data-case="25" tabIndex={-1} />
    <input className="hidden" data-case="26" type="file" />
    <div role="menuitemcheckbox"><Checkbox data-case="27" /></div>
    <Slider data-case="28" thumbLabel="Volume" />
    <Command label="Search authors"><CommandInput data-case="29" /></Command>
    <Command label={label}><div><CommandInput data-case="30" /></div></Command>
    <Button aria-label="Author" data-case="31" role="combobox">Pick</Button>
    <Button aria-labelledby="author-label" data-case="32" role="combobox">Pick</Button>
    <SelectTrigger aria-label="Role" data-case="33"><span>Value</span></SelectTrigger>
    <label htmlFor={"also-linked"}>Path</label>
    <select data-case="34" id="also-linked" />
    <Button data-case="35">Not a combobox</Button>
    <RadioGroup data-case="36"><span /></RadioGroup>
    <Switch aria-label="Allow" data-case="37" />
    {/* Known gaps, pinned so a change to them is deliberate: a wrapper
        component around Input is not checked where it is used, a
        CommandInput with no Command in the file is assumed labelled
        there, an aria-label expression counts even when undefined, a
        non-literal id counts even when nothing points at it, and a hidden
        attribute counts as hidden even when false. */}
    <SearchBox data-case="38" placeholder="Search..." />
    <CommandInput data-case="39" />
    <Input aria-label={undefined} data-case="40" />
    <Input data-case="41" id={\`\${id}-field\`} />
    <Input data-case="42" hidden={false} />
    <Label><Button data-case="43" role="combobox">Pick</Button></Label>
    <div role="option"><Checkbox data-case="44" /></div>
    <Input className="hidden data-[state=open]:block" data-case="45" />
    <Input data-case="46" type="submit" />
    <Label htmlFor="combo">Author</Label>
    <Button data-case="47" id="combo" role="combobox">Pick</Button>
    <Input data-case="48" id="later" />
    <Label htmlFor="later">Named by a label after it</Label>
    <Command {...props}><CommandInput data-case="49" /></Command>
    <input data-case="50" type="submit" value="Go" />
    <input data-case="51" type="button" value="Go" />
    <div role="menuitemradio"><RadioGroupItem data-case="52" value="b" /></div>
    {/* Known gap: a literal id inside a map, or in a component rendered more
        than once, is duplicated on the page, and the rule cannot tell. */}
    {[1, 2].map((n) => (
      <div key={n}>
        <Label htmlFor="repeated">Path</Label>
        <Input data-case="53" id="repeated" />
      </div>
    ))}
  </div>
);
`;

const unnamedFormControls = async (filePath: string) => {
  const [result] = await eslint.lintText(FORM_CONTROL_NAME_FIXTURE, {
    filePath,
  });
  const lines = FORM_CONTROL_NAME_FIXTURE.split("\n");
  return result.messages
    .filter((message) => message.ruleId === "shisho/form-control-has-name")
    .map((message) =>
      Number(/data-case="(\d+)"/.exec(lines[message.line - 1])?.[1]),
    );
};

describe("ESLint form control name rule", () => {
  it("rejects form controls named only by a placeholder, value, or nothing", async () => {
    expect(await unnamedFormControls("app/components/Fixture.tsx")).toEqual([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 43, 44, 45, 46,
    ]);
  });

  // A known gap: app/components/ui is not linted, so a composite there (such
  // as MultiSelectCombobox) relies on a required label prop instead.
  it("allows them in tests and the ui kit", async () => {
    expect(
      await unnamedFormControls("app/components/Fixture.test.tsx"),
    ).toEqual([]);
    expect(await unnamedFormControls("app/components/ui/fixture.tsx")).toEqual(
      [],
    );
  });
});

// Each element carries its case number in data-case, and each rule has its
// own range. Radio group names, cases 1 to 14: 1 to 5 have no name; 6 to 12
// are the accepted forms and must stay clean; 13 and 14 are the known gaps.
// aria-invalid messages, cases 15 to 27: 15 to 20 point at no message; 21 to
// 26 are accepted; 27 is the known gap. Error borders, cases 28 to 47: 28 to
// 35 and 45 to 47 show an error only by the border; 36 to 42 are accepted; 43
// and 44 are the known gaps.
const GROUP_STATE_FIXTURE = `
import * as RadioGroupPrimitive from "@radix-ui/react-radio-group";

import { Input } from "@/components/ui/input";
import { RadioGroup } from "@/components/ui/radio-group";
import { SelectTrigger } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/libraries/utils";

declare const error: string | undefined;
declare const label: string;
declare const props: Record<string, string>;
declare const Wrapper: (props: { className: string }) => null;
export const Fixture = () => (
  <div>
    <RadioGroup data-case="1" value="a" />
    <div data-case="2" role="radiogroup" />
    <RadioGroup aria-label="" data-case="3" value="a" />
    <div data-case="4" role={"radiogroup"} />
    <RadioGroup aria-label={undefined} data-case="5" value="a" />
    <RadioGroup aria-label="Action" data-case="6" value="a" />
    <RadioGroup aria-labelledby="heading" data-case="7" value="a" />
    <RadioGroup aria-label={label} data-case="8" value="a" />
    <RadioGroup data-case="9" {...props} />
    <fieldset><legend>Mode</legend><RadioGroup data-case="10" value="a" /></fieldset>
    <div aria-label="Mode" data-case="11" role="radiogroup" />
    <div data-case="12" role="group" />
    {/* Known gaps: a fieldset counts even with no legend, and a Radix
        primitive used directly (a member expression) is not checked. */}
    <fieldset><RadioGroup data-case="13" value="a" /></fieldset>
    <RadioGroupPrimitive.Root data-case="14" value="a" />
    <Input aria-invalid data-case="15" />
    <Input aria-invalid={Boolean(error)} data-case="16" />
    <input aria-invalid="true" data-case="17" />
    <Textarea aria-invalid data-case="18" />
    <Input aria-describedby="" aria-invalid data-case="19" />
    <Input aria-describedby={undefined} aria-invalid data-case="20" />
    <Input aria-describedby={error ? "err" : undefined} aria-invalid={error ? true : undefined} data-case="21" />
    <Input aria-errormessage="err" aria-invalid data-case="22" />
    <Input aria-invalid={false} data-case="23" />
    <Input aria-invalid="false" data-case="24" />
    <Input aria-invalid data-case="25" {...props} />
    <Input aria-invalid={undefined} data-case="26" />
    {/* Known gap: an aria-describedby counts even when it points at nothing
        or at text that is not the error. */}
    <Input aria-describedby="hint" aria-invalid data-case="27" />
    <Input className={cn("w-20", error && "border-red-500")} data-case="28" />
    <Input className={cn("w-20", error ? "border-destructive" : "")} data-case="29" />
    <input className={cn({ "border-destructive/50": error })} data-case="30" />
    <Textarea className={\`w-20 \${error ? "border-red-500" : ""}\`} data-case="31" />
    <SelectTrigger className={cn(error && "dark:border-red-400")} data-case="32" />
    <Input className={cn(error && "!border-red-500")} data-case="33" />
    <Input className={cn(error && "border-red-500!")} data-case="34" />
    <Input className={cn(error && "md:border-red-500")} data-case="35" />
    <Input aria-invalid={error ? true : undefined} aria-describedby="err" className={cn(error && "border-red-500")} data-case="36" />
    <Input className="border-destructive" data-case="37" />
    <Input className={cn("border-destructive", label)} data-case="38" />
    <Input className={cn(error && "focus-visible:border-red-500")} data-case="39" />
    <Input className={cn(error && "aria-invalid:border-destructive")} data-case="40" />
    <Input className={cn(error && "border-red-500")} data-case="41" {...props} />
    <div className={cn(error && "border-destructive")} data-case="42" />
    {/* Known gaps: a wrapper component is not checked, and a ring or text
        color alone is not an error border. */}
    <Wrapper className={cn(error && "border-red-500")} data-case="43" />
    <Input className={cn(error && "ring-red-500 text-destructive")} data-case="44" />
    <Input aria-invalid={undefined} className={cn(error && "border-red-500")} data-case="45" />
    <Input className={cn(error && "border-l-red-500")} data-case="46" />
    <Textarea className={cn(error && "border-x-destructive/50")} data-case="47" />
  </div>
);
`;

const groupStateCases = async (filePath: string, ruleId: string) => {
  const [result] = await eslint.lintText(GROUP_STATE_FIXTURE, { filePath });
  const lines = GROUP_STATE_FIXTURE.split("\n");
  return result.messages
    .filter((message) => message.ruleId === ruleId)
    .map((message) =>
      Number(/data-case="(\d+)"/.exec(lines[message.line - 1])?.[1]),
    );
};

describe("ESLint group and validation state rules", () => {
  it("rejects radio groups with no name", async () => {
    expect(
      await groupStateCases(
        "app/components/Fixture.tsx",
        "shisho/radio-group-has-name",
      ),
    ).toEqual([1, 2, 3, 4, 5]);
  });

  it("rejects aria-invalid with no message to point at", async () => {
    expect(
      await groupStateCases(
        "app/components/Fixture.tsx",
        "shisho/invalid-has-message",
      ),
    ).toEqual([15, 16, 17, 18, 19, 20]);
  });

  it("rejects a conditional error border with no aria-invalid", async () => {
    expect(
      await groupStateCases(
        "app/components/Fixture.tsx",
        "shisho/error-border-marks-invalid",
      ),
    ).toEqual([28, 29, 30, 31, 32, 33, 34, 35, 45, 46, 47]);
  });

  it("allows them in tests and the ui kit", async () => {
    for (const ruleId of [
      "shisho/radio-group-has-name",
      "shisho/invalid-has-message",
      "shisho/error-border-marks-invalid",
    ]) {
      expect(
        await groupStateCases("app/components/Fixture.test.tsx", ruleId),
      ).toEqual([]);
      expect(
        await groupStateCases("app/components/ui/fixture.tsx", ruleId),
      ).toEqual([]);
    }
  });
});
