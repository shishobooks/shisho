import {
  FileTypeAZW3,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  FileTypeMOBI,
  FileTypePDF,
} from "@/types";

/** The ebook cover category, as opposed to M4B audiobooks. Mirrors
 *  models.EbookFileTypes. Preferred Cover is exclusive within a category. */
const EBOOK_FILE_TYPES: readonly string[] = [
  FileTypeEPUB,
  FileTypeAZW3,
  FileTypeMOBI,
  FileTypeCBZ,
  FileTypePDF,
];

/** The types Shisho parses itself, each eligible to be a main file. Mirrors
 *  models.BuiltInFileTypes. */
const MAIN_ELIGIBLE_FILE_TYPES: readonly string[] = [
  ...EBOOK_FILE_TYPES,
  FileTypeM4B,
];

export const isEbookFileType = (fileType: string): boolean =>
  EBOOK_FILE_TYPES.includes(fileType);

export const isMainEligibleFileType = (fileType: string): boolean =>
  MAIN_ELIGIBLE_FILE_TYPES.includes(fileType);

/** Orders ebook files when none is the Preferred Cover: EPUB, then AZW3, then
 *  MOBI, then the other ebook formats, which share a rank so a stable sort
 *  keeps their order. Mirrors models.EbookCoverRank. */
export const ebookCoverRank = (fileType: string): number => {
  switch (fileType) {
    case FileTypeEPUB:
      return 0;
    case FileTypeAZW3:
      return 1;
    case FileTypeMOBI:
      return 2;
    default:
      return 3;
  }
};
