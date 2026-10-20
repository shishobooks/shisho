import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import { FileTypeCBZ, FileTypeEPUB, FileTypeKePub, FileTypeM4B } from "@/types";

import FileDownloadControl from "./FileDownloadControl";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

type Props = React.ComponentProps<typeof FileDownloadControl>;

const renderControl = (overrides: Partial<Props> = {}) => {
  const props: Props = {
    file: {
      id: 7,
      file_type: FileTypeEPUB,
      reachable_file_types: [FileTypeEPUB, FileTypeKePub],
    },
    bookTitle: "The Book",
    deliveredFileType: FileTypeEPUB,
    isSupplement: false,
    isDownloading: false,
    onDownload: vi.fn(),
    onDownloadAs: vi.fn(),
    onDownloadOriginal: vi.fn(),
    onCancelDownload: vi.fn(),
    ...overrides,
  };
  const result = render(<FileDownloadControl {...props} />);
  return { ...result, props };
};

const callbacks = [
  "onDownload",
  "onDownloadAs",
  "onDownloadOriginal",
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

const openMenu = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole("button", { name: "Download as" }));
  return within(screen.getByRole("menu"))
    .getAllByRole("menuitem")
    .map((item) => item.getAttribute("aria-label"));
};

describe("FileDownloadControl", () => {
  beforeEach(() => {
    setAuth();
  });

  it("downloads the original file for a supplement", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    // An in-flight download must not affect supplements.
    const { props } = renderControl({
      isSupplement: true,
      isDownloading: true,
    });

    expect(screen.getAllByRole("button")).toHaveLength(1);
    await user.click(screen.getByRole("button"));

    expectOnlyCalled(props, "onDownloadOriginal");
  });

  it.each<[string, Props["file"], string[]]>([
    [
      "an EPUB",
      {
        id: 7,
        file_type: FileTypeEPUB,
        reachable_file_types: [FileTypeEPUB, FileTypeKePub],
      },
      ["Download The Book as EPUB", "Download The Book as KePub"],
    ],
    [
      "a CBZ",
      {
        id: 7,
        file_type: FileTypeCBZ,
        reachable_file_types: [FileTypeCBZ, FileTypeKePub, FileTypeEPUB],
      },
      [
        "Download The Book as CBZ",
        "Download The Book as KePub",
        "Download The Book as EPUB",
      ],
    ],
    [
      "an M4B",
      { id: 7, file_type: FileTypeM4B, reachable_file_types: [FileTypeM4B] },
      ["Download The Book as M4B"],
    ],
  ])(
    "lists exactly the File Types %s can become, its own first",
    async (_, file, entries) => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderControl({ file });

      expect(await openMenu(user)).toEqual(entries);
    },
  );

  it("heads the menu with the File's own title and shows only the File Type in each entry", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderControl({
      file: {
        id: 7,
        file_type: FileTypeEPUB,
        name: "Special Edition",
        reachable_file_types: [FileTypeEPUB, FileTypeKePub],
      },
    });

    expect(await openMenu(user)).toEqual([
      "Download Special Edition as EPUB",
      "Download Special Edition as KePub",
    ]);
    const menu = screen.getByRole("menu");
    expect(menu).toHaveTextContent(/^Special Edition/);
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["EPUB", "KePub"]);
  });

  it.each([FileTypeCBZ, FileTypeKePub, FileTypeEPUB])(
    "downloads the %s entry as that File Type",
    async (fileType) => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const { props } = renderControl({
        file: {
          id: 7,
          file_type: FileTypeCBZ,
          reachable_file_types: [FileTypeCBZ, FileTypeKePub, FileTypeEPUB],
        },
      });

      await openMenu(user);
      await user.click(
        screen.getByRole("menuitem", {
          name: new RegExp(` as ${fileType}$`, "i"),
        }),
      );

      expectOnlyCalled(props, "onDownloadAs");
      expect(props.onDownloadAs).toHaveBeenCalledWith(fileType);
    },
  );

  it("downloads from the main button and names its File Type", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { props } = renderControl({ deliveredFileType: FileTypeKePub });

    const button = screen.getByRole("button", { name: "Download" });
    await user.hover(button);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Download as KePub",
    );
    await user.click(button);

    expectOnlyCalled(props, "onDownload");
  });

  it("shows a cancel button while a download is in flight", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { container, props } = renderControl({ isDownloading: true });

    expect(container.querySelector(".animate-spin")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel download" }));

    expectOnlyCalled(props, "onCancelDownload");
  });

  it.each<[string, Partial<Props>]>([
    ["supplement", { isSupplement: true }],
    ["in-flight download", { isDownloading: true }],
    ["default download", {}],
  ])("renders nothing in Demo Mode (%s)", (_, overrides) => {
    setAuth({ demoMode: true });

    const { container } = renderControl(overrides);

    expect(container).toBeEmptyDOMElement();
  });
});
