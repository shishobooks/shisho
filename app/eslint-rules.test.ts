import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

// The permission, query, URL and mutation rules in eslint.config.js are
// selectors, so a typo in one would silently match nothing. This lints
// fixtures as if they sat in the app to prove each rule still fires where it
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
};
`;

const isMutateMessage = (message: string) => message.includes("onError");

describe("ESLint mutation rule", () => {
  it("rejects mutate calls that do not handle a rejection", async () => {
    const messages = await restrictedSyntax(
      "app/components/Fixture.tsx",
      MUTATE_FIXTURE,
    );

    // 1, 2, the outer 5, and 7.
    expect(messages.filter(isMutateMessage)).toHaveLength(4);
  });

  it("allows them in tests", async () => {
    const messages = await restrictedSyntax(
      "app/components/Fixture.test.tsx",
      MUTATE_FIXTURE,
    );

    expect(messages.filter(isMutateMessage)).toHaveLength(0);
  });
});
