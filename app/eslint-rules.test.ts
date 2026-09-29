import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

// The permission and query rules in eslint.config.js are selectors, so a typo
// in one would silently match nothing. This lints fixtures as if they sat in
// the app to prove each rule still fires where it should.

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

const restrictedSyntax = async (filePath: string) => {
  const [result] = await eslint.lintText(FIXTURE, { filePath });
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
