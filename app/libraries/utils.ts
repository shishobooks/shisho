import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Returns true for file types that derive covers from page content (CBZ, PDF).
 * These formats should never have their covers replaced by external sources.
 */
export const isPageBasedFileType = (fileType: string | undefined): boolean =>
  fileType === "cbz" || fileType === "pdf";
