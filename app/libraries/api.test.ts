import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API, markErrorDisplayed, ShishoAPIError } from "./api";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    getToasts: vi.fn(() => []),
  },
}));

const demoModeResponse = () =>
  Response.json(
    {
      error: {
        code: "demo_mode",
        message: "This action is unavailable in the demo.",
        status_code: 403,
      },
    },
    { status: 403 },
  );

const visibleDemoToast = {
  id: 1,
  title: "This action is unavailable in the demo.",
  type: "error" as const,
};

describe("ShishoAPI Demo Mode errors", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(toast.getToasts).mockReturnValue([]);
  });
  it("shows the Demo Mode toast once and still rejects with the API error", async () => {
    const error = await API.checkStatus(demoModeResponse()).catch(
      (caught) => caught,
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      code: "demo_mode",
      message: "This action is unavailable in the demo.",
      status: 403,
    });
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith(
      "This action is unavailable in the demo.",
      { id: "demo-mode" },
    );
  });

  it("does not add a fallback when the caller already showed the message", async () => {
    vi.mocked(toast.getToasts).mockReturnValue([visibleDemoToast]);

    await API.checkStatus(demoModeResponse()).catch(() => undefined);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).not.toHaveBeenCalled();
  });

  it("does not add a fallback when the caller showed the message with a prefix", async () => {
    vi.mocked(toast.getToasts).mockReturnValue([
      {
        ...visibleDemoToast,
        title: "Failed to save: This action is unavailable in the demo.",
      },
    ]);

    await API.checkStatus(demoModeResponse()).catch(() => undefined);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).not.toHaveBeenCalled();
  });

  it("does not add a fallback when the caller displays the error inline", async () => {
    await API.checkStatus(demoModeResponse()).catch(markErrorDisplayed);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).not.toHaveBeenCalled();
  });

  it("coalesces concurrent Demo Mode fallback toasts", async () => {
    vi.mocked(toast.getToasts).mockImplementation(() =>
      vi.mocked(toast.error).mock.calls.length > 0 ? [visibleDemoToast] : [],
    );

    await Promise.all([
      API.checkStatus(demoModeResponse()).catch(() => undefined),
      API.checkStatus(demoModeResponse()).catch(() => undefined),
    ]);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).toHaveBeenCalledTimes(1);
  });
});
