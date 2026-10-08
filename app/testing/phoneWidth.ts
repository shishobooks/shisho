import { afterEach, beforeEach } from "vitest";

/**
 * Hides what Tailwind's `hidden` utility hides on a phone, for the tests in the
 * calling scope. jsdom loads no Tailwind CSS, so a button whose text sits in a
 * `hidden sm:inline` span still gets that text as its accessible name in tests,
 * while a screen reader on a phone announces the button unnamed. Below `sm`
 * none of the responsive variants apply, so `.hidden` alone is the phone view.
 */
export const emulatePhoneWidth = () => {
  let style: HTMLStyleElement | undefined;
  beforeEach(() => {
    style = document.createElement("style");
    style.textContent = ".hidden { display: none; }";
    document.head.appendChild(style);
  });
  afterEach(() => style?.remove());
};
