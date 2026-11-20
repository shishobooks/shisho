import SwiftUI

/// A Book's cover from the cover cache, fading in once loaded. A Book with
/// no cover gets one drawn from its title, so the grid never shows a blank.
struct CoverView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.displayScale) private var displayScale
    let book: BookSummary
    let width: CGFloat
    var shadow: CoverShadow = .small
    var onImage: ((UIImage) -> Void)?

    @State private var image: UIImage?
    @State private var loaded = false

    enum CoverShadow {
        case small, large
    }

    /// Square for an audiobook's cover, as the web app's grid decides: by the
    /// Library's setting, and in its fallback modes by which kinds of File
    /// the Book has.
    private var aspectRatio: CGFloat {
        let hasAudiobook = book.fileTypes.contains("m4b")
        let hasEbook = book.fileTypes.contains { $0 != "m4b" && FileTypes.isKnown($0) }
        let square = switch book.coverAspectRatio {
        case "audiobook": true
        case "audiobook_fallback_book": hasAudiobook
        case "book_fallback_audiobook": !hasEbook && hasAudiobook
        default: false
        }
        return square ? 1 : 2.0 / 3.0
    }

    private var size: Int {
        CoverCache.pixelSize(forWidth: width, scale: displayScale)
    }

    var body: some View {
        ZStack {
            if let image {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
                    .transition(.opacity)
            } else if loaded || book.coverVersion.isEmpty {
                DrawnCover(title: book.title, authors: book.authors, width: width)
            } else {
                Theme.muted
            }
        }
        .frame(width: width, height: width / aspectRatio)
        .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
        .shadow(color: .black.opacity(shadow == .large ? 0.22 : 0.08), radius: shadow == .large ? 18 : 3, y: shadow == .large ? 12 : 1)
        .accessibilityHidden(true)
        .task(id: "\(book.id)-\(book.coverVersion)-\(size)") {
            guard let covers = model.session?.covers else { return }
            if let cached = covers.memoryImage(bookID: book.id, version: book.coverVersion, size: size) {
                image = cached
            } else {
                let fetched = await covers.image(bookID: book.id, version: book.coverVersion, size: size)
                withAnimation(.easeOut(duration: 0.25)) {
                    image = fetched
                    loaded = true
                }
            }
            if let image { onImage?(image) }
        }
    }
}

/// A cover for a Book that has none: its title and authors on a quiet
/// tint of the primary color.
private struct DrawnCover: View {
    let title: String
    let authors: String
    let width: CGFloat

    var body: some View {
        ZStack(alignment: .bottomLeading) {
            LinearGradient(colors: [Theme.primary.opacity(0.18), Theme.primary.opacity(0.32)], startPoint: .top, endPoint: .bottom)
                .background(Theme.card)
            Image("LogoMark")
                .resizable()
                .renderingMode(.template)
                .foregroundStyle(Theme.primary.opacity(0.35))
                .frame(width: width * 0.32, height: width * 0.32)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topTrailing)
                .padding(width * 0.08)
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.geist(size: max(11, width * 0.11), relativeTo: .caption, weight: .bold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(4)
                if !authors.isEmpty {
                    Text(authors)
                        .font(.geist(size: max(9, width * 0.075), relativeTo: .caption2))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(2)
                }
            }
            .padding(width * 0.09)
        }
    }
}
