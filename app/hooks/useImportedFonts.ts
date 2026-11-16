import { useCallback, useEffect, useState } from "react";

import {
  importFont,
  listImportedFonts,
  removeImportedFont,
  type ImportedFont,
  type LoadedImportedFont,
} from "@/libraries/readerFonts";

// The fonts imported into this browser for the ebook reader.
export function useImportedFonts() {
  const [fonts, setFonts] = useState<ImportedFont[]>([]);
  // False until the stored fonts are read, so a chosen imported font isn't
  // taken for one that's gone.
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    listImportedFonts()
      .catch((err: unknown) => {
        // Without IndexedDB there are no imported fonts to offer.
        console.warn("Couldn't read imported fonts", err);
        return [];
      })
      .then((stored) => {
        if (cancelled) return;
        setFonts(stored);
        setReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const add = useCallback(async (file: File) => {
    const font = await importFont(file);
    setFonts((current) => [...current, font]);
    return font;
  }, []);

  const remove = useCallback(async (id: number) => {
    await removeImportedFont(id);
    setFonts((current) => current.filter((font) => font.id !== id));
  }, []);

  return { fonts, ready, add, remove };
}

// An address in this page for an imported font's bytes, which the book's
// frames can load, released when the font changes or the reader closes.
export function useImportedFontUrl(
  font: ImportedFont | undefined,
): LoadedImportedFont | undefined {
  const [loaded, setLoaded] = useState<LoadedImportedFont>();
  useEffect(() => {
    if (!font) {
      setLoaded(undefined);
      return;
    }
    const url = URL.createObjectURL(new Blob([font.data]));
    setLoaded({ font, url });
    return () => URL.revokeObjectURL(url);
  }, [font]);
  return loaded?.font === font ? loaded : undefined;
}
