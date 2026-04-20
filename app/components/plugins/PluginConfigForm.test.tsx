import { PluginConfigForm } from "./PluginConfigForm";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockSaveConfig = vi.fn();
const mockSaveFields = vi.fn();
const mockToastError = vi.fn();

// Per-test override: when set, the corresponding mutateAsync rejects.
let saveConfigError: Error | null = null;
let saveFieldsError: Error | null = null;

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}));

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginConfig: () => ({
    data: {
      schema: {
        apiKey: {
          type: "string",
          label: "API Key",
          description: "",
          required: true,
          secret: false,
        },
        maxResults: {
          type: "number",
          label: "Max Results",
          description: "",
          required: false,
          secret: false,
          min: 1,
          max: 100,
        },
      },
      values: { apiKey: "", maxResults: 10 },
      // Declare a field so the field-settings save path fires.
      declaredFields: ["title"],
      fieldSettings: { title: true },
      confidence_threshold: null,
    },
    isLoading: false,
    dataUpdatedAt: 1,
  }),
  useSavePluginConfig: () => ({
    mutateAsync: (args: unknown) => {
      mockSaveConfig(args);
      return saveConfigError
        ? Promise.reject(saveConfigError)
        : Promise.resolve();
    },
    isPending: false,
  }),
  useSavePluginFieldSettings: () => ({
    mutateAsync: (args: unknown) => {
      mockSaveFields(args);
      return saveFieldsError
        ? Promise.reject(saveFieldsError)
        : Promise.resolve();
    },
    isPending: false,
  }),
}));

const wrap = (ui: React.ReactNode) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {ui}
  </QueryClientProvider>
);

describe("PluginConfigForm", () => {
  beforeEach(() => {
    mockSaveConfig.mockClear();
    mockSaveFields.mockClear();
    mockToastError.mockClear();
    saveConfigError = null;
    saveFieldsError = null;
  });

  it("renders the declared schema fields", () => {
    render(wrap(<PluginConfigForm canWrite={true} id="test" scope="shisho" />));
    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/max results/i)).toBeInTheDocument();
  });

  it("calls save with updated values when the user clicks Save", async () => {
    render(wrap(<PluginConfigForm canWrite={true} id="test" scope="shisho" />));
    fireEvent.change(screen.getByLabelText(/api key/i), {
      target: { value: "sk-123" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => {
      expect(mockSaveConfig).toHaveBeenCalledWith(
        expect.objectContaining({
          scope: "shisho",
          id: "test",
          config: expect.objectContaining({ apiKey: "sk-123" }),
        }),
      );
    });
  });

  it("hides Save button and disables inputs when canWrite is false", () => {
    render(
      wrap(<PluginConfigForm canWrite={false} id="test" scope="shisho" />),
    );
    expect(
      screen.queryByRole("button", { name: /save/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText(/api key/i)).toBeDisabled();
  });

  // Regression test: when the field-settings save fails we must NOT reset
  // initialValues — otherwise hasChanges flips back to false and the user
  // can navigate away thinking the save succeeded.
  it("keeps the form dirty if the field-settings save fails", async () => {
    saveFieldsError = new Error("boom");
    const onDirtyChange = vi.fn();

    render(
      wrap(
        <PluginConfigForm
          canWrite={true}
          id="test"
          onDirtyChange={onDirtyChange}
          scope="shisho"
        />,
      ),
    );

    // Toggle the declared-field switch so the field-settings save fires.
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() => {
      expect(onDirtyChange).toHaveBeenCalledWith(true);
    });

    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    // Both mutations should have been attempted.
    await waitFor(() => {
      expect(mockSaveConfig).toHaveBeenCalled();
      expect(mockSaveFields).toHaveBeenCalled();
    });

    // Toast surfaces the failure.
    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        expect.stringContaining("boom"),
      );
    });

    // After the failed save, the latest dirty state must still be true.
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);
  });
});
