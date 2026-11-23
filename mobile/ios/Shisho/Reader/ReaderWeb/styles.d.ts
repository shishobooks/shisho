// For the web app's unit test of styles.js.
export interface PageFontFace {
  src: string;
  format: string;
  weight: string;
  style: string;
}
export interface PageStyleSettings {
  theme: "light" | "sepia" | "dark" | "black";
  layout: "paginated" | "scrolled";
  pages: "auto" | "one";
  fg: string;
  bg: string;
  link: string | null;
  size: number;
  forceFontSize: boolean;
  lineSpacing: number;
  margins: number;
  justify: boolean;
  font: {
    family: string;
    fallback: string;
    faces: PageFontFace[];
  } | null;
}
export declare const fontFaces: (
  family: string,
  faces: PageFontFace[],
) => string;
export declare const bookCSS: (s: PageStyleSettings) => string;
export declare const bookLayout: (s: PageStyleSettings) => {
  flow: string;
  gap: string;
  maxColumnCount: string;
};
export declare const recordBookAlignment: (doc: Document) => void;
