// Type declarations for foliate-js's <foliate-view> custom element.
// See app/libraries/foliate/view.js for the runtime definition.

import type { DetailedHTMLProps, HTMLAttributes } from "react";

interface FoliateTOCItem {
  label: string;
  href: string;
  subitems?: FoliateTOCItem[] | null;
}

interface FoliateRenderer extends HTMLElement {
  // setStyles accepts either a single CSS string (applied as the main stylesheet)
  // or a [beforeStyle, style] tuple for layered rules. See paginator.js:setStyles.
  setStyles(styles: string | [string, string]): void;
}

interface FoliateViewElement extends HTMLElement {
  // open() accepts a URL string, a Blob/File (anything with arrayBuffer()),
  // or a directory-like object with isDirectory=true. See view.js:open / makeBook.
  open(book: Blob | File | string | { isDirectory: true }): Promise<void>;
  close(): void;
  goLeft(): Promise<void> | void;
  goRight(): Promise<void> | void;
  goTo(target: string | number | { fraction: number }): Promise<unknown>;
  goToFraction(fraction: number): Promise<void>;
  next(distance?: number): Promise<void>;
  prev(distance?: number): Promise<void>;
  renderer: FoliateRenderer;
  book?: {
    toc?: FoliateTOCItem[];
    [key: string]: unknown;
  };
}

type FoliateViewJSXProps = DetailedHTMLProps<
  HTMLAttributes<FoliateViewElement>,
  FoliateViewElement
>;

declare global {
  interface HTMLElementTagNameMap {
    "foliate-view": FoliateViewElement;
  }
}

// React 19 with `jsx: react-jsx` looks up intrinsic elements on the JSX
// namespaces exported by `react` and `react/jsx-runtime`. Augment all three
// so the tag type-checks regardless of how tsc resolves the JSX factory.
declare module "react" {
  namespace JSX {
    interface IntrinsicElements {
      "foliate-view": FoliateViewJSXProps;
    }
  }
}

declare module "react/jsx-runtime" {
  namespace JSX {
    interface IntrinsicElements {
      "foliate-view": FoliateViewJSXProps;
    }
  }
}

declare module "react/jsx-dev-runtime" {
  namespace JSX {
    interface IntrinsicElements {
      "foliate-view": FoliateViewJSXProps;
    }
  }
}
