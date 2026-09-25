import { FileTypeCBZ, FileTypeEPUB } from "@/types";

/**
 * Returns true if the file type can be converted to KePub. Only EPUB and CBZ
 * files support KePub conversion.
 */
export const supportsKepub = (fileType: string): boolean => {
  return fileType === FileTypeEPUB || fileType === FileTypeCBZ;
};
