import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";
import {
  DownloadFormatAsk,
  DownloadFormatOriginal,
  FileTypeEPUB,
  FileTypeM4B,
} from "@/types";

import FileDownloadControl from "./FileDownloadControl";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

type Props = React.ComponentProps<typeof FileDownloadControl>;

const renderControl = (overrides: Partial<Props> = {}) => {
  const props: Props = {
    file: { id: 7, file_type: FileTypeEPUB },
    libraryDownloadPreference: DownloadFormatOriginal,
    isSupplement: false,
    isDownloading: false,
    onDownload: vi.fn(),
    onDownloadKepub: vi.fn(),
    onDownloadOriginal: vi.fn(),
    onDownloadWithEndpoint: vi.fn(),
    onCancelDownload: vi.fn(),
    ...overrides,
  };
  const result = render(<FileDownloadControl {...props} />);
  return { ...result, props };
};

const callbacks = [
  "onDownload",
  "onDownloadKepub",
  "onDownloadOriginal",
  "onDownloadWithEndpoint",
  "onCancelDownload",
] as const;

const expectOnlyCalled = (props: Props, called: (typeof callbacks)[number]) => {
  for (const name of callbacks) {
    if (name === called) {
      expect(props[name]).toHaveBeenCalledTimes(1);
    } else {
      expect(props[name]).not.toHaveBeenCalled();
    }
  }
};

describe("FileDownloadControl", () => {
  beforeEach(() => {
    vi.mocked(useAuth).mockReturnValue({ demoMode: false } as never);
  });

  it("downloads the original file for a supplement", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    // The Ask preference and an in-flight download must not affect supplements.
    const { props } = renderControl({
      isSupplement: true,
      libraryDownloadPreference: DownloadFormatAsk,
      isDownloading: true,
    });

    await user.click(screen.getByRole("button"));

    expectOnlyCalled(props, "onDownloadOriginal");
  });

  it("shows the format popover for the Ask preference with a KePub-capable file", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { props } = renderControl({
      libraryDownloadPreference: DownloadFormatAsk,
    });

    await user.click(screen.getByRole("button", { name: "Download" }));
    expect(screen.getByText("Download format")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Original" }));
    expectOnlyCalled(props, "onDownloadWithEndpoint");
    expect(props.onDownloadWithEndpoint).toHaveBeenCalledWith(
      "/api/books/files/7/download",
    );
  });

  it("downloads KePub from the format popover", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { props } = renderControl({
      libraryDownloadPreference: DownloadFormatAsk,
    });

    await user.click(screen.getByRole("button", { name: "Download" }));
    await user.click(screen.getByRole("button", { name: /kepub/i }));

    expectOnlyCalled(props, "onDownloadKepub");
  });

  it("uses the format popover's cancel button for the Ask preference while a download is in flight", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { props } = renderControl({
      libraryDownloadPreference: DownloadFormatAsk,
      isDownloading: true,
    });

    // Only the popover's loading state titles its cancel button. The generic
    // in-flight branch labels it with a tooltip instead.
    await user.click(screen.getByTitle("Cancel download"));

    expectOnlyCalled(props, "onCancelDownload");
  });

  it("shows a plain download button for the Ask preference when the file cannot be KePub", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { props } = renderControl({
      file: { id: 7, file_type: FileTypeM4B },
      libraryDownloadPreference: DownloadFormatAsk,
    });

    await user.click(screen.getByRole("button"));

    expect(screen.queryByText("Download format")).not.toBeInTheDocument();
    expectOnlyCalled(props, "onDownload");
  });

  it("shows a cancel button while a download is in flight", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { container, props } = renderControl({ isDownloading: true });

    expect(container.querySelector(".animate-spin")).toBeInTheDocument();
    await user.click(screen.getByRole("button"));

    expectOnlyCalled(props, "onCancelDownload");
  });

  it("downloads with the library preference by default", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { props } = renderControl();

    await user.click(screen.getByRole("button"));

    expectOnlyCalled(props, "onDownload");
  });

  it.each<[string, Partial<Props>]>([
    ["supplement", { isSupplement: true }],
    ["format popover", { libraryDownloadPreference: DownloadFormatAsk }],
    ["in-flight download", { isDownloading: true }],
    ["default download", {}],
  ])("renders nothing in Demo Mode (%s)", (_, overrides) => {
    vi.mocked(useAuth).mockReturnValue({ demoMode: true } as never);

    const { container } = renderControl(overrides);

    expect(container).toBeEmptyDOMElement();
  });
});
