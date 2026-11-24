import Compression
import UIKit
import XCTest
@testable import Shisho

/// The image pipeline under the page reader (#921): a CBZ's pages read
/// from its archive in the server's order, a PDF's pages drawn on the
/// device, and memory that stays flat while paging through a large scan.
final class PageImagesTests: XCTestCase {
    // MARK: - Making test files

    /// A zip of entries, each stored or deflated.
    static func zip(_ entries: [(name: String, data: Data, deflate: Bool)]) -> Data {
        var body = Data()
        var directory = Data()
        func u16(_ value: Int, into data: inout Data) { withUnsafeBytes(of: UInt16(value).littleEndian) { data.append(contentsOf: $0) } }
        func u32(_ value: Int, into data: inout Data) { withUnsafeBytes(of: UInt32(value).littleEndian) { data.append(contentsOf: $0) } }
        for entry in entries {
            let stored = entry.deflate ? deflated(entry.data) : entry.data
            let method = entry.deflate ? 8 : 0
            let name = Data(entry.name.utf8)
            let offset = body.count
            u32(0x0403_4B50, into: &body)
            u16(20, into: &body); u16(0x0800, into: &body); u16(method, into: &body); u16(0, into: &body); u16(0, into: &body)
            u32(0, into: &body); u32(stored.count, into: &body); u32(entry.data.count, into: &body)
            u16(name.count, into: &body); u16(0, into: &body)
            body.append(name)
            body.append(stored)
            u32(0x0201_4B50, into: &directory)
            u16(20, into: &directory); u16(20, into: &directory); u16(0x0800, into: &directory); u16(method, into: &directory)
            u16(0, into: &directory); u16(0, into: &directory)
            u32(0, into: &directory); u32(stored.count, into: &directory); u32(entry.data.count, into: &directory)
            u16(name.count, into: &directory); u16(0, into: &directory); u16(0, into: &directory)
            u16(0, into: &directory); u16(0, into: &directory); u32(0, into: &directory); u32(offset, into: &directory)
            directory.append(name)
        }
        var end = Data()
        u32(0x0605_4B50, into: &end)
        u16(0, into: &end); u16(0, into: &end); u16(entries.count, into: &end); u16(entries.count, into: &end)
        u32(directory.count, into: &end); u32(body.count, into: &end); u16(0, into: &end)
        return body + directory + end
    }

    private static func deflated(_ data: Data) -> Data {
        var output = Data(count: data.count + 1024)
        let size = output.withUnsafeMutableBytes { out in
            data.withUnsafeBytes { input in
                compression_encode_buffer(out.bindMemory(to: UInt8.self).baseAddress!, out.count,
                                          input.bindMemory(to: UInt8.self).baseAddress!, data.count, nil, COMPRESSION_ZLIB)
            }
        }
        return output.prefix(size)
    }

    /// A PNG of size pixels.
    static func png(width: Int, height: Int, color: UIColor = .red) -> Data {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        return UIGraphicsImageRenderer(size: CGSize(width: width, height: height), format: format).pngData { context in
            color.setFill()
            context.fill(CGRect(x: 0, y: 0, width: width, height: height))
        }
    }

    // MARK: - Zip

    func testStoredAndDeflatedEntriesRead() throws {
        let text = Data(String(repeating: "Shisho ", count: 500).utf8)
        let archive = try ZipArchive(data: Self.zip([("a.txt", text, false), ("dir/b.txt", text, true)]))
        XCTAssertEqual(archive.entries.map(\.name), ["a.txt", "dir/b.txt"])
        let entries = archive.entries(named: ["a.txt", "dir/b.txt"])
        XCTAssertEqual(try archive.bytes(of: XCTUnwrap(entries[0])), text)
        let deflated = try XCTUnwrap(entries[1])
        XCTAssertLessThan(deflated.compressedSize, text.count)
        XCTAssertEqual(try archive.bytes(of: deflated), text)
        XCTAssertEqual(try archive.prefix(of: deflated, limit: 10), text.prefix(10))
    }

    /// A damaged archive throws rather than reading past its end.
    func testADamagedArchiveThrows() throws {
        XCTAssertThrowsError(try ZipArchive(data: Data("not a zip at all, not even close to one".utf8)))
        let whole = Self.zip([("a.png", Self.png(width: 4, height: 4), false)])
        let archive = try ZipArchive(data: whole)
        let entry = try XCTUnwrap(archive.entries.first)
        // The same directory over a file cut short.
        let cut = whole.prefix(20) + whole.suffix(from: whole.count - 22 - 46 - 5)
        XCTAssertThrowsError(try ZipArchive(data: Data(cut)).bytes(of: entry))
    }

    // MARK: - CBZ

    /// Pages come in the order the server sent, never sorted here; a name
    /// the archive lacks is an unreadable page, not a shifted one.
    func testComicPagesFollowTheServersOrder() async throws {
        let archive = try ZipArchive(data: Self.zip([
            ("page2.png", Self.png(width: 20, height: 30), false),
            ("page10.png", Self.png(width: 40, height: 30), true),
            ("ComicInfo.xml", Data("<ComicInfo/>".utf8), true),
        ]))
        let pages = try CBZPages(archive: archive, pageNames: ["page10.png", "page2.png", "missing.png"])
        XCTAssertEqual(pages.pageCount, 3)
        let sizes = await pages.pageSizes()
        XCTAssertEqual(sizes, [CGSize(width: 40, height: 30), CGSize(width: 20, height: 30), PageImages.unknownSize])
        let image = await pages.image(page: 0, maxPixel: 4096)
        XCTAssertEqual(image?.width, 40)
        let none = await pages.image(page: 2, maxPixel: 100)
        XCTAssertNil(none)
    }

    /// A page decodes at the size it's shown, not its own.
    func testComicPagesDecodeAtScreenSize() async throws {
        let archive = try ZipArchive(data: Self.zip([("big.png", Self.png(width: 1000, height: 1500), false)]))
        let pages = try CBZPages(archive: archive, pageNames: ["big.png"])
        let image = await pages.image(page: 0, maxPixel: 300)
        XCTAssertEqual(image?.height, 300)
        XCTAssertEqual(image?.width, 200)
    }

    /// Two entries with one name are two pages, each its own image, as the
    /// server counts them.
    func testPagesSharingANameAreEachTheirOwn() async throws {
        let archive = try ZipArchive(data: Self.zip([
            ("page.png", Self.png(width: 20, height: 30), false),
            ("page.png", Self.png(width: 40, height: 30), false),
        ]))
        let pages = try CBZPages(archive: archive, pageNames: ["page.png", "page.png"])
        let sizes = await pages.pageSizes()
        XCTAssertEqual(sizes, [CGSize(width: 20, height: 30), CGSize(width: 40, height: 30)])
    }

    func testAComicWithNoPagesDoesntOpen() throws {
        let archive = try ZipArchive(data: Self.zip([("a.png", Self.png(width: 4, height: 4), false)]))
        XCTAssertThrowsError(try CBZPages(archive: archive, pageNames: [])) { error in
            XCTAssertEqual(error as? CBZPages.Failure, .noPages)
        }
    }

    // MARK: - PDF

    /// A PDF of pageCount pages, each a different photo-like scan of
    /// width by height pixels, as a scanned book's pages are.
    static func scannedPDF(pageCount: Int, width: Int, height: Int) throws -> URL {
        // Named for everything that shapes it, so a copy left by an earlier
        // version of this generator isn't taken for it.
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("scan-v1-\(pageCount)-\(width)x\(height).pdf")
        if FileManager.default.fileExists(atPath: url.path) { return url }
        let pageRect = CGRect(x: 0, y: 0, width: 612, height: 612 * CGFloat(height) / CGFloat(width))
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = true
        let scanRenderer = UIGraphicsImageRenderer(size: CGSize(width: width, height: height), format: format)
        let data = UIGraphicsPDFRenderer(bounds: pageRect).pdfData { context in
            for index in 0..<pageCount {
                context.beginPage()
                let scan = scanRenderer.jpegData(withCompressionQuality: 0.8) { scan in
                    // Different on every page, so no two share an image.
                    UIColor(white: 0.95, alpha: 1).setFill()
                    scan.fill(CGRect(x: 0, y: 0, width: width, height: height))
                    for line in 0..<120 {
                        UIColor(hue: CGFloat((index * 37 + line) % 100) / 100, saturation: 0.3, brightness: 0.3, alpha: 1).setFill()
                        scan.fill(CGRect(x: 80 + (line * 13 + index * 7) % 200, y: 60 + line * (height - 120) / 120,
                                         width: width - 400 + (line * index) % 200, height: 10))
                    }
                }
                guard let image = UIImage(data: scan) else { continue }
                image.draw(in: pageRect)
            }
        }
        try data.write(to: url, options: .atomic)
        return url
    }

    func testPDFPagesDrawAtTheSizeAsked() async throws {
        let pages = try PDFPages(url: Self.scannedPDF(pageCount: 3, width: 600, height: 800))
        XCTAssertEqual(pages.pageCount, 3)
        let sizes = await pages.pageSizes()
        XCTAssertEqual(sizes.count, 3)
        XCTAssertEqual(sizes[0].height / sizes[0].width, 800.0 / 600.0, accuracy: 0.01)
        let image = await pages.image(page: 2, maxPixel: 400)
        XCTAssertEqual(image?.height, 400)
        XCTAssertEqual(image?.width, 300)
        let past = await pages.image(page: 3, maxPixel: 400)
        XCTAssertNil(past)
    }

    /// Paging through 60 pages of a large scanned PDF at a phone's screen
    /// size keeps the app's memory flat: past the first pages, which fill
    /// the cache, it doesn't grow with the pages read. PDFKit's viewer grew
    /// to gigabytes doing this (ADR 0025).
    @MainActor
    func testPagingThroughALargeScanKeepsMemoryFlat() async throws {
        let url = try Self.scannedPDF(pageCount: 60, width: 2400, height: 3300)
        let cache = PageImageCache(source: try PDFPages(url: url))
        var footprints: [Int] = []
        for page in 0..<60 {
            _ = await cache.image(page: page, maxPixel: 2556, need: .now)
            footprints.append(MemoryFootprint.bytes)
        }
        XCTAssertLessThanOrEqual(cache.cost, cache.limit)
        let settled = footprints[14]
        let last = footprints[59]
        let peak = footprints[14...].max() ?? 0
        let megabyte = 1_048_576
        print("footprint after 15 pages \(settled / megabyte) MB, after 60 \(last / megabyte) MB, peak \(peak / megabyte) MB")
        XCTAssertLessThan(last - settled, 40 * megabyte, "memory grew from \(settled / megabyte) MB to \(last / megabyte) MB")
        XCTAssertLessThan(peak - settled, 60 * megabyte, "memory peaked at \(peak / megabyte) MB from \(settled / megabyte) MB")
    }
}
