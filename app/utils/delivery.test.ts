import { describe, expect, it } from "vitest";

import {
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeKePub,
  FileTypeM4B,
  FileTypePDF,
  type File,
  type Rule,
} from "@/types";

import { defaultDeliveredFileType } from "./delivery";

const cbz: Pick<File, "file_type" | "reachable_file_types"> = {
  file_type: FileTypeCBZ,
  reachable_file_types: [FileTypeCBZ, FileTypeKePub, FileTypeEPUB],
};

describe("defaultDeliveredFileType", () => {
  it("delivers the file as its rule's File Type", () => {
    const rules: Rule[] = [
      { source_file_type: FileTypeCBZ, delivered_file_type: FileTypeEPUB },
    ];
    expect(defaultDeliveredFileType(cbz, rules)).toBe(FileTypeEPUB);
  });

  it("delivers the file as itself under the built-in rules", () => {
    const rules: Rule[] = [
      { source_file_type: FileTypeEPUB, delivered_file_type: FileTypeEPUB },
      { source_file_type: FileTypeCBZ, delivered_file_type: FileTypeCBZ },
    ];
    expect(defaultDeliveredFileType(cbz, rules)).toBe(FileTypeCBZ);
  });

  it("downloads a File Type no rule lists as itself", () => {
    const rules: Rule[] = [
      { source_file_type: FileTypeEPUB, delivered_file_type: FileTypeEPUB },
    ];
    expect(
      defaultDeliveredFileType(
        { file_type: FileTypeM4B, reachable_file_types: [FileTypeM4B] },
        rules,
      ),
    ).toBe(FileTypeM4B);
  });

  it("ignores a rule the file can't reach", () => {
    const rules: Rule[] = [
      { source_file_type: FileTypePDF, delivered_file_type: FileTypeKePub },
    ];
    expect(
      defaultDeliveredFileType(
        { file_type: FileTypePDF, reachable_file_types: [FileTypePDF] },
        rules,
      ),
    ).toBe(FileTypePDF);
  });

  it("delivers the file as itself without rules", () => {
    expect(defaultDeliveredFileType(cbz, undefined)).toBe(FileTypeCBZ);
  });
});
