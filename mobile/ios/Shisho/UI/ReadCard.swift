import SwiftUI

/// Variant E's read bar on a Book's page: the primary action for a Book
/// with an ebook, beside the listen bar when it also has an audiobook, as
/// ListenCard draws it: the chapter, the pages left in the book, how far
/// through, and a progress line. It opens the reader.
struct ReadCard: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let progress: ReadProgress

    private var started: Bool { progress.percentage > 0.001 }
    private var finished: Bool { progress.percentage >= 0.999 }
    private var percent: String { progress.percentage.formatted(.percent.precision(.fractionLength(0))) }

    private var title: LocalizedStringKey {
        if finished { return "Read again" }
        return started ? "Continue reading" : "Read"
    }

    /// Which ebook this opens, when the Book has more than one.
    private var fileName: String? {
        progress.ebookCount > 1 && !progress.fileName.isEmpty ? progress.fileName : nil
    }

    /// The chapter the reader last showed, on a line of its own so a long
    /// name can't push the rest out of view.
    private var chapter: String? {
        started && !finished ? progress.chapter : nil
    }

    private var detail: String? {
        if finished { return String(localized: "Finished") }
        // Before it's been read, how big it is: its length isn't known
        // without the book.
        if !started { return progress.sizeBytes > 0 ? FileFacts.size(progress.sizeBytes) : nil }
        guard let pages = progress.pagesLeft else { return nil }
        return String(AttributedString(localized: "^[\(pages) page](inflect: true) left").characters)
    }

    private var titleText: some View {
        Text(title)
            .font(.geist(.callout, weight: .bold))
            .fixedSize(horizontal: false, vertical: true)
    }

    var body: some View {
        Button { model.read(fileID: progress.fileID) } label: {
            // At accessibility text sizes the parts stack, as ListenCard's
            // do, so no word breaks.
            let stacked = dynamicTypeSize.isAccessibilitySize
            let layout = stacked
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
                : AnyLayout(HStackLayout(spacing: 12))
            layout {
                HStack(spacing: 12) {
                    Icon(finished ? .rotateCcw : .bookOpen, size: 22, relativeTo: nil)
                        .frame(width: 24)
                    if stacked { titleText }
                }
                VStack(alignment: .leading, spacing: 2) {
                    if !stacked { titleText }
                    if let fileName {
                        Text(fileName)
                            .font(.geist(.caption, weight: .medium))
                            .lineLimit(2)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    if let chapter {
                        Text(chapter)
                            .font(.geist(.caption, weight: .medium))
                            .lineLimit(stacked ? nil : 1)
                            .fixedSize(horizontal: false, vertical: stacked)
                    }
                    if let detail {
                        Text(detail)
                            .font(.geist(.caption))
                            .opacity(0.8)
                            .lineLimit(stacked ? nil : 1)
                            .fixedSize(horizontal: false, vertical: stacked)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if started && !finished {
                    Text(percent)
                        .font(.geist(.title3, weight: .heavy))
                        .tracking(-0.4)
                        .monospacedDigit()
                }
            }
            .padding(.top, 12)
            .padding(.bottom, started ? 15 : 12)
            .padding(.horizontal, 16)
            .frame(maxWidth: .infinity, minHeight: 60)
            .foregroundStyle(Theme.primaryForeground)
            .background(Theme.primary)
            .overlay(alignment: .bottomLeading) {
                if started && !finished {
                    GeometryReader { geometry in
                        Rectangle()
                            .fill(Theme.primaryForeground.opacity(0.55))
                            .frame(width: geometry.size.width * progress.percentage, height: 3)
                            .frame(maxHeight: .infinity, alignment: .bottom)
                    }
                    .accessibilityHidden(true)
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .shadow(color: Theme.primary.opacity(0.35), radius: 14, y: 10)
        }
        .buttonStyle(PressableStyle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(title))
        .accessibilityValue(([fileName, chapter, started && !finished ? percent : nil, detail].compactMap { $0 })
            .joined(separator: ", "))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("book.read")
    }
}
