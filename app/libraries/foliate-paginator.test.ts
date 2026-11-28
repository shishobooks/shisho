import { describe, expect, it } from "vitest";

import {
  clearLeadingBreaks,
  Paginator,
} from "@/libraries/foliate/paginator.js";

// useLocationHistory calls settled() only where it exists, since the
// fixed-layout renderer has none, so losing the patch would fail nothing
// else: history moves made during a page turn would just be dropped again.
describe("the paginator's settled() patch", () => {
  it("is there", () => {
    expect(typeof Paginator.prototype.settled).toBe("function");
  });
});

// The iOS reader (mobile/ios/Shisho/Reader/ReaderWeb/reader.js) counts the
// pages to the next chapter with pageOf() where it exists, and falls back to
// an estimate where it doesn't, so losing the patch would fail nothing else.
describe("the paginator's pageOf() patch", () => {
  it("is there", () => {
    expect(typeof Paginator.prototype.pageOf).toBe("function");
  });
});

// A chapter's file often opens with `page-break-before: always` on its first
// element, which WebKit honors in the paginator's columns by leaving the
// section's first page blank. Every section starts on a page of its own, so
// breaks before its first content are cleared; later ones stay.
describe("the paginator's leading-break patch", () => {
  const breakBefore = (el: Element | null) =>
    (el as HTMLElement | null)?.style.getPropertyValue("break-before");

  it("clears forced breaks before the section's first content", () => {
    const doc = new DOMParser().parseFromString(
      `<html><body>
        <a id="top"></a>
        <div class="chapter" style="page-break-before: always">
          <a id="page8"></a>
          <div class="head" style="break-before: page"><h2>2</h2></div>
          <p>Text</p>
          <div class="later" style="break-before: page">Later</div>
        </div>
      </body></html>`,
      "text/html",
    );
    clearLeadingBreaks(doc);
    expect(breakBefore(doc.querySelector(".chapter"))).toBe("auto");
    expect(breakBefore(doc.querySelector(".head"))).toBe("auto");
    expect(breakBefore(doc.querySelector(".later"))).toBe("page");
  });

  it("looks past text that isn't drawn", () => {
    const doc = new DOMParser().parseFromString(
      `<html><body><span style="display: none">page 8</span>
        <h2 class="head" style="break-before: page">Two</h2></body></html>`,
      "text/html",
    );
    clearLeadingBreaks(doc);
    expect(breakBefore(doc.querySelector(".head"))).toBe("auto");
  });

  it("walks on past a wrapper whose only text isn't drawn", () => {
    const doc = new DOMParser().parseFromString(
      `<html><body><div class="wrap"><span hidden>page 8</span></div>
        <h2 class="head" style="break-before: page">Two</h2></body></html>`,
      "text/html",
    );
    clearLeadingBreaks(doc);
    expect(breakBefore(doc.querySelector(".head"))).toBe("auto");
  });

  it("leaves a break after content alone", () => {
    const doc = new DOMParser().parseFromString(
      `<html><body>Intro<div class="chapter" style="break-before: page">Two</div></body></html>`,
      "text/html",
    );
    clearLeadingBreaks(doc);
    expect(breakBefore(doc.querySelector(".chapter"))).toBe("page");
  });

  it("leaves a break after an image alone", () => {
    const doc = new DOMParser().parseFromString(
      `<html><body><p><img src="a.png"></p><div class="chapter" style="break-before: page">Two</div></body></html>`,
      "text/html",
    );
    clearLeadingBreaks(doc);
    expect(breakBefore(doc.querySelector(".chapter"))).toBe("page");
  });
});
