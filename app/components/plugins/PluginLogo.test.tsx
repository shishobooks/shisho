import { PluginLogo } from "./PluginLogo";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

describe("PluginLogo", () => {
  it("renders an <img> when imageUrl is provided", () => {
    render(
      <PluginLogo
        id="google-books"
        imageUrl="https://example/g.png"
        scope="shisho"
        size={40}
      />,
    );
    const img = screen.getByRole("img");
    expect(img).toHaveAttribute("src", "https://example/g.png");
    expect(img).toHaveAttribute("alt", "google-books logo");
  });

  it("falls back to initials when imageUrl is missing", () => {
    render(<PluginLogo id="google-books" scope="shisho" size={40} />);
    expect(screen.getByText("GB")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("swaps to initials when the <img> onError fires", () => {
    render(
      <PluginLogo
        id="google-books"
        imageUrl="https://broken"
        scope="shisho"
        size={40}
      />,
    );
    const img = screen.getByRole("img");
    fireEvent.error(img);
    expect(screen.getByText("GB")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("sizes the container to the size prop", () => {
    const { container } = render(
      <PluginLogo id="google-books" scope="shisho" size={64} />,
    );
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ width: "64px", height: "64px" });
  });
});
