import { act, cleanup, render, screen } from "@testing-library/react";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it } from "vitest";

import { API } from "./api";

afterEach(() => {
  toast.dismiss();
  cleanup();
});

const demoModeResponse = () =>
  Response.json(
    {
      error: {
        code: "demo_mode",
        message: "This action is unavailable in the demo.",
      },
    },
    { status: 403 },
  );

describe("Demo Mode toast rendering", () => {
  it("renders one toast for concurrent rejections", async () => {
    render(<Toaster />);

    await act(async () => {
      await Promise.all([
        API.checkStatus(demoModeResponse()).catch(() => undefined),
        API.checkStatus(demoModeResponse()).catch(() => undefined),
        API.checkStatus(demoModeResponse()).catch(() => undefined),
      ]);
    });

    expect(
      await screen.findByText("This action is unavailable in the demo."),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText("This action is unavailable in the demo."),
    ).toHaveLength(1);
  });
});
