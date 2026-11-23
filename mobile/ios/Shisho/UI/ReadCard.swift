import SwiftUI

/// Read on a Book's page (variant E). Beside an audiobook's listen bar it's
/// the outlined button under it, with how far through the ebook the user
/// is; for a Book with only an ebook it's the primary bar, as the listen
/// bar is for audiobooks.
struct ReadCard: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let progress: ReadProgress
    /// The Book also has an audiobook, whose bar is the primary one.
    var secondary = false

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

    private var detail: String {
        var parts: [String] = []
        if let fileName { parts.append(fileName) }
        if finished {
            parts.append(String(localized: "Finished"))
        } else if started {
            parts.append(percent)
        }
        return parts.joined(separator: " · ")
    }

    var body: some View {
        Button { model.read(fileID: progress.fileID) } label: {
            if secondary { outlined } else { primary }
        }
        .buttonStyle(PressableStyle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(title))
        .accessibilityValue(detail)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("book.read")
    }

    /// The outlined button under the listen bar, as variant E draws it:
    /// lighter than OutlineButtonStyle, a translucent fill under the
    /// cover's wash and the input border, and it carries how far through.
    private var outlined: some View {
        // At accessibility text sizes the detail goes under the title, so
        // no word breaks.
        let stacked = dynamicTypeSize.isAccessibilitySize
        let layout = stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 4))
            : AnyLayout(HStackLayout(spacing: 8))
        return layout {
            HStack(spacing: 8) {
                Icon(finished ? .rotateCcw : .bookOpen, size: 17, relativeTo: nil)
                Text(title)
                    .font(.geist(.callout, weight: .semibold))
                    .fixedSize(horizontal: false, vertical: true)
            }
            if !detail.isEmpty {
                Text(detail)
                    .font(.geist(.footnote))
                    .foregroundStyle(Theme.mutedForeground)
                    .lineLimit(stacked ? nil : 1)
                    .fixedSize(horizontal: false, vertical: stacked)
            }
        }
        .frame(maxWidth: stacked ? .infinity : nil, alignment: .leading)
        .foregroundStyle(Theme.foreground)
        .padding(.vertical, stacked ? 10 : 0)
        .padding(.horizontal, 16)
        .frame(maxWidth: .infinity, minHeight: 46)
        .background(Theme.background.opacity(0.7), in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(Theme.input))
        .shadow(color: .black.opacity(0.04), radius: 1, y: 1)
    }

    /// The primary bar, as ListenCard draws it, for a Book with only an
    /// ebook.
    private var primary: some View {
        // At accessibility text sizes the parts stack, as ListenCard's do.
        let stacked = dynamicTypeSize.isAccessibilitySize
        let layout = stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
            : AnyLayout(HStackLayout(spacing: 12))
        let titleText = Text(title)
            .font(.geist(.callout, weight: .bold))
            .fixedSize(horizontal: false, vertical: true)
        return layout {
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
                        .lineLimit(stacked ? nil : 1)
                        .fixedSize(horizontal: false, vertical: stacked)
                }
                if finished {
                    Text("Finished")
                        .font(.geist(.caption))
                        .opacity(0.8)
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
}
