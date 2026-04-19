import { PluginConfigForm } from "./PluginConfigForm";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

const mockSaveConfig = vi.fn();
const mockSaveFields = vi.fn();

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
      declaredFields: [],
      fieldSettings: {},
      confidence_threshold: null,
    },
    isLoading: false,
    dataUpdatedAt: 1,
  }),
  useSavePluginConfig: () => ({
    mutate: (
      args: unknown,
      opts?: { onSuccess?: () => void; onError?: (err: Error) => void },
    ) => {
      mockSaveConfig(args);
      opts?.onSuccess?.();
    },
    isPending: false,
  }),
  useSavePluginFieldSettings: () => ({
    mutate: (
      args: unknown,
      opts?: { onSuccess?: () => void; onError?: (err: Error) => void },
    ) => {
      mockSaveFields(args);
      opts?.onSuccess?.();
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
  it("renders the declared schema fields", () => {
    render(wrap(<PluginConfigForm id="test" scope="shisho" />));
    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/max results/i)).toBeInTheDocument();
  });

  it("calls save with updated values when the user clicks Save", async () => {
    render(wrap(<PluginConfigForm id="test" scope="shisho" />));
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
});
