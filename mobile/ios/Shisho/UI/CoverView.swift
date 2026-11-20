import SwiftUI

/// A Book's cover, from the cover cache, with a calm placeholder while it
/// loads or when there's none.
struct CoverView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.displayScale) private var displayScale
    let book: BookSummary
    let width: CGFloat
    var onImage: ((UIImage) -> Void)?

    @State private var image: UIImage?

    private var aspectRatio: CGFloat {
        book.coverAspectRatio.hasPrefix("audiobook") ? 1 : 2.0 / 3.0
    }

    private var size: Int {
        CoverCache.pixelSize(forWidth: width, scale: displayScale)
    }

    var body: some View {
        ZStack {
            Rectangle().fill(Theme.card)
            if let image {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
            } else {
                Image(systemName: "book.closed")
                    .font(.system(size: width / 4))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .frame(width: width, height: width / aspectRatio)
        .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).stroke(Theme.border, lineWidth: 1))
        .accessibilityHidden(true)
        .task(id: "\(book.id)-\(book.coverVersion)-\(size)") {
            guard let covers = model.session?.covers else { return }
            if let cached = covers.cachedImage(bookID: book.id, version: book.coverVersion, size: size) {
                image = cached
            } else {
                image = await covers.image(bookID: book.id, version: book.coverVersion, size: size)
            }
            if let image { onImage?(image) }
        }
    }
}
