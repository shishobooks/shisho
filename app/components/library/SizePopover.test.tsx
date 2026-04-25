import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SizeButton, SizePopover } from "@/components/library/SizePopover";

describe("SizePopover", () => {
  const renderPopover = (
    overrides: Partial<{
      effectiveSize: "s" | "m" | "l" | "xl";
      savedSize: "s" | "m" | "l" | "xl";
      onChange: ReturnType<typeof vi.fn>;
      onSaveAsDefault: ReturnType<typeof vi.fn>;
      isSaving: boolean;
    }> = {},
  ) => {
    const onChange = overrides.onChange ?? vi.fn();
    const onSaveAsDefault = overrides.onSaveAsDefault ?? vi.fn();
    render(
      <SizePopover
        effectiveSize={overrides.effectiveSize ?? "m"}
        savedSize={overrides.savedSize ?? "m"}
        isSaving={overrides.isSaving ?? false}
        onChange={onChange}
        onSaveAsDefault={onSaveAsDefault}
        trigger={<SizeButton isDirty={false} />}
      />,
    );
    return { onChange, onSaveAsDefault };
  };

  it("opens when the trigger is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPopover();
    await user.click(screen.getByRole("button", { name: /size/i }));
    expect(screen.getByRole("button", { name: "M" })).toBeInTheDocument();
  });

  it("hides the save-as-default card when effective size matches saved", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPopover({ effectiveSize: "m", savedSize: "m" });
    await user.click(screen.getByRole("button", { name: /size/i }));
    expect(
      screen.queryByRole("button", { name: /save as my default everywhere/i }),
    ).not.toBeInTheDocument();
  });

  it("shows the save-as-default card when sizes differ and calls handler", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onSaveAsDefault } = renderPopover({
      effectiveSize: "l",
      savedSize: "m",
    });
    await user.click(screen.getByRole("button", { name: /size/i }));
    const saveBtn = await screen.findByRole("button", {
      name: /save as my default everywhere/i,
    });
    expect(
      screen.getByText("Other users won't be affected."),
    ).toBeInTheDocument();
    await user.click(saveBtn);
    expect(onSaveAsDefault).toHaveBeenCalledTimes(1);
  });
});

describe("SizeButton", () => {
  it("shows a dirty dot when isDirty=true", () => {
    render(<SizeButton isDirty={true} />);
    expect(
      screen.getByLabelText("Size differs from default"),
    ).toBeInTheDocument();
  });

  it("hides the dirty dot when isDirty=false", () => {
    render(<SizeButton isDirty={false} />);
    expect(
      screen.queryByLabelText("Size differs from default"),
    ).not.toBeInTheDocument();
  });
});
