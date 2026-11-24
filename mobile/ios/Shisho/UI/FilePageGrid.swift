import SwiftUI

/// A CBZ's or PDF's pages as a grid of tiles on its File page, as the web's
/// `FilePageGrid`: two rows, the second fading out above Show all, and each
/// tile opening the reader on its page.
///
/// The pages are drawn on the device (ADR 0025), so a tile shows its page
/// once the File is on the phone, from reading it; before that it shows its
/// number. Showing the File page never downloads the File.
struct FilePageGrid: View {
    @Environment(AppModel.self) private var model
    let fileID: Int
    let pageCount: Int
    @State private var expanded = false
    @State private var cache: PageImageCache?
    @State private var width: CGFloat = 0

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
                    PageTile(page: page, cache: cache) { model.read(fileID: fileID, page: page) }
                }
            }
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
        // Again once the reader closes, which may have downloaded the File.
        .task(id: model.reader == nil) {
            guard cache == nil else { return }
            await openPages()
        }
    }

    /// The File's pages, when it's on the device.
    private func openPages() async {
        guard let session = model.session,
              let file = try? await session.store.pool.read({ try CatalogStore.readingFile($0, fileID: fileID) }),
              let url = session.ebooks.kept(fileID: file.fileID, sizeBytes: file.sizeBytes, fileType: file.fileType)
        else { return }
        let pages = await Task.detached(priority: .utility) { () -> PageImageSource? in
            if file.fileType == "pdf" { return try? PDFPages(url: url) }
            return try? CBZPages(url: url, pageNames: file.pages)
        }.value
        guard let pages, !Task.isCancelled else { return }
        // Small: thumbnails, kept while the page is open.
        cache = PageImageCache(source: pages, limit: 24 * 1024 * 1024)
    }
}

/// One page's tile: its thumbnail, or a blank page until the File is on
/// the device, with its number under it.
private struct PageTile: View {
    let page: Int
    let cache: PageImageCache?
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
        .task(id: cache.map(ObjectIdentifier.init)) {
            // Cancelled when the tile scrolls away, which stops its decode.
            guard let cache else { return }
            image = await cache.image(page: page, maxPixel: 300, need: .later)
        }
    }
}
