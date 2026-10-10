import {
  FileTypeAZW3,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  FileTypeMOBI,
  FileTypePDF,
} from "@/types";

/**
 * The in-app reading action a file type supports, surfaced as a button on the
 * File and Book detail views. Audiobooks (M4B) get a "Listen" action that opens
 * the audio player; ebooks/comics get a "Read" action that opens their reader.
 * Both route through the same `/read` route. Returns null for file types with
 * no in-app reader.
 */
export type ReadingAction = "read" | "listen";

export function getReadingAction(fileType: string): ReadingAction | null {
  switch (fileType) {
    case FileTypeM4B:
      return "listen";
    case FileTypeAZW3:
    case FileTypeCBZ:
    case FileTypeEPUB:
    case FileTypeMOBI:
    case FileTypePDF:
      return "read";
    default:
      return null;
  }
}
