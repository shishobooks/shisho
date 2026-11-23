// A book's package document (its OPF), tidied for a browser's strict XML
// parser and for foliate. The Shisho app reads a File's original bytes; the
// web reader reads the server's generated copy, whose package Shisho wrote
// afresh, so only the app meets these. The server's own parser matches names
// and ignores namespaces, so books with them scan fine.

const DC = "http://purl.org/dc/elements/1.1/";
const OPF = "http://www.idpf.org/2007/opf";
const DC_ELEMENTS = [
  "title",
  "creator",
  "subject",
  "description",
  "publisher",
  "contributor",
  "date",
  "type",
  "format",
  "identifier",
  "source",
  "language",
  "relation",
  "coverage",
  "rights",
];

export const cleanPackage = (text) => {
  let out = text;
  // A byte-order mark or blank line before the declaration is an error.
  const declaration = out.indexOf("<?xml");
  if (declaration > 0) out = out.slice(declaration);
  // Dublin Core 1.0, from the oldest books, as the 1.1 foliate reads.
  out = out.replaceAll("http://purl.org/dc/elements/1.0/", DC);
  // Dublin Core elements written without a prefix, in the package's own
  // namespace, as by a converter that wrote dc="..." for xmlns:dc="...":
  // foliate finds no title or language. Only when none has the prefix.
  const metadata = /<metadata\b[^>]*>([\s\S]*?)<\/metadata>/.exec(out);
  if (metadata && !/<dc:/.test(metadata[1])) {
    const names = new RegExp(`<(/?)(${DC_ELEMENTS.join("|")})(?=[\\s>/])`, "g");
    const open = metadata[0].slice(0, metadata[0].indexOf(">") + 1);
    const inner = metadata[1].replace(names, "<$1dc:$2");
    if (inner !== metadata[1]) {
      out =
        out.slice(0, metadata.index) +
        open +
        inner +
        "</metadata>" +
        out.slice(metadata.index + metadata[0].length);
    }
  }
  // A prefix used but never declared makes the whole document an error.
  const declare = (prefix, uri) => {
    const used = new RegExp(`</?${prefix}:|\\s${prefix}:[\\w-]+\\s*=`);
    const declared = new RegExp(`xmlns:${prefix}\\s*=`);
    if (used.test(out) && !declared.test(out))
      out = out.replace(/<package\b/, `<package xmlns:${prefix}="${uri}"`);
  };
  declare("dc", DC);
  declare("opf", OPF);
  return out;
};
