import {
  FileTypeAZW3,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeKePub,
  FileTypeM4B,
  FileTypeMOBI,
  FileTypePDF,
} from "@/types";

/** The ebook cover category, as opposed to M4B audiobooks. Mirrors
 *  models.EbookFileTypes. Preferred Cover is exclusive within a category. */
const EBOOK_FILE_TYPES: readonly string[] = [
  FileTypeEPUB,
  FileTypeKePub,
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

/** Reflowable ebooks: text that reflows to the screen, as opposed to the
 *  fixed pages of CBZ and PDF. Their chapters come from a nested table of
 *  contents, which Shisho can rename and delete entries of but not add to or
 *  reorder. */
const REFLOWABLE_FILE_TYPES: readonly string[] = [
  FileTypeEPUB,
  FileTypeKePub,
  FileTypeAZW3,
  FileTypeMOBI,
];

export const isReflowableFileType = (fileType: string): boolean =>
  REFLOWABLE_FILE_TYPES.includes(fileType);

export const isMainEligibleFileType = (fileType: string): boolean =>
  MAIN_ELIGIBLE_FILE_TYPES.includes(fileType);

/** Whether chapters can be added to the type: pages and audio positions can
 *  be chosen, while a reflowable ebook's chapters can only be renamed or
 *  removed. */
export const canAddChapters = (fileType: string): boolean =>
  fileType === FileTypeCBZ ||
  fileType === FileTypePDF ||
  fileType === FileTypeM4B;

/** Orders ebook files when none is the Preferred Cover: EPUB, then KePub,
 *  then AZW3, then MOBI, then the other ebook formats, which share a rank so a
 *  stable sort keeps their order. Mirrors models.EbookCoverRank. */
export const ebookCoverRank = (fileType: string): number => {
  switch (fileType) {
    case FileTypeEPUB:
      return 0;
    case FileTypeKePub:
      return 1;
    case FileTypeAZW3:
      return 2;
    case FileTypeMOBI:
      return 3;
    default:
      return 4;
  }
};
