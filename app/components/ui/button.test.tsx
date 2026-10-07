import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Button, buttonVariants } from "./button";

const LAYOUT = ["inline-flex", "rounded-md", "text-sm", "[&_svg]:size-4"];

describe("Button", () => {
  it("defaults to type=button so it never submits a form by accident", () => {
    render(<Button>Go</Button>);
    expect(screen.getByRole("button", { name: "Go" })).toHaveAttribute(
      "type",
      "button",
    );
  });

  it("keeps an explicit type", () => {
    render(<Button type="submit">Save</Button>);
    expect(screen.getByRole("button", { name: "Save" })).toHaveAttribute(
      "type",
      "submit",
    );
  });

  it("does not add type when rendering asChild", () => {
    render(
      <Button asChild>
        <a href="/x">Link</a>
      </Button>,
    );
    expect(screen.getByRole("link", { name: "Link" })).not.toHaveAttribute(
      "type",
    );
  });

  it("renders small icon sizes", () => {
    expect(buttonVariants({ variant: "ghost", size: "icon-sm" })).toContain(
      "h-7 w-7",
    );
    expect(buttonVariants({ variant: "ghost", size: "icon-xs" })).toContain(
      "h-5 w-5",
    );
  });

  it("renders link as an inline text link with no height or padding", () => {
    const classes = buttonVariants({ variant: "link" }).split(" ");
    expect(classes).toEqual(
      expect.arrayContaining(["text-primary", "hover:underline", "p-0"]),
    );
    expect(classes).not.toContain("h-9");
    expect(classes).not.toContain("px-4");
  });

  it("keeps an explicit size on link", () => {
    expect(buttonVariants({ variant: "link", size: "sm" })).toContain("h-8");
  });

  it("renders unstyled with only cursor, focus ring, and disabled styles", () => {
    render(
      <Button className="w-full text-left" disabled variant="unstyled">
        Row
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Row" });
    const classes = button.className.split(" ");
    expect(classes).toEqual(
      expect.arrayContaining([
        "cursor-pointer",
        "focus-visible:ring-1",
        "disabled:opacity-50",
        "w-full",
        "text-left",
      ]),
    );
    for (const cls of [...LAYOUT, "h-9", "px-4", "font-medium"]) {
      expect(classes).not.toContain(cls);
    }
    expect(button).toHaveAttribute("type", "button");
    expect(button).toBeDisabled();
  });
});
