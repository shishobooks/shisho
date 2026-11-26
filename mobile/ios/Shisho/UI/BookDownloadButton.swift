import SwiftUI

/// Downloads a whole Book from the top of its page, under Listen and Read:
/// its Files at once, supplements included, as a quiet outlined control
/// so it doesn't compete with them. It says how much it will download, fills while it
/// downloads and stops it when tapped, and once everything is here offers
/// removing it. A Book with nothing it can download shows nothing.
struct BookDownloadButton: View {
    @Environment(AppModel.self) private var model
    let bookID: Int
    /// The space above it, when it shows.
    var spacing: CGFloat = 10
    @State private var downloads: BookDownloads?
    @State private var confirmingStop = false
    @State private var confirmingRemove = false

    var body: some View {
        VStack(spacing: 0) {
            // Always something, even with nothing to show, so its
            // observation starts: an empty view never appears.
            Color.clear.frame(height: 0)
            if let downloads {
                // Progress is only read while something downloads, so the
                // page isn't redrawn for other Books' transfers.
                let phase = downloads.phase(progress: downloads.isDownloading ? model.session?.downloads.progress ?? [:] : [:])
                if phase != .unavailable {
                    control(phase)
                        .padding(.top, spacing)
                }
            }
        }
        .observing(model.session?.store, id: bookID, into: $downloads) { [bookID] db in
            try CatalogStore.bookDownloads(db, bookID: bookID)
        }
    }

    @ViewBuilder
    private func control(_ phase: BookDownloads.Phase) -> some View {
        switch phase {
        case .unavailable:
            EmptyView()
        case .notDownloaded(let files, let bytes), .partlyDownloaded(let files, let bytes):
            let partly = if case .partlyDownloaded = phase { true } else { false }
            Button { model.session?.downloads.downloadBook(bookID) } label: {
                row(icon: .download, text: Self.downloadTitle(files: files, bytes: bytes, partly: partly))
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityLabel(Self.downloadTitle(files: files, bytes: bytes, partly: partly))
            .accessibilityIdentifier("book.download")
            // Downloading needs the server, as listening and reading do.
            .modifier(NeedsConnection(available: model.connection == .online))
        case .downloading(let fraction):
            Button { confirmingStop = true } label: {
                HStack(spacing: 8) {
                    DownloadRing(fraction: fraction)
                    Text(verbatim: fraction.map { String(localized: "Downloading · \($0.formatted(.percent.precision(.fractionLength(0))))") }
                        ?? String(localized: "Downloading"))
                        .monospacedDigit()
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityLabel(String(localized: "Stop downloading this Book"))
            .accessibilityValue(fraction.map { $0.formatted(.percent.precision(.fractionLength(0))) } ?? String(localized: "Downloading"))
            .accessibilityIdentifier("book.download")
            // On the button, so where iOS shows it as a popover, it points
            // at the button.
            .confirmationDialog(String(localized: "Stop downloading this Book?"), isPresented: $confirmingStop, titleVisibility: .visible) {
                Button("Stop Download", role: .destructive) { model.session?.downloads.cancelBook(bookID) }
                    .accessibilityIdentifier("book.download.stop")
            } message: {
                Text("What's already downloaded stays on this device.")
            }
        case .downloaded(_, let bytes):
            Button { confirmingRemove = true } label: {
                row(icon: .check, text: String(localized: "Downloaded · \(FileFacts.size(bytes))"))
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityLabel(String(localized: "Downloaded, \(FileFacts.size(bytes))"))
            .accessibilityHint(String(localized: "Offers removing the Download"))
            .accessibilityIdentifier("book.download")
            .confirmationDialog(String(localized: "Remove this Book's Downloads from this device?"), isPresented: $confirmingRemove,
                                titleVisibility: .visible) {
                Button("Remove Download", role: .destructive) { model.session?.downloads.removeBook(bookID) }
                    .accessibilityIdentifier("book.download.remove")
            } message: {
                Text("It stays in your library, and you can download it again later.")
            }
        }
    }

    private func row(icon: Lucide, text: String) -> some View {
        HStack(spacing: 8) {
            Icon(icon, size: 15, relativeTo: .footnote)
                .accessibilityHidden(true)
            Text(verbatim: text)
                .monospacedDigit()
        }
        .frame(maxWidth: .infinity)
    }

    /// "Download all · 1.2 GB" for several Files, "Download · 412 MB" for
    /// one, and "Download the rest" once some are here.
    static func downloadTitle(files: Int, bytes: Int, partly: Bool) -> String {
        let size = FileFacts.size(bytes)
        if partly { return String(localized: "Download the rest · \(size)") }
        return files > 1 ? String(localized: "Download all · \(size)") : String(localized: "Download · \(size)")
    }
}

// MARK: - Press-and-hold menus

/// A Book's Download items for its press-and-hold menu, in a grid or on a
/// shelf: Download its Files, Cancel Download while they're on their
/// way, and Remove Download once something is here. downloads is the Book's
/// entry from `CatalogStore.booksDownloads`, nil when it has none. Put it
/// inside a `.contextMenu`, or use `.bookDownloadMenu(book:downloads:)`.
struct BookDownloadMenu: View {
    @Environment(AppModel.self) private var model
    let book: BookSummary
    let downloads: BookDownloads?

    /// A Book with no Downloads at all can download when it has a File of a
    /// type this version knows.
    private var canDownload: Bool {
        guard let downloads else { return book.fileTypes.contains(where: FileTypes.isKnown) }
        return !downloads.missing.isEmpty
    }

    var body: some View {
        let downloading = downloads?.isDownloading == true
        if downloading {
            Button { model.session?.downloads.cancelBook(book.id) } label: {
                Label("Cancel Download", menuIcon: .x)
            }
            .accessibilityIdentifier("book.menu.cancelDownload")
        } else if canDownload {
            Button { model.session?.downloads.downloadBook(book.id) } label: {
                Label("Download", menuIcon: .download)
            }
            .disabled(model.connection != .online)
            .accessibilityIdentifier("book.menu.download")
        }
        if downloads?.hasDownloaded == true {
            Button(role: .destructive) { model.session?.downloads.removeBook(book.id) } label: {
                Label("Remove Download", menuIcon: .trash)
            }
            .accessibilityIdentifier("book.menu.removeDownload")
        }
    }
}

extension View {
    /// Adds a Book's Download items as this view's press-and-hold menu. A
    /// view that has a menu already puts `BookDownloadMenu` in it instead.
    func bookDownloadMenu(book: BookSummary, downloads: BookDownloads?) -> some View {
        contextMenu { BookDownloadMenu(book: book, downloads: downloads) }
    }
}

/// One File's Download items for its press-and-hold menu, as its Download
/// button offers them.
struct FileDownloadMenu: View {
    @Environment(AppModel.self) private var model
    let fileID: Int
    let supported: Bool
    let row: DownloadRow?

    var body: some View {
        let downloads = model.session?.downloads
        switch downloads?.status(row) ?? .notDownloaded {
        case .notDownloaded, .failed:
            if supported {
                Button { downloads?.download([fileID]) } label: {
                    Label("Download", menuIcon: .download)
                }
                .disabled(model.connection != .online)
                .accessibilityIdentifier("file.menu.download.\(fileID)")
            }
        case .waiting, .running:
            Button { downloads?.remove([fileID]) } label: {
                Label("Cancel Download", menuIcon: .x)
            }
            .accessibilityIdentifier("file.menu.cancelDownload.\(fileID)")
        case .downloaded:
            Button(role: .destructive) { downloads?.remove([fileID]) } label: {
                Label("Remove Download", menuIcon: .trash)
            }
            .accessibilityIdentifier("file.menu.removeDownload.\(fileID)")
        }
    }
}

// MARK: - A cell's mark

/// A Book cell's Download mark at its cover's bottom right: a ring filling
/// while any of its Files is on its way, then the downloaded mark.
/// downloaded is what the cell was told; downloads, when given, adds the
/// ring. Only a cell downloading reads the transfers' progress, so the
/// rest of a grid isn't redrawn as bytes arrive.
struct BookDownloadMark: View {
    @Environment(AppModel.self) private var model
    let downloads: BookDownloads?
    var downloaded = false

    var body: some View {
        if let downloads, downloads.isDownloading {
            CoverDownloadProgress(fraction: downloads.fraction(progress: model.session?.downloads.progress ?? [:]))
                .accessibilityIdentifier("book.downloading")
        } else if downloaded || downloads?.hasDownloaded == true {
            CoverDownloadBadge()
                .accessibilityIdentifier("book.downloaded")
        }
    }
}

/// A Book cell's VoiceOver label, with where its Downloads stand after it:
/// "Downloading, 40%", or "Downloaded". A modifier, so a percent changing
/// redraws only the label, not the cell.
struct BookDownloadLabel: ViewModifier {
    @Environment(AppModel.self) private var model
    let label: String
    let downloads: BookDownloads?
    var downloaded = false

    func body(content: Content) -> some View {
        content.accessibilityLabel(text)
    }

    private var text: String {
        let words: String?
        if let downloads, downloads.isDownloading {
            words = downloads.words(progress: model.session?.downloads.progress ?? [:])
        } else if downloaded || downloads?.hasDownloaded == true {
            words = String(localized: "Downloaded")
        } else {
            words = nil
        }
        return [label, words].compactMap(\.self).joined(separator: ", ")
    }
}
