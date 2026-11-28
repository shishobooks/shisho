import SwiftUI

/// A CBZ's or PDF's pages as a grid of tiles on its File page, as the web's
/// `FilePageGrid`: two rows, the second fading out above Show all, and each
/// tile opening the reader on its page.
///
/// A downloaded File's pages are drawn on the device (ADR 0025). One that
/// isn't downloaded shows the server's thumbnails, as the web's grid does,
/// while the server can be reached; without it, each tile shows its number.
/// Showing the File page never downloads the File.
struct FilePageGrid: View {
    @Environment(AppModel.self) private var model
    let fileID: Int
    let pageCount: Int
    /// The File's Download, as its page has it: nil until that's known, so
    /// a downloaded File doesn't first fetch from the server, and nil
    /// inside without one. The pages are drawn from it as soon as it
    /// finishes, and again when a replacement does.
    let download: DownloadRow??
    @State private var expanded = false
    @State private var pages: TilePages?
    @State private var width: CGFloat = 0

    /// Where the tiles come from now: changes as the File downloads or the
    /// server comes and goes.
    private var source: TileSource {
        guard let download else { return .none }
        // A replaced File's old copy still opens while the new one
        // downloads; the new one, once there, is another copy to open.
        if let row = download, row.state == .done || row.replacing {
            return .downloaded(copy: "\(row.state)-\(row.replacing)-\(row.sizeBytes)")
        }
        guard model.connection == .online, model.session != nil else { return .none }
        return .server
    }

    private static let tileMinimum: CGFloat = 96
    private static let gap: CGFloat = 12
    private static let collapsedRows = 2

    private var columns: Int {
        max(1, Int((width + Self.gap) / (Self.tileMinimum + Self.gap)))
    }

    private var overflows: Bool { pageCount > columns * Self.collapsedRows }
    private var shown: Int { expanded ? pageCount : min(pageCount, columns * Self.collapsedRows) }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: Self.gap), count: columns), spacing: Self.gap) {
                ForEach(0..<shown, id: \.self) { page in
                    PageTile(page: page, pages: pages) { model.read(fileID: fileID, page: page) }
                }
            }
            // Opening a page downloads the File, which needs the server.
            .modifier(NeedsConnection(available: model.canOpen(fileID: fileID)))
            .onGeometryChange(for: CGFloat.self, of: \.size.width) { width = $0 }
            .mask {
                if overflows && !expanded {
                    LinearGradient(stops: [.init(color: .black, location: 0.6), .init(color: .clear, location: 1)],
                                   startPoint: .top, endPoint: .bottom)
                } else {
                    Color.black
                }
            }
            .accessibilityIdentifier("file.pages")
            if overflows || expanded {
                Button { expanded.toggle() } label: {
                    HStack(spacing: 4) {
                        Text(expanded ? String(localized: "Show fewer") : String(localized: "Show all \(FileFacts.pages(pageCount))"))
                        Icon(expanded ? .chevronUp : .chevronDown, size: 16)
                            .accessibilityHidden(true)
                    }
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(Theme.primary)
                    .frame(minHeight: 44)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("file.pages.toggle")
            }
        }
        .task(id: source) {
            // Server thumbnails already shown stay when the connection goes,
            // rather than turning back into numbers.
            if source == .none, pages?.fromServer == true { return }
            let opened = await openPages(source)
            // A source changed meanwhile has its own task.
            guard !Task.isCancelled else { return }
            pages = opened
        }
    }

    private func openPages(_ source: TileSource) async -> TilePages? {
        guard let session = model.session else { return nil }
        switch source {
        case .none:
            return nil
        case .server:
            return TilePages(.server(session.client, fileID: fileID))
        case .downloaded:
            guard let file = try? await session.store.pool.read({ try CatalogStore.readingFile($0, fileID: fileID) }),
                  let url = session.downloads.localFile(fileID: file.fileID)
            else { return nil }
            let opened = await Task.detached(priority: .utility) { () -> PageImageSource? in
                if file.fileType == "pdf" { return try? PDFPages(url: url) }
                return try? CBZPages(url: url, pageNames: file.pages)
            }.value
            guard let opened, !Task.isCancelled else { return nil }
            return TilePages(.device(PageImageCache(source: opened, limit: TilePages.limit)))
        }
    }
}

private enum TileSource: Hashable {
    case none, server
    case downloaded(copy: String)
}

/// The tiles' pictures: drawn from the downloaded File, or the server's
/// thumbnails. Either is kept while the File's page is open, up to a few
/// megabytes, so a tile scrolled back to doesn't decode or fetch again, and
/// Show all on a long File doesn't hold every page.
@MainActor
private final class TilePages {
    enum Source {
        case device(PageImageCache)
        case server(APIClient, fileID: Int)
    }

    /// Small: thumbnails.
    static let limit = 24 * 1024 * 1024
    /// The server's thumbnail tier for a tile: its longer side is about
    /// 170 points, at three pixels a point.
    private static let thumbnailSize = 512

    private let source: Source
    private let fetched = NSCache<NSNumber, UIImage>()

    init(_ source: Source) {
        self.source = source
        fetched.totalCostLimit = Self.limit
    }

    var fromServer: Bool {
        if case .server = source { return true }
        return false
    }

    func image(page: Int) async -> UIImage? {
        switch source {
        case .device(let cache):
            return await cache.image(page: page, maxPixel: 300, need: .later)
        case .server(let server, let fileID):
            if let image = fetched.object(forKey: page as NSNumber) { return image }
            guard let data = try? await server.pageThumbnail(fileID: fileID, page: page, size: Self.thumbnailSize),
                  let image = UIImage(data: data)
            else { return nil }
            let bytes = image.cgImage.map { $0.bytesPerRow * $0.height } ?? data.count
            fetched.setObject(image, forKey: page as NSNumber, cost: bytes)
            return image
        }
    }
}

/// One page's tile: its thumbnail, or a blank page without one, with its
/// number under it.
private struct PageTile: View {
    let page: Int
    let pages: TilePages?
    let open: () -> Void
    @State private var image: UIImage?
    @ScaledMetric(relativeTo: .caption) private var numberHeight: CGFloat = 16

    var body: some View {
        Button(action: open) {
            VStack(spacing: 4) {
                ZStack {
                    Theme.muted
                    if let image {
                        Image(uiImage: image)
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .background(Color.white)
                    }
                }
                .aspectRatio(2.0 / 3.0, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 4, style: .continuous).strokeBorder(Theme.border))
                Text(page + 1, format: .number)
                    .font(.geist(.caption))
                    .monospacedDigit()
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(minHeight: numberHeight)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .accessibilityLabel(Text("Page \(page + 1)"))
        .accessibilityIdentifier("file.page.\(page)")
        .task(id: pages.map(ObjectIdentifier.init)) {
            // Cancelled when the tile scrolls away, which stops its decode
            // or fetch.
            guard let pages else {
                image = nil
                return
            }
            let shown = await pages.image(page: page)
            guard !Task.isCancelled else { return }
            image = shown
        }
    }
}
