import type { FoliateMOBI } from "@/libraries/foliate/mobi.js";
import type { RangeFile } from "@/libraries/rangeFile";

// What foliate's MOBI reader asks of a file: slice(start, end) and its
// bytes, and nothing else (PDB in mobi.js). A slice reads from the server
// when its bytes are asked for.
interface RangeBlob {
  size: number;
  slice(from?: number, to?: number): RangeBlob;
  arrayBuffer(): Promise<ArrayBuffer>;
}

const rangeBlob = (file: RangeFile, start: number, end: number): RangeBlob => {
  const clamp = (at: number) => Math.min(Math.max(at, 0), end - start);
  return {
    size: end - start,
    slice: (from = 0, to = end - start) =>
      rangeBlob(file, start + clamp(from), start + clamp(to)),
    arrayBuffer: async () => {
      const data = await file.read(start, end - start);
      return data.buffer.slice(
        data.byteOffset,
        data.byteOffset + data.byteLength,
      ) as ArrayBuffer;
    },
  };
};

/**
 * Opens a MOBI or AZW3 a part at a time from the server, for
 * <foliate-view>'s open(): what foliate's makeBook does with a whole file
 * (view.js), reading through a RangeFile instead. A record is a read, so a
 * photo is one request; mobi.js's Shisho patches start a chapter's photos,
 * and the scan of every record at open, at once.
 */
export const openRangeMOBI = async (file: RangeFile): Promise<FoliateMOBI> => {
  const [{ MOBI }, { unzlibSync }] = await Promise.all([
    import("@/libraries/foliate/mobi.js"),
    import("@/libraries/foliate/vendor/fflate.js"),
  ]);
  return new MOBI({ unzlib: unzlibSync }).open(rangeBlob(file, 0, file.size));
};
