// Stubbed: upstream foliate's pdf.js depends on vendored pdfjs files that
// we don't ship. PDFs are rendered by the separate PDFReader component, so
// this path is never exercised at runtime — view.js only reaches it via a
// dynamic import that Rolldown must still be able to resolve at build time.
export const makePDF = async () => {
    throw new Error('PDF reading via foliate is not supported in this app')
}
