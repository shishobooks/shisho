import atkinsonBold from "@/assets/fonts/atkinson-hyperlegible/AtkinsonHyperlegible-Bold.woff2?url";
import atkinsonBoldItalic from "@/assets/fonts/atkinson-hyperlegible/AtkinsonHyperlegible-BoldItalic.woff2?url";
import atkinsonItalic from "@/assets/fonts/atkinson-hyperlegible/AtkinsonHyperlegible-Italic.woff2?url";
import atkinsonRegular from "@/assets/fonts/atkinson-hyperlegible/AtkinsonHyperlegible-Regular.woff2?url";
import libronBold from "@/assets/fonts/libron/Libron-Bold.woff2?url";
import libronBoldItalic from "@/assets/fonts/libron/Libron-BoldItalic.woff2?url";
import libronItalic from "@/assets/fonts/libron/Libron-Italic.woff2?url";
import libronRegular from "@/assets/fonts/libron/Libron-Regular.woff2?url";
import literataItalic from "@/assets/fonts/literata/Literata-Italic-Variable.woff2?url";
import literata from "@/assets/fonts/literata/Literata-Variable.woff2?url";
import notoSansItalic from "@/assets/fonts/noto-sans/NotoSans-Italic-Variable.ttf?url";
import notoSans from "@/assets/fonts/noto-sans/NotoSans-Variable.ttf?url";
import notoSerifItalic from "@/assets/fonts/noto-serif/NotoSerif-Italic-Variable.ttf?url";
import notoSerif from "@/assets/fonts/noto-serif/NotoSerif-Variable.ttf?url";
import openDyslexicBoldItalic from "@/assets/fonts/opendyslexic/OpenDyslexic-Bold-Italic.woff2?url";
import openDyslexicBold from "@/assets/fonts/opendyslexic/OpenDyslexic-Bold.woff2?url";
import openDyslexicItalic from "@/assets/fonts/opendyslexic/OpenDyslexic-Italic.woff2?url";
import openDyslexicRegular from "@/assets/fonts/opendyslexic/OpenDyslexic-Regular.woff2?url";
import {
  IMPORTED_FONT_PREFIX,
  type BundledFontId,
  type ReaderFontFace,
} from "@/libraries/readerSettings";

// Fonts for the ebook reader. The bundled ones are under the SIL Open Font
// License and ship unmodified under their own names, with their licenses
// beside them in app/assets/fonts. A book's frame loads a font's files only
// when it's chosen, since only the chosen font's @font-face rules reach it.

type FontFormat = "woff2" | "truetype" | "opentype";

interface Face {
  src: string;
  format: FontFormat;
  // A range for a variable font.
  weight: string;
  style: "normal" | "italic";
}

export interface BundledFont {
  id: BundledFontId;
  name: string;
  fallback: string;
  faces: Face[];
}

const fourStyles = (
  format: FontFormat,
  [regular, italic, bold, boldItalic]: string[],
): Face[] => [
  { src: regular, format, weight: "normal", style: "normal" },
  { src: italic, format, weight: "normal", style: "italic" },
  { src: bold, format, weight: "bold", style: "normal" },
  { src: boldItalic, format, weight: "bold", style: "italic" },
];

const variable = (
  format: FontFormat,
  weight: string,
  [upright, italic]: string[],
): Face[] => [
  { src: upright, format, weight, style: "normal" },
  { src: italic, format, weight, style: "italic" },
];

export const BUNDLED_FONTS: BundledFont[] = [
  {
    id: "libron",
    name: "Libron",
    fallback: "serif",
    faces: fourStyles("woff2", [
      libronRegular,
      libronItalic,
      libronBold,
      libronBoldItalic,
    ]),
  },
  {
    id: "literata",
    name: "Literata",
    fallback: "serif",
    faces: variable("woff2", "200 900", [literata, literataItalic]),
  },
  {
    id: "atkinson-hyperlegible",
    name: "Atkinson Hyperlegible",
    fallback: "sans-serif",
    faces: fourStyles("woff2", [
      atkinsonRegular,
      atkinsonItalic,
      atkinsonBold,
      atkinsonBoldItalic,
    ]),
  },
  {
    id: "noto-serif",
    name: "Noto Serif",
    fallback: "serif",
    faces: variable("truetype", "100 900", [notoSerif, notoSerifItalic]),
  },
  {
    id: "noto-sans",
    name: "Noto Sans",
    fallback: "sans-serif",
    faces: variable("truetype", "100 900", [notoSans, notoSansItalic]),
  },
  {
    id: "opendyslexic",
    name: "OpenDyslexic",
    fallback: "sans-serif",
    faces: fourStyles("woff2", [
      openDyslexicRegular,
      openDyslexicItalic,
      openDyslexicBold,
      openDyslexicBoldItalic,
    ]),
  },
];

export interface ImportedFont {
  // IndexedDB's key, which grows with each import, so fonts list in the
  // order they were added.
  id: number;
  // The file's name without its extension.
  name: string;
  format: FontFormat;
  data: ArrayBuffer;
}

// A font file the reader refused, with a message for the reader.
export class FontImportError extends Error {
  name = "FontImportError";
}

// An imported font with an address this page made for its bytes.
export interface LoadedImportedFont {
  font: ImportedFont;
  url: string;
}

// A book's section is a blob: document, which can't resolve a path against
// its own address, so every font is declared at an absolute one.
const absolute = (url: string) => new URL(url, document.baseURI).href;

const fontFaces = (family: string, faces: Face[]) =>
  faces
    .map(
      (face) => `@font-face {
        font-family: "${family}";
        src: url("${absolute(face.src)}") format("${face.format}");
        font-weight: ${face.weight};
        font-style: ${face.style};
        font-display: block;
      }`,
    )
    .join("\n");

// The faces to inject for a font setting, or undefined to keep the book's
// own fonts (including when an imported font chosen earlier is gone).
// `imported` is the chosen imported font with its address in this page, if
// it has one yet. `as` declares the faces under another family name, for a
// page that has a font of the same name already.
export const fontFaceFor = (
  fontId: string,
  imported?: LoadedImportedFont,
  as?: string,
): ReaderFontFace | undefined => {
  const bundled = BUNDLED_FONTS.find((f) => f.id === fontId);
  if (bundled) {
    const family = as ?? bundled.name;
    return {
      family,
      fallback: bundled.fallback,
      faces: fontFaces(family, bundled.faces),
    };
  }
  if (!fontId.startsWith(IMPORTED_FONT_PREFIX)) return undefined;
  const id = fontId.slice(IMPORTED_FONT_PREFIX.length);
  const match = imported && String(imported.font.id) === id ? imported : null;
  if (!match) return undefined;
  // Named by id, so a file named like a bundled font can't take its place.
  const family = as ?? `Imported Font ${id}`;
  return {
    family,
    fallback: "serif",
    faces: fontFaces(family, [
      {
        src: match.url,
        format: match.font.format,
        weight: "normal",
        style: "normal",
      },
    ]),
  };
};

const FORMATS: Record<string, FontFormat> = {
  ttf: "truetype",
  otf: "opentype",
  woff2: "woff2",
};

const DB_NAME = "shisho-reader";
const STORE = "fonts";

// Imported fonts stay in this browser's IndexedDB and are never uploaded.
const openDB = () =>
  new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
      request.result.createObjectStore(STORE, {
        keyPath: "id",
        autoIncrement: true,
      });
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });

const withStore = async <T>(
  mode: IDBTransactionMode,
  run: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> => {
  const db = await openDB();
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(STORE, mode);
      const request = run(tx.objectStore(STORE));
      tx.oncomplete = () => resolve(request.result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
};

export const listImportedFonts = (): Promise<ImportedFont[]> =>
  withStore<ImportedFont[]>("readonly", (store) => store.getAll());

// Keeps a font file in this browser, after checking the browser can use it.
export const importFont = async (file: File): Promise<ImportedFont> => {
  const extension = file.name.match(/\.([^.]+)$/)?.[1]?.toLowerCase() ?? "";
  const format = FORMATS[extension];
  if (!format) {
    throw new FontImportError("Choose a TTF, OTF, or WOFF2 font file.");
  }
  const data = await file.arrayBuffer();
  if (typeof FontFace !== "undefined") {
    try {
      await new FontFace("Shisho font check", data).load();
    } catch {
      throw new FontImportError(
        `${file.name} isn't a font this browser can read.`,
      );
    }
  }
  const font = {
    name: file.name.slice(0, -(extension.length + 1)),
    format,
    data,
  };
  const id = await withStore("readwrite", (store) => store.add(font));
  return { ...font, id: id as number };
};

export const removeImportedFont = async (id: number): Promise<void> => {
  await withStore("readwrite", (store) => store.delete(id));
};
