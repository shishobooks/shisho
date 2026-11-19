import { describe, expect, it } from "vitest";

import { isLoopbackHost } from "./location";

describe("isLoopbackHost", () => {
  it.each([
    "localhost",
    "LOCALHOST",
    "shisho.localhost",
    "127.0.0.1",
    "127.1.2.3",
    "::1",
    "[::1]",
    "0.0.0.0",
  ])("treats %s as loopback", (host) =>
    expect(isLoopbackHost(host)).toBe(true),
  );

  it.each([
    "192.168.1.20",
    "shisho.example.com",
    "10.0.0.5",
    "127.example.com",
    "[fe80::1]",
  ])("treats %s as reachable", (host) =>
    expect(isLoopbackHost(host)).toBe(false),
  );
});
