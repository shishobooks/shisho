// Builds a small KF8 book (an AZW3) for tests of the web reader's MOBI
// path. It writes what foliate's mobi.js reads and nothing more: a header
// record, uncompressed text, a skeleton and a fragment index, image
// records, and an FDST record. pkg/mobi/kf8_write.go writes the whole
// format; see it, and mobi.js, for what each part means.

export interface KF8Section {
  title: string;
  // The body's markup. An image is `<img src="${kf8Image(n)}"/>`.
  body: string;
}

export interface KF8Book {
  title: string;
  sections: KF8Section[];
  // Each image's bytes. foliate takes the type from the reference.
  images?: Uint8Array[];
}

// The reference to the book's nth image (from 0), as a KF8 book writes it.
export const kf8Image = (index: number) =>
  `kindle:embed:${(index + 1).toString(32).toUpperCase().padStart(4, "0")}?mime=image/jpeg`;

const NONE = 0xffffffff;
const TEXT_RECORD_SIZE = 4096;
const INDX_HEADER_LENGTH = 192;

const encoder = new TextEncoder();

const concat = (parts: Uint8Array[]) => {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let at = 0;
  for (const part of parts) {
    out.set(part, at);
    at += part.length;
  }
  return out;
};

const padTo4 = (bytes: Uint8Array) =>
  bytes.length % 4
    ? concat([bytes, new Uint8Array(4 - (bytes.length % 4))])
    : bytes;

// Fixed-size big-endian fields set into a zeroed record.
const record = (length: number, fields: Array<[number, number | string]>) => {
  const out = new Uint8Array(length);
  const view = new DataView(out.buffer);
  for (const [offset, value] of fields) {
    if (typeof value === "string") out.set(encoder.encode(value), offset);
    else view.setUint32(offset, value);
  }
  return out;
};

// A variable-length number as an index entry holds it: seven bits a byte,
// most significant first, the last byte's high bit set (getVarLen).
const varLen = (value: number) => {
  const bytes = [value & 0x7f];
  for (let rest = value >>> 7; rest; rest >>>= 7) bytes.unshift(rest & 0x7f);
  bytes[bytes.length - 1] |= 0x80;
  return bytes;
};

interface IndexEntry {
  name: string;
  values: number[];
}

// An index: its main record, which names the tags each entry has, then one
// record of entries. Every tag here holds one group of values, so the
// control byte sets the lowest bit of each tag's mask.
const index = (
  tags: Array<[tag: number, numValues: number, mask: number]>,
  entries: IndexEntry[],
): Uint8Array[] => {
  const tagx = concat([
    record(12, [
      [0, "TAGX"],
      [4, 12 + 4 * (tags.length + 1)],
      [8, 1],
    ]),
    new Uint8Array([
      ...tags.flatMap(([tag, n, mask]) => [tag, n, mask, 0]),
      0,
      0,
      0,
      1,
    ]),
  ]);
  const main = concat([
    record(INDX_HEADER_LENGTH, [
      [0, "INDX"],
      [4, INDX_HEADER_LENGTH],
      [24, 1],
      [28, 65001],
      [36, entries.length],
    ]),
    tagx,
  ]);

  const control = tags.reduce((bits, [, , mask]) => bits | (mask & -mask), 0);
  const offsets: number[] = [];
  const body: number[] = [];
  for (const { name, values } of entries) {
    offsets.push(INDX_HEADER_LENGTH + body.length);
    const nameBytes = encoder.encode(name);
    body.push(
      nameBytes.length,
      ...nameBytes,
      control,
      ...values.flatMap(varLen),
    );
  }
  const idxtAt = INDX_HEADER_LENGTH + body.length;
  const idxt = new Uint8Array(4 + 2 * offsets.length);
  idxt.set(encoder.encode("IDXT"));
  offsets.forEach((offset, i) =>
    new DataView(idxt.buffer).setUint16(4 + 2 * i, offset),
  );
  const data = concat([
    record(INDX_HEADER_LENGTH, [
      [0, "INDX"],
      [4, INDX_HEADER_LENGTH],
      [8, 1],
      [20, idxtAt],
      [24, entries.length],
    ]),
    new Uint8Array(body),
    idxt,
  ]);
  return [padTo4(main), padTo4(data)];
};

export const kf8Book = ({
  title,
  sections,
  images = [],
}: KF8Book): Uint8Array => {
  // Each section is a skeleton, its body cut out, and that body as its one
  // fragment, inserted back right after <body>.
  const textParts: Uint8Array[] = [];
  const skeletons: IndexEntry[] = [];
  const fragments: IndexEntry[] = [];
  let textLength = 0;
  sections.forEach((section, i) => {
    const head = encoder.encode(
      `<html xmlns="http://www.w3.org/1999/xhtml"><head><title>${section.title}</title></head><body>`,
    );
    const tail = encoder.encode("</body></html>");
    const body = encoder.encode(section.body);
    skeletons.push({
      name: `SKEL${String(i).padStart(10, "0")}`,
      values: [1, textLength, head.length + tail.length],
    });
    fragments.push({
      name: String(textLength + head.length),
      values: [0, i, 0, body.length],
    });
    textParts.push(head, tail, body);
    textLength += head.length + tail.length + body.length;
  });
  const text = concat(textParts);
  const textRecords: Uint8Array[] = [];
  for (let at = 0; at < text.length; at += TEXT_RECORD_SIZE) {
    textRecords.push(text.slice(at, at + TEXT_RECORD_SIZE));
  }

  const skelIndex = index(
    [
      [1, 1, 0x03],
      [6, 2, 0x0c],
    ],
    skeletons,
  );
  const fragIndex = index(
    [
      [2, 1, 0x01],
      [4, 1, 0x02],
      [6, 2, 0x04],
    ],
    fragments,
  );
  const fdst = record(20, [
    [0, "FDST"],
    [4, 12],
    [8, 1],
    [12, 0],
    [16, text.length],
  ]);

  const skelAt = 1 + textRecords.length;
  const fragAt = skelAt + skelIndex.length;
  const resourceStart = fragAt + fragIndex.length;
  const fdstAt = resourceStart + images.length;

  const mobiLength = 264;
  const exth = record(12, [
    [0, "EXTH"],
    [4, 12],
    [8, 0],
  ]);
  const titleBytes = encoder.encode(title);
  const titleOffset = 16 + mobiLength + exth.length;
  const header = padTo4(
    concat([
      record(16 + mobiLength, [
        // PalmDOC: no compression, then the text's length and records.
        [0, 0x00010000],
        [4, text.length],
        [8, (textRecords.length << 16) | TEXT_RECORD_SIZE],
        [16, "MOBI"],
        [20, mobiLength],
        [24, 2],
        [28, 65001],
        [36, 8],
        [84, titleOffset],
        [88, titleBytes.length],
        [108, resourceStart],
        [128, 0x40],
        [192, fdstAt],
        [196, 1],
        [244, NONE],
        [248, fragAt],
        [252, skelAt],
        [260, NONE],
      ]),
      exth,
      titleBytes,
    ]),
  );

  const records = [
    header,
    ...textRecords,
    ...skelIndex,
    ...fragIndex,
    ...images,
    fdst,
    new Uint8Array([0xe9, 0x8e, 0x0d, 0x0a]),
  ];
  // The Palm database header, then where each record starts.
  const listLength = 78 + records.length * 8 + 2;
  const pdb = record(listLength, [[60, "BOOKMOBI"]]);
  const view = new DataView(pdb.buffer);
  pdb.set(encoder.encode(title.slice(0, 31).replace(/\W/g, "_")));
  view.setUint16(76, records.length);
  let offset = listLength;
  records.forEach((bytes, i) => {
    view.setUint32(78 + i * 8, offset);
    view.setUint32(78 + i * 8 + 4, i * 2);
    offset += bytes.length;
  });
  return concat([pdb, ...records]);
};
