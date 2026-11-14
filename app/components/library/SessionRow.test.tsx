import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import SessionRow from "@/components/library/SessionRow";
import { setAuth } from "@/testing/auth";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const session = {
  seconds: 1800,
  start_percentage: 0.1,
  end_percentage: 0.2,
  kind: "reading",
  source: "web_reader",
  measured: true,
  device_id: null,
  device_name: null,
} as const;

const renderRow = () =>
  render(
    <ul>
      <SessionRow
        onDelete={vi.fn()}
        session={session}
        title="Dune"
        when="8:00 PM to 8:30 PM"
      />
    </ul>,
  );

describe("SessionRow", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));

  it("offers the delete", () => {
    renderRow();
    expect(
      screen.getByRole("button", { name: /^Delete session of Dune/ }),
    ).toBeEnabled();
  });

  it("disables the delete in Demo Mode, like the other reading controls", () => {
    setAuth({ permissions: ["books:read"], demoMode: true });
    renderRow();
    expect(
      screen.getByRole("button", { name: /^Delete session of Dune/ }),
    ).toBeDisabled();
  });
});
