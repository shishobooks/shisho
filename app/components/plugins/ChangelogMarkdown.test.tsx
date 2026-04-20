import { ChangelogMarkdown } from "./ChangelogMarkdown";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

describe("ChangelogMarkdown", () => {
  it("renders h2/h3 headings and paragraphs", () => {
    render(
      <ChangelogMarkdown>
        {"## What's New\n\nAdded thing.\n\n### Details\n\nMore info."}
      </ChangelogMarkdown>,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /what's new/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 3, name: /details/i }),
    ).toBeInTheDocument();
  });

  it("renders lists, inline code, and code blocks", () => {
    render(
      <ChangelogMarkdown>
        {"- item 1\n- item 2\n\nUse `foo()` or:\n\n```js\nconsole.log(1)\n```"}
      </ChangelogMarkdown>,
    );
    expect(screen.getByText("item 1")).toBeInTheDocument();
    expect(screen.getByText("foo()")).toBeInTheDocument();
    expect(screen.getByText("console.log(1)")).toBeInTheDocument();
  });

  it("opens links in a new tab with rel=noopener noreferrer", () => {
    render(
      <ChangelogMarkdown>{"[link](https://example.com)"}</ChangelogMarkdown>,
    );
    const link = screen.getByRole("link", { name: /link/i });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("strips images, iframes, and raw html", () => {
    render(
      <ChangelogMarkdown>
        {
          "![img](x.png)\n\n<iframe src='x'></iframe>\n\n<script>alert(1)</script>"
        }
      </ChangelogMarkdown>,
    );
    expect(document.querySelector("img")).toBeNull();
    expect(document.querySelector("iframe")).toBeNull();
    expect(document.querySelector("script")).toBeNull();
  });
});
