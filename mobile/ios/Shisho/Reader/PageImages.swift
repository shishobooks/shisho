import CoreGraphics
import Foundation
import ImageIO

/// A Page-based File's pages as images, for the image pager (ADR 0025).
/// A page is decoded at the size asked for, off the main thread, and never
/// kept here: PageImageCache keeps what's shown and near it.
protocol PageImageSource: Sendable {
    var pageCount: Int { get }
    /// Each page's shape, in pixels for a CBZ and points for a PDF, as it's
    /// shown (rotation applied). A page that can't be read is portrait.
    func pageSizes() async -> [CGSize]
    /// The page at most maxPixel on its longer side, or nil when it can't be
    /// read. A page is never drawn larger than it is, nor past
    /// largestPixel.
    func image(page: Int, maxPixel: Int) async -> CGImage?
}

enum PageImages {
    /// The longest side a page is ever decoded at, zoomed in or not: past
    /// this a scan's every pixel would cost more memory than it shows.
    static let largestPixel = 4096
    /// A page's shape when it can't be read: portrait, as most are.
    static let unknownSize = CGSize(width: 2, height: 3)

    /// The size an image is when drawn upright: EXIF orientations 5 to 8
    /// turn it on its side.
    static func upright(width: Int, height: Int, orientation: Int) -> CGSize {
        orientation >= 5 && orientation <= 8 ? CGSize(width: height, height: width) : CGSize(width: width, height: height)
    }
}

// MARK: - CBZ

/// A CBZ's pages: the archive entries Shisho named, in Shisho's order
/// (ADR 0025), so a page number is the same page everywhere. The app never
/// sorts or filters them itself.
struct CBZPages: PageImageSource {
    enum Failure: Error, Equatable {
        /// The server couldn't read the CBZ's pages, so it sent none.
        case noPages
    }

    private let archive: ZipArchive
    /// Each page's entry, or nil where the archive has no entry by the name
    /// the server sent.
    private let entries: [ZipArchive.Entry?]

    var pageCount: Int { entries.count }

    init(url: URL, pageNames: [String]) throws {
        try self.init(archive: ZipArchive(url: url), pageNames: pageNames)
    }

    init(archive: ZipArchive, pageNames: [String]) throws {
        guard !pageNames.isEmpty else { throw Failure.noPages }
        self.archive = archive
        entries = pageNames.map { archive.entries[$0] }
    }

    /// The bytes an image's header is read from: most formats put the size
    /// in the first few kilobytes.
    private static let headerBytes = 64 * 1024

    func pageSizes() async -> [CGSize] {
        entries.map { entry in
            guard let entry else { return PageImages.unknownSize }
            if let prefix = try? archive.prefix(of: entry, limit: Self.headerBytes), let size = Self.size(from: prefix, final: false) {
                return size
            }
            // A header further in, as after a large embedded thumbnail.
            if let bytes = try? archive.bytes(of: entry), let size = Self.size(from: bytes, final: true) { return size }
            return PageImages.unknownSize
        }
    }

    private static func size(from data: Data, final: Bool) -> CGSize? {
        let source = CGImageSourceCreateIncremental([kCGImageSourceShouldCache: false] as CFDictionary)
        CGImageSourceUpdateData(source, data as CFData, final)
        guard let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
              let width = properties[kCGImagePropertyPixelWidth] as? Int,
              let height = properties[kCGImagePropertyPixelHeight] as? Int, width > 0, height > 0
        else { return nil }
        let orientation = properties[kCGImagePropertyOrientation] as? Int ?? 1
        return PageImages.upright(width: width, height: height, orientation: orientation)
    }

    func image(page: Int, maxPixel: Int) async -> CGImage? {
        guard entries.indices.contains(page), let entry = entries[page], !Task.isCancelled,
              let bytes = try? archive.bytes(of: entry),
              let source = CGImageSourceCreateWithData(bytes as CFData, [kCGImageSourceShouldCache: false] as CFDictionary)
        else { return nil }
        // Downsampled as it's decoded, so the full-size pixels never exist.
        let options: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceShouldCacheImmediately: true,
            kCGImageSourceThumbnailMaxPixelSize: max(1, min(maxPixel, PageImages.largestPixel)),
        ]
        return CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary)
    }
}

// MARK: - PDF

/// A PDF's pages, each drawn to an image on the device from the downloaded
/// file (ADR 0025), one at a time.
///
/// PDFKit's viewer grew to gigabytes on a large scanned PDF and kept them
/// through memory warnings. Core Graphics keeps what it decoded for a page
/// with the document, so the document is opened afresh every few pages,
/// which lets that go and keeps memory flat however far the reader pages.
actor PDFPages: PageImageSource {
    enum Failure: Error, Equatable {
        case unreadable
        /// Opening it needs a password, which the reader can't ask for.
        case locked
    }

    nonisolated let pageCount: Int
    private let url: URL
    private var document: CGPDFDocument
    private var drawnSinceOpening = 0
    /// Pages drawn before the document is opened again.
    static let pagesPerOpening = 8

    init(url: URL) throws {
        guard let document = CGPDFDocument(url as CFURL) else { throw Failure.unreadable }
        guard document.isUnlocked else { throw Failure.locked }
        self.url = url
        self.document = document
        pageCount = document.numberOfPages
    }

    func pageSizes() async -> [CGSize] {
        (0..<pageCount).map { index in
            guard let page = document.page(at: index + 1) else { return PageImages.unknownSize }
            return Self.uprightSize(page)
        }
    }

    /// The page's crop box in points, turned as it's shown.
    private static func uprightSize(_ page: CGPDFPage) -> CGSize {
        let box = page.getBoxRect(.cropBox)
        let turned = abs(page.rotationAngle) % 180 == 90
        let size = turned ? CGSize(width: box.height, height: box.width) : box.size
        return size.width > 0 && size.height > 0 ? size : PageImages.unknownSize
    }

    func image(page index: Int, maxPixel: Int) async -> CGImage? {
        // A page the reader has since moved away from isn't drawn.
        guard index >= 0, index < pageCount, !Task.isCancelled else { return nil }
        drawnSinceOpening += 1
        if drawnSinceOpening > Self.pagesPerOpening, let fresh = CGPDFDocument(url as CFURL) {
            document = fresh
            drawnSinceOpening = 1
        }
        guard let page = document.page(at: index + 1) else { return nil }
        return autoreleasepool { Self.draw(page, maxPixel: maxPixel) }
    }

    private static func draw(_ page: CGPDFPage, maxPixel: Int) -> CGImage? {
        let size = uprightSize(page)
        let scale = CGFloat(max(1, min(maxPixel, PageImages.largestPixel))) / max(size.width, size.height)
        let width = max(1, Int((size.width * scale).rounded()))
        let height = max(1, Int((size.height * scale).rounded()))
        guard let space = CGColorSpace(name: CGColorSpace.sRGB),
              let context = CGContext(data: nil, width: width, height: height, bitsPerComponent: 8, bytesPerRow: 0, space: space,
                                      bitmapInfo: CGImageAlphaInfo.noneSkipFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue)
        else { return nil }
        // Paper is white: a page that draws no background shows on it.
        context.setFillColor(CGColor(gray: 1, alpha: 1))
        context.fill(CGRect(x: 0, y: 0, width: width, height: height))
        context.interpolationQuality = .high
        context.scaleBy(x: CGFloat(width) / size.width, y: CGFloat(height) / size.height)
        // The transform turns the page and moves its box to the origin; it
        // never scales up, which the scale above does.
        context.concatenate(page.getDrawingTransform(.cropBox, rect: CGRect(origin: .zero, size: size), rotate: 0,
                                                     preserveAspectRatio: true))
        context.clip(to: page.getBoxRect(.cropBox))
        context.drawPDFPage(page)
        return context.makeImage()
    }
}

// MARK: - Layout

/// How a Page-based File's pages lie on the screen.
enum PageLayout {
    /// A page at least this many times taller than it is wide is a strip
    /// from a comic made for vertical scrolling, as the web reader judges
    /// (app/hooks/useDetectedPageFlow.ts).
    static let tallPageRatio: CGFloat = 2.5

    /// The flow a Book opens in until the user picks one, from its pages'
    /// shapes as the web reader picks it: scrolled when the two pages after
    /// the cover, often printed size even in a webtoon, or the only page,
    /// are a tall strip.
    static func detectedFlow(sizes: [CGSize]) -> PageFlow {
        let probed = sizes.count == 1 ? [0] : [1, 2].filter { $0 < sizes.count }
        let tall = probed.contains { sizes[$0].width > 0 && sizes[$0].height / sizes[$0].width >= tallPageRatio }
        return tall ? .scrolled : .paginated
    }

    /// The screens the pages are shown on, each one or two pages in reading
    /// order. In landscape (twoUp), pages pair into spreads, except the
    /// cover, a wide page (two printed pages scanned as one), and a page
    /// whose partner would be wide or missing, which stand alone.
    static func spreads(sizes: [CGSize], twoUp: Bool) -> [[Int]] {
        guard !sizes.isEmpty else { return [] }
        guard twoUp else { return sizes.indices.map { [$0] } }
        let wide = { (page: Int) in sizes[page].width > sizes[page].height }
        var spreads: [[Int]] = [[0]]
        var page = 1
        while page < sizes.count {
            if wide(page) || page == sizes.count - 1 || wide(page + 1) {
                spreads.append([page])
                page += 1
            } else {
                spreads.append([page, page + 1])
                page += 2
            }
        }
        return spreads
    }

    /// The spread showing page.
    static func spread(of page: Int, in spreads: [[Int]]) -> Int {
        spreads.firstIndex { $0.contains(page) } ?? 0
    }

    /// The page a spread records as the reader's place: its first, or the
    /// File's last when it shows it, so reaching the last spread is
    /// reaching the end.
    static func place(of spread: [Int], pageCount: Int) -> Int {
        spread.contains(pageCount - 1) ? pageCount - 1 : spread.first ?? 0
    }

    /// The frames of a spread's pages, left to right on screen, at one
    /// height and fitted in box.
    static func frames(sizes: [CGSize], in box: CGSize) -> [CGRect] {
        guard !sizes.isEmpty, box.width > 0, box.height > 0 else { return [] }
        let widths = sizes.map { $0.height > 0 ? $0.width / $0.height : 1 }
        let total = widths.reduce(0, +)
        let height = min(box.height, box.width / max(total, 0.0001))
        var x: CGFloat = (box.width - total * height) / 2
        let y = (box.height - height) / 2
        return widths.map { width in
            defer { x += width * height }
            return CGRect(x: x, y: y, width: width * height, height: height)
        }
    }

    /// How far through the File page is, as the web reader counts it: the
    /// first page is 0 and the last 1.
    static func fraction(of page: Int, pageCount: Int) -> Double {
        pageCount > 1 ? Double(page) / Double(pageCount - 1) : 0
    }

    /// The page fraction of the way through lands on.
    static func page(at fraction: Double, pageCount: Int) -> Int {
        guard pageCount > 1, fraction.isFinite else { return 0 }
        return min(pageCount - 1, max(0, Int((fraction * Double(pageCount - 1)).rounded())))
    }
}

/// Paginated, a page or spread a screen, turned sideways; or scrolled, the
/// pages in one column, as a webtoon is read.
enum PageFlow: String, Codable, CaseIterable, Sendable {
    case paginated, scrolled
}
