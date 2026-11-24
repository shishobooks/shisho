import SwiftUI

/// The controls every reader shares (ADR 0026: the chrome is written once):
/// the bar over the page, the way back to the Book, the opening and failure
/// states, and back and forward through jumps.

/// The bar over the page, in the app's look whatever the page's: a native
/// navigation bar with the web app's type and colors (variant E). Solid,
/// not a blur: a blur over the live pages costs every frame they or the
/// bars move.
struct ReaderTopBar<Title: View, Buttons: View>: View {
    /// The room each side of the title keeps for the buttons, so the title
    /// stays centered: the wider side, which doesn't grow with the text
    /// size.
    let sideRoom: CGFloat
    let close: () -> Void
    @ViewBuilder let title: () -> Title
    @ViewBuilder let buttons: () -> Buttons

    var body: some View {
        // Centered when the title fits between the buttons' room on both
        // sides; otherwise it takes all the room between them, a little off
        // center, as a navigation bar's title does, rather than cutting off.
        ViewThatFits(in: .horizontal) {
            ZStack {
                title()
                    .fixedSize(horizontal: true, vertical: false)
                    .padding(.horizontal, sideRoom)
                HStack(spacing: 0) {
                    ReaderCloseButton(close: close)
                    Spacer(minLength: 0)
                    buttons()
                }
            }
            HStack(spacing: 4) {
                ReaderCloseButton(close: close)
                title()
                    .frame(maxWidth: .infinity)
                buttons()
            }
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 2)
        // Past this the bar would cover the page; the buttons show large
        // on a long press instead.
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        // To the screen's edges in landscape, past the sides' safe areas,
        // which the buttons stay inside.
        .background(ReaderView.barBackground.ignoresSafeArea(edges: [.top, .horizontal]))
        .overlay(alignment: .bottom) { Theme.border.frame(height: 1).ignoresSafeArea(edges: .horizontal) }
    }
}

/// The chapter and the book over the page, or the book alone.
struct ReaderTitleText: View {
    let chapter: String?
    let book: String

    var body: some View {
        VStack(spacing: 1) {
            Text(chapter ?? book)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
            if chapter != nil {
                Text(book)
                    .font(.geist(.caption2))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .lineLimit(1)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("reader.title")
    }
}

/// Back to the Book's page.
struct ReaderCloseButton: View {
    let close: () -> Void

    var body: some View {
        Button(action: close) {
            HStack(spacing: 2) {
                Icon(.chevronLeft, size: 24, relativeTo: .body)
                Text("Book")
                    .font(.geist(.subheadline, weight: .medium))
            }
            .foregroundStyle(Theme.primary)
            .frame(minWidth: 44, minHeight: 44, alignment: .leading)
            .padding(.leading, 6)
        }
        // Named as it shows, so Voice Control's "Tap Book" works.
        .accessibilityHint(Text("Closes the book"))
        .accessibilityInputLabels([Text("Book"), Text("Close")])
        .accessibilityShowsLargeContentViewer()
        .accessibilityIdentifier("reader.close")
    }
}

/// The book opening, or why it couldn't, over the page in the page's own
/// colors rather than the app's NoticeCard, which is drawn for the app's
/// background.
struct ReaderStatus: View {
    let phase: ReaderModel.Phase
    let foreground: Color
    let retry: () -> Void

    var body: some View {
        switch phase {
        case .loading:
            ProgressView()
                .controlSize(.large)
                .tint(foreground)
                .accessibilityLabel(Text("Opening the book"))
                .accessibilityIdentifier("reader.loading")
        case .failed(let message):
            VStack(spacing: 14) {
                Icon(.triangleAlert, size: 30, relativeTo: .title)
                Text(message)
                    .font(.geist(.body))
                    .multilineTextAlignment(.center)
                Button("Try Again", action: retry)
                    .buttonStyle(OutlineButtonStyle())
                    .frame(maxWidth: 200)
                    .accessibilityIdentifier("reader.retry")
            }
            .foregroundStyle(foreground)
            .padding(32)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("reader.error")
        case .reading:
            EmptyView()
        }
    }
}

/// Back or forward through jumps, either side of the bar under the page,
/// as the web reader has them: always there, and disabled when there's
/// nowhere to go, so they're where the reader looks after paging around a
/// jump.
struct ReaderHistoryButton: View {
    let icon: Lucide
    let label: LocalizedStringKey
    let enabled: Bool
    let identifier: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Icon(icon, size: 18)
                .frame(width: 36, height: 44)
                // 44 points wide to tap, a little into the scrubber's ends,
                // which only its handle moves.
                .contentShape(Rectangle().inset(by: -4))
        }
        .buttonStyle(.plain)
        .foregroundStyle(Theme.foreground.opacity(enabled ? 0.85 : 0.3))
        .disabled(!enabled)
        .accessibilityLabel(Text(label))
        .accessibilityShowsLargeContentViewer()
        .accessibilityIdentifier(identifier)
    }
}
