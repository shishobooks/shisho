import SwiftUI
import UIKit

/// The reader for a Reflowable File: the book's own page, with controls
/// over it that a tap in the middle shows and hides. The page stays the
/// book's; the controls take the reader's theme.
struct ReaderView: View {
    let model: ReaderModel
    @State private var showingContents = false
    @State private var showingSettings = false
    @State private var showingSearch = false
    /// The page's frame on screen.
    @State private var pagerFrame: CGRect = .zero
    /// The readout counts pages left in the book rather than the chapter,
    /// as the bar does.
    @AppStorage(ReaderBottomBar.pagesLeftInBookKey) private var pagesLeftInBook = false
    let close: () -> Void

    /// The app's appearance, which the bars and sheets take.
    @Environment(\.colorScheme) private var appScheme
    private var theme: ReaderSettings.Theme { model.settings.theme }
    private var foreground: Color { Color(theme.foreground) }

    var body: some View {
        ZStack {
            Color(theme.background).ignoresSafeArea()
            if let source = model.source {
                PagerHost(pager: source.pager)
                    // Try Again makes a new source, with new pages.
                    .id(ObjectIdentifier(source))
                    // Where the page is, which a footnote's place is given in.
                    .onGeometryChange(for: CGRect.self, of: { $0.frame(in: .global) }, action: { pagerFrame = $0 })
                    .padding(.top, 8)
                    .padding(.bottom, 28)
                    .accessibilityIdentifier("reader.page")
                    .opacity(model.phase == .reading ? 1 : 0)
                    // Hidden pages are still read by VoiceOver. Under a
                    // sheet, which VoiceOver keeps to anyway, the page's
                    // text isn't read out of its web process each time
                    // the screen's elements are, which UI tests do often.
                    .accessibilityHidden(model.phase != .reading || showingContents || showingSettings)
            }
            status
            if model.turnWaiting {
                turnSpinner
            }
            VStack(spacing: 0) {
                if model.chromeVisible { topBar.transition(.move(edge: .top).combined(with: .opacity)) }
                // Only once the book shows, so going there has a page to move.
                if let offer = model.offer, model.phase == .reading {
                    PlaceOfferBanner(offer: offer, kind: .reading, accept: model.acceptOffer, dismiss: model.dismissOffer)
                        .padding(.horizontal, 12)
                        .padding(.top, 8)
                        .transition(.move(edge: .top).combined(with: .opacity))
                }
                Spacer(minLength: 0)
                // Only once the page shows, so it never reads 0% while the
                // book opens.
                if model.phase == .reading {
                    if model.chromeVisible {
                        if showingSearchBar {
                            searchBar.padding(.bottom, 8)
                        }
                        ReaderBottomBar(model: model).transition(.move(edge: .bottom).combined(with: .opacity))
                    } else if showingSearchBar {
                        // In the readout's place under the page. Paginated,
                        // that and the band under the text are as tall as
                        // the bar, so it covers no text; scrolled, text
                        // passes under its top edge.
                        searchBar
                    } else {
                        readout
                    }
                }
            }
            if let pull = model.pull {
                PullHint(direction: pull.direction, progress: pull.progress,
                         chapter: pull.chapter, foreground: foreground)
                    .frame(maxHeight: .infinity, alignment: pull.direction == .forward ? .bottom : .top)
                    .padding(.vertical, 56)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
            }
            if let image = model.image {
                ImageViewer(image: image.image) { model.image = nil }
                    .transition(.opacity)
            }
            // A card of the app's own rather than a system popover, whose
            // corners and arrow it can't shape, over everything dimmed so
            // the note stands out.
            if let note = model.note {
                NoteOverlay(note: note, theme: theme, page: pagerFrame, goToNote: model.goToNote, close: model.closeNote)
                    .transition(.opacity)
            }
        }
        .animation(.easeOut(duration: 0.18), value: model.chromeVisible)
        .animation(.easeOut(duration: 0.2), value: model.offer)
        .animation(.easeOut(duration: 0.15), value: model.turnWaiting)
        .animation(.easeOut(duration: 0.15), value: model.note)
        .animation(.easeOut(duration: 0.15), value: model.image)
        .animation(.easeOut(duration: 0.18), value: model.search.current == nil)
        // Hidden while reading, bars or not: the page under it may be in a
        // theme the app's appearance doesn't show on, and showing it with
        // the bars would resize the pages on iPhones whose status bar takes
        // its own room.
        .statusBarHidden(model.phase == .reading)
        .background { keyboardShortcuts }
        .task {
            model.settings.appDark = appScheme == .dark
            await model.load()
        }
        // The screen stays on while reading, past the auto-lock time.
        .onAppear { ScreenAwake.hold() }
        .onDisappear { ScreenAwake.release() }
        .onChange(of: appScheme) { _, scheme in
            model.settings.appDark = scheme == .dark
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didReceiveMemoryWarningNotification)) { _ in
            model.memoryWarning()
        }
        .sheet(isPresented: $showingContents) {
            ContentsSheet(entries: model.contents, currentID: model.currentEntry?.id) { entry in
                showingContents = false
                model.go(to: entry)
            }
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
            .presentationBackground(Theme.background)
            .tint(Theme.primary)
        }
        .sheet(isPresented: $showingSettings) {
            // Half height first, so the page shows each change beside it.
            ReaderSettingsSheet(settings: Bindable(model).settings) { showingSettings = false }
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
                .presentationBackground(Theme.background)
                .presentationContentInteraction(.scrolls)
        }
        .sheet(isPresented: $showingSearch) {
            // Leaving the sheet for a result hides the controls, as a
            // contents jump does; stepping from the find bar leaves them as
            // they are, so the bar stays under the finger.
            ReaderSearchSheet(search: model.search, choose: { index in
                showingSearch = false
                model.hideChromeToRead()
                model.search.goToResult(index)
            }, step: { forward in
                showingSearch = false
                model.hideChromeToRead()
                if forward { model.search.next() } else { model.search.previous() }
            }, done: { showingSearch = false })
            // Full height from the start, as the keyboard comes up with it:
            // a half-height sheet would grow once the keyboard showed, and
            // drop back when it went.
            .presentationDetents([.large])
            .presentationDragIndicator(.visible)
            .presentationBackground(Theme.background)
        }
    }

    // MARK: - Keyboard

    /// Search from a hardware keyboard, as in the web reader: Command-F
    /// opens it, Command-G and Shift-Command-G step through the results
    /// from anywhere in the reader, and Escape closes it. Buttons out of
    /// sight, so they work with the controls hidden; the shortcuts list
    /// shows them by name.
    private var keyboardShortcuts: some View {
        Group {
            Button("Search in Book") { showingSearch = true }
                .keyboardShortcut("f", modifiers: .command)
                .disabled(model.phase != .reading)
            Button("Next Result") { model.search.next() }
                .keyboardShortcut("g", modifiers: .command)
                .disabled(!model.search.active)
            Button("Previous Result") { model.search.previous() }
                .keyboardShortcut("g", modifiers: [.command, .shift])
                .disabled(!model.search.active)
            Button("Close Search") { model.search.close() }
                .keyboardShortcut(.escape, modifiers: [])
                .disabled(!model.search.active)
        }
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
    }

    // MARK: - Loading

    @ViewBuilder
    private var status: some View {
        switch model.phase {
        case .loading:
            ProgressView()
                .controlSize(.large)
                .tint(foreground)
                .accessibilityLabel(Text("Opening the book"))
                .accessibilityIdentifier("reader.loading")
        case .failed(let message):
            // Over the page, in the reader theme's colors, rather than the
            // app's NoticeCard, which is drawn for the app's own background.
            VStack(spacing: 14) {
                Icon(.triangleAlert, size: 30, relativeTo: .title)
                Text(message)
                    .font(.geist(.body))
                    .multilineTextAlignment(.center)
                Button("Try Again") { Task { await model.load() } }
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

    /// A chosen search result's bar, while the results are away.
    private var showingSearchBar: Bool {
        model.search.current != nil && !showingSearch
    }

    private var searchBar: some View {
        ReaderSearchBar(search: model.search) { showingSearch = true }
            .transition(.move(edge: .bottom).combined(with: .opacity))
    }

    /// A turn waiting for its page to be laid out.
    private var turnSpinner: some View {
        ProgressView()
            .controlSize(.regular)
            .tint(foreground)
            .padding(14)
            .background(.ultraThinMaterial, in: Circle())
            .transition(.opacity)
            .allowsHitTesting(false)
            .accessibilityLabel(Text("Loading the next page"))
            .accessibilityIdentifier("reader.turnLoading")
    }

    // MARK: - Controls

    /// The bars over the page take the app's look and appearance, whatever
    /// the reader theme: a native navigation bar with the web app's type and
    /// colors (variant E). Solid, not a blur: a blur over the live pages
    /// costs every frame they or the bars move.
    static var barBackground: some View {
        Theme.background
    }

    /// The room each side of the title keeps for the buttons, so the title
    /// stays centered: the three on the right, which don't grow with the
    /// text size, and are wider than Book on the left.
    private static let topBarSide: CGFloat = 3 * 44

    private var topBar: some View {
        // Centered when the title fits between the buttons' room on both
        // sides; otherwise it takes all the room between them, a little off
        // center, as a navigation bar's title does, rather than cutting off.
        ViewThatFits(in: .horizontal) {
            ZStack {
                ReaderTitle(model: model)
                    .fixedSize(horizontal: true, vertical: false)
                    .padding(.horizontal, Self.topBarSide)
                HStack(spacing: 0) {
                    closeButton
                    Spacer(minLength: 0)
                    topBarButtons
                }
            }
            HStack(spacing: 4) {
                closeButton
                ReaderTitle(model: model)
                    .frame(maxWidth: .infinity)
                topBarButtons
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

    private var closeButton: some View {
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

    private var topBarButtons: some View {
        HStack(spacing: 0) {
            Button { showingSearch = true } label: {
                Icon(.search, size: 20)
                    .frame(minWidth: 44, minHeight: 44)
            }
            .disabled(model.phase != .reading)
            .accessibilityLabel(Text("Search"))
            .accessibilityShowsLargeContentViewer()
            .accessibilityIdentifier("reader.search")
            Button { showingContents = true } label: {
                Icon(.list, size: 20)
                    .frame(minWidth: 44, minHeight: 44)
            }
            // The book's own controls only once it shows.
            .disabled(model.contents.isEmpty || model.phase != .reading)
            .accessibilityLabel(Text("Contents"))
            .accessibilityShowsLargeContentViewer()
            .accessibilityIdentifier("reader.contents")
            Button { showingSettings = true } label: {
                Icon(.settings, size: 20)
                    .frame(minWidth: 44, minHeight: 44)
            }
            .disabled(model.phase != .reading)
            .accessibilityLabel(Text("Settings"))
            .accessibilityShowsLargeContentViewer()
            .accessibilityIdentifier("reader.settings")
        }
        .foregroundStyle(Theme.foreground)
    }

    private var percentShown: String {
        model.fraction.formatted(.percent.precision(.fractionLength(0)))
    }

    private var pagesLeftLabel: AttributedString? {
        model.pagesLeftLabel(inBook: pagesLeftInBook)
    }

    /// Where the reader is, small under the page while the controls are
    /// hidden.
    private var readout: some View {
        // The chapter gives way first, so the counts always show. A tap
        // counts the book's pages rather than the chapter's, as the bar's
        // does.
        Button { pagesLeftInBook.toggle() } label: {
            HStack(spacing: 4) {
                if let chapter = model.chapterTitle {
                    Text(chapter)
                        .lineLimit(1)
                    Text(verbatim: "·")
                }
                if let pagesLeft = pagesLeftLabel {
                    Text(pagesLeft)
                        .lineLimit(1)
                        .layoutPriority(1)
                    Text(verbatim: "·")
                }
                Text(percentShown)
                    .layoutPriority(1)
            }
            .font(.geist(.caption2, weight: .medium))
            .monospacedDigit()
            .foregroundStyle(foreground.opacity(0.55))
            .padding(.horizontal, 24)
            .frame(maxWidth: .infinity, minHeight: 22)
            .hitArea(height: 22)
        }
        .buttonStyle(.plain)
        .disabled(pagesLeftLabel == nil)
        .accessibilityLabel(Text(readoutLabel))
        .accessibilityHint(pagesLeftInBook ? Text("Counts pages left in the chapter") : Text("Counts pages left in the book"))
        .accessibilityIdentifier("reader.progress")
    }

    private var readoutLabel: String {
        model.placeLabel(percent: percentShown, pagesLeft: pagesLeftLabel)
    }
}

/// Keeps the screen on while any reader is open. Counted, since a reader
/// that replaces another can appear before the old one disappears.
@MainActor
enum ScreenAwake {
    private static var holds = 0

    static func hold() {
        holds += 1
        UIApplication.shared.isIdleTimerDisabled = true
    }

    static func release() {
        holds = max(0, holds - 1)
        UIApplication.shared.isIdleTimerDisabled = holds > 0
    }
}

/// The chapter and the book over the page. Its own view, so following
/// every move redraws only it.
private struct ReaderTitle: View {
    let model: ReaderModel

    var body: some View {
        VStack(spacing: 1) {
            Text(model.chapterTitle ?? model.file.book.title)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
            if model.chapterTitle != nil {
                Text(model.file.book.title)
                    .font(.geist(.caption2))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .lineLimit(1)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("reader.title")
    }
}

/// The bar under the page: the scrubber, the chapter, pages left, and how
/// far through. Its own view, so a drag on the scrubber redraws only it.
private struct ReaderBottomBar: View {
    static let pagesLeftInBookKey = "reader.pagesLeftInBook"

    let model: ReaderModel
    /// The scrubber's place while it's being dragged.
    @State private var scrubbing: Double?
    /// Counts pages left in the book rather than the chapter, as Now
    /// Playing counts time.
    @AppStorage(ReaderBottomBar.pagesLeftInBookKey) private var pagesLeftInBook = false

    private var percentShown: String {
        (scrubbing ?? model.fraction).formatted(.percent.precision(.fractionLength(0)))
    }

    /// The scrubber's value as VoiceOver says it: the chapter there, and how
    /// far through.
    private var scrubValue: String {
        let fraction = scrubbing ?? model.fraction
        return [model.chapterTitle(at: fraction), percentShown].compactMap { $0 }.joined(separator: ", ")
    }

    /// Where the reader is, as VoiceOver says it, and as the readout does.
    private var place: String {
        model.placeLabel(percent: percentShown, pagesLeft: pagesLeftLabel)
    }

    private func historyButton(_ icon: Lucide, label: LocalizedStringKey, enabled: Bool, identifier: String,
                               action: @escaping () -> Void) -> some View {
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

    /// While the scrubber rests.
    private var pagesLeftLabel: AttributedString? {
        scrubbing == nil ? model.pagesLeftLabel(inBook: pagesLeftInBook) : nil
    }

    /// Back and forward through jumps either side of the bar, as the web
    /// reader has them: always there, and disabled when there's nowhere to
    /// go, so they're where the reader looks after paging around a jump.
    private var scrubberRow: some View {
        HStack(spacing: 2) {
            historyButton(.undo2, label: "Back to previous location", enabled: model.history.canGoBack,
                          identifier: "reader.back", action: model.goBack)
            Scrubber(value: scrubbing ?? model.fraction, length: 1, onDrag: { scrubbing = $0 }, onCommit: { value in
                scrubbing = nil
                model.go(toFraction: value)
            }, step: 0.01, onAdjust: { model.go(toFraction: $0, adjusting: true) }, marks: model.marks)
            .accessibilityLabel(Text("Place in the book"))
            .accessibilityValue(Text(scrubValue))
            .accessibilityIdentifier("reader.scrubber")

            historyButton(.redo2, label: "Forward to next location", enabled: model.history.canGoForward,
                          identifier: "reader.forward", action: model.goForward)
        }
    }

    /// The chapter, pages left, and how far through.

    private func placeRow(showingChapter: Bool) -> some View {
        let pagesLeftHidden = !showingChapter && scrubbing != nil
        return HStack(spacing: 6) {
            if showingChapter {
                Text((scrubbing.map(model.chapterTitle(at:)) ?? model.chapterTitle) ?? "")
                    .lineLimit(1)
                    // Read with the percentage, as the place.
                    .accessibilityHidden(true)
                Spacer(minLength: 12)
            }
            // Kept while the scrubber's dragged, hidden, in the one-row bar,
            // so the scrubber beside it doesn't change width under the drag.
            if let pagesLeft = showingChapter ? pagesLeftLabel : model.pagesLeftLabel(inBook: pagesLeftInBook) {
                Button { pagesLeftInBook.toggle() } label: {
                    Text(pagesLeft)
                        .lineLimit(1)
                        .contentTransition(.numericText())
                        .frame(minHeight: 32)
                        .hitArea(height: 32)
                }
                .buttonStyle(.plain)
                .accessibilityHint(pagesLeftInBook ? Text("Counts pages left in the chapter") : Text("Counts pages left in the book"))
                .accessibilityIdentifier("reader.pagesLeft")
                .opacity(pagesLeftHidden ? 0 : 1)
                .accessibilityHidden(pagesLeftHidden)
                Text(verbatim: "·")
                    .opacity(pagesLeftHidden ? 0 : 1)
                    .accessibilityHidden(true)
            }
            // In the one-row bar, as wide as 100% whatever it reads, so the
            // scrubber beside it keeps its width through a drag.
            ZStack(alignment: .trailing) {
                if !showingChapter {
                    Text(1.0.formatted(.percent.precision(.fractionLength(0))))
                        .hidden()
                        .accessibilityHidden(true)
                }
                Text(percentShown)
                    .accessibilityLabel(Text(place))
                    .accessibilityIdentifier("reader.progress")
            }
            .monospacedDigit()
        }
        .font(.geist(.caption))
        .foregroundStyle(Theme.mutedForeground)
        .accessibilityElement(children: .contain)
    }

    /// Short, as a phone in landscape: one row, the chapter left to the
    /// title over the page, so the bar covers less of the page.
    @Environment(\.verticalSizeClass) private var verticalSizeClass

    var body: some View {
        Group {
            if verticalSizeClass == .compact {
                HStack(spacing: 12) {
                    scrubberRow
                    placeRow(showingChapter: false)
                        .fixedSize()
                }
            } else {
                VStack(spacing: 4) {
                    scrubberRow
                    placeRow(showingChapter: true)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .padding(.bottom, 6)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .background(ReaderView.barBackground.ignoresSafeArea(edges: [.bottom, .horizontal]))
        .overlay(alignment: .top) { Theme.border.frame(height: 1).ignoresSafeArea(edges: .horizontal) }
        // Where a drag would go, above the bar where a finger doesn't
        // cover it, so it isn't blind. After the border, so it's in front.
        .overlay(alignment: .top) {
            if let scrubbing {
                ScrubLabel(chapter: model.chapterTitle(at: scrubbing), percent: percentShown)
                    // Its bottom on the bar's top edge, then 12 points up.
                    .frame(height: 0, alignment: .bottom)
                    .offset(y: -12)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
                    .transition(.opacity)
            }
        }
        .animation(.easeOut(duration: 0.12), value: scrubbing == nil)
    }
}

/// The chapter and place a scrubber drag would go to, in a pill centered
/// over the bar, in the brand color so it catches the eye.
struct ScrubLabel: View {
    let chapter: String?
    let percent: String

    var body: some View {
        HStack(spacing: 6) {
            if let chapter {
                Text(chapter)
                    .font(.geist(.footnote, weight: .semibold))
                    .lineLimit(1)
            }
            Text(percent)
                .font(.geist(.footnote))
                .monospacedDigit()
                .opacity(chapter == nil ? 1 : 0.8)
        }
        .foregroundStyle(Theme.primaryForeground)
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(Theme.primary, in: Capsule())
        .shadow(color: Theme.primary.opacity(0.35), radius: 10, y: 4)
        .padding(.horizontal, 24)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
    }
}

/// Scrolled, the hint for a pull past a section's end or start: it fades in
/// as the pull goes on, and says letting go brings the chapter in once it
/// would.
private struct PullHint: View {
    let direction: TurnDirection
    let progress: Double
    let chapter: String?
    let foreground: Color

    private var armed: Bool { progress >= 1 }

    private var text: String {
        if armed {
            return chapter.map { String(localized: "Release for \($0)") }
                ?? (direction == .forward ? String(localized: "Release for next chapter")
                    : String(localized: "Release for previous chapter"))
        }
        return direction == .forward ? String(localized: "Pull for next chapter") : String(localized: "Pull for previous chapter")
    }

    var body: some View {
        HStack(spacing: 8) {
            Icon(direction == .forward ? .arrowUp : .arrowDown, size: 15)
                .rotationEffect(.degrees(armed ? 180 : 0))
            Text(text)
                .font(.geist(.footnote, weight: .medium))
                .lineLimit(1)
        }
        .foregroundStyle(foreground.opacity(0.75))
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(foreground.opacity(0.08), in: Capsule())
        .padding(.horizontal, 24)
        .opacity(min(1, progress * 1.4))
        .scaleEffect(0.92 + 0.08 * min(1, progress))
        .animation(.snappy(duration: 0.2), value: armed)
    }
}

/// An image from the book on its own, over everything, to pinch or
/// double-tap into; closing goes back to the page.
private struct ImageViewer: View {
    let image: UIImage
    let close: () -> Void
    /// How far a drag down has gone toward closing, from 0 to 1.
    @State private var dismissing: CGFloat = 0

    var body: some View {
        ZStack(alignment: .topTrailing) {
            Color.black.opacity(1 - dismissing * 0.8).ignoresSafeArea()
            ZoomingImage(image: image, onDrag: { dismissing = $0 }, onDismiss: close)
                .ignoresSafeArea()
            Button(action: close) {
                Icon(.x, size: 20)
                    .foregroundStyle(.white)
                    .frame(width: 36, height: 36)
                    .background(.white.opacity(0.18), in: Circle())
                    .frame(width: 44, height: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.trailing, 12)
            .accessibilityLabel(Text("Close"))
            .accessibilityIdentifier("reader.image.close")
        }
        .accessibilityElement(children: .contain)
        .accessibilityAddTraits(.isModal)
        .accessibilityAction(.escape, close)
        .accessibilityIdentifier("reader.image")
    }
}

/// A scroll view that fits the image and zooms it, by pinch or double-tap.
private struct ZoomingImage: UIViewRepresentable {
    let image: UIImage
    /// A drag down's progress toward closing, from 0 to 1.
    let onDrag: (CGFloat) -> Void
    let onDismiss: () -> Void

    func makeUIView(context: Context) -> ZoomView { ZoomView(image: image, onDrag: onDrag, onDismiss: onDismiss) }
    func updateUIView(_ view: ZoomView, context: Context) {}

    final class ZoomView: UIScrollView, UIScrollViewDelegate, UIGestureRecognizerDelegate {
        private let imageView: UIImageView
        private let onDrag: (CGFloat) -> Void
        private let onDismiss: () -> Void
        private var dismissPan: UIPanGestureRecognizer!

        init(image: UIImage, onDrag: @escaping (CGFloat) -> Void, onDismiss: @escaping () -> Void) {
            imageView = UIImageView(image: image)
            self.onDrag = onDrag
            self.onDismiss = onDismiss
            super.init(frame: .zero)
            delegate = self
            showsVerticalScrollIndicator = false
            showsHorizontalScrollIndicator = false
            contentInsetAdjustmentBehavior = .never
            imageView.contentMode = .scaleAspectFit
            // A book's transparent images are drawn for a white page, as the
            // reader's dark themes put white behind them too.
            imageView.backgroundColor = .white
            addSubview(imageView)
            let doubleTap = UITapGestureRecognizer(target: self, action: #selector(doubleTapped(_:)))
            doubleTap.numberOfTapsRequired = 2
            addGestureRecognizer(doubleTap)
            // A drag down closes the image, as in Photos, while it isn't
            // zoomed in; zoomed, a drag pans it.
            dismissPan = UIPanGestureRecognizer(target: self, action: #selector(dragged(_:)))
            dismissPan.delegate = self
            addGestureRecognizer(dismissPan)
            panGestureRecognizer.require(toFail: dismissPan)
            // VoiceOver's swipe up and down zooms, for those who can't pinch.
            isAccessibilityElement = true
            accessibilityLabel = String(localized: "Image")
            accessibilityHint = String(localized: "Swipe up or down to zoom")
            accessibilityTraits = [.image, .adjustable]
        }

        override func accessibilityIncrement() {
            setZoomScale(min(maximumZoomScale, zoomScale * 2), animated: true)
        }

        override func accessibilityDecrement() {
            setZoomScale(max(minimumZoomScale, zoomScale / 2), animated: true)
        }

        override func gestureRecognizerShouldBegin(_ recognizer: UIGestureRecognizer) -> Bool {
            guard recognizer === dismissPan else { return super.gestureRecognizerShouldBegin(recognizer) }
            let velocity = dismissPan.velocity(in: self)
            return zoomScale <= minimumZoomScale * 1.01 && abs(velocity.y) > abs(velocity.x) * 1.5
        }

        private static let dismissDistance: CGFloat = 140

        @objc private func dragged(_ recognizer: UIPanGestureRecognizer) {
            let dy = recognizer.translation(in: self).y
            switch recognizer.state {
            case .changed:
                imageView.transform = CGAffineTransform(translationX: 0, y: dy).scaledBy(x: zoomScale, y: zoomScale)
                onDrag(min(1, abs(dy) / (Self.dismissDistance * 2)))
            case .ended, .cancelled:
                let velocity = recognizer.velocity(in: self).y
                let closes = recognizer.state == .ended && (abs(dy) > Self.dismissDistance || abs(velocity) > 900)
                if closes {
                    let away = (dy >= 0 ? 1 : -1) * bounds.height
                    UIView.animate(withDuration: 0.22, delay: 0, options: .curveEaseIn) {
                        self.imageView.transform = CGAffineTransform(translationX: 0, y: away).scaledBy(x: self.zoomScale, y: self.zoomScale)
                    } completion: { _ in self.onDismiss() }
                    onDrag(1)
                } else {
                    UIView.animate(withDuration: 0.3, delay: 0, usingSpringWithDamping: 0.85, initialSpringVelocity: 0) {
                        self.imageView.transform = CGAffineTransform(scaleX: self.zoomScale, y: self.zoomScale)
                    }
                    onDrag(0)
                }
            default:
                break
            }
        }

        required init?(coder: NSCoder) { nil }

        /// The size the image was last fitted to: fitting again only when it
        /// changes, as on rotation, since setting the zoom lays out again.
        private var fittedTo: CGSize = .zero

        override func layoutSubviews() {
            super.layoutSubviews()
            guard bounds.size != fittedTo, bounds.width > 0, let size = imageView.image?.size, size.width > 0 else {
                center()
                return
            }
            fittedTo = bounds.size
            // Back to no zoom before the frame is set: a frame set under a
            // zoom's transform is undone by it.
            minimumZoomScale = 1
            maximumZoomScale = 1
            zoomScale = 1
            imageView.frame = CGRect(origin: .zero, size: size)
            contentSize = size
            let fit = min(bounds.width / size.width, bounds.height / size.height)
            maximumZoomScale = max(fit * 4, 1)
            minimumZoomScale = fit
            zoomScale = fit
            center()
        }

        /// Keeps a smaller image in the middle.
        private func center() {
            let x = max(0, (bounds.width - contentSize.width) / 2)
            let y = max(0, (bounds.height - contentSize.height) / 2)
            contentInset = UIEdgeInsets(top: y, left: x, bottom: y, right: x)
        }

        func viewForZooming(in scrollView: UIScrollView) -> UIView? { imageView }

        func scrollViewDidZoom(_ scrollView: UIScrollView) { center() }

        @objc private func doubleTapped(_ recognizer: UITapGestureRecognizer) {
            if zoomScale > minimumZoomScale * 1.01 {
                setZoomScale(minimumZoomScale, animated: true)
            } else {
                let point = recognizer.location(in: imageView)
                let scale = min(maximumZoomScale, minimumZoomScale * 2.5)
                let size = CGSize(width: bounds.width / scale, height: bounds.height / scale)
                zoom(to: CGRect(x: point.x - size.width / 2, y: point.y - size.height / 2, width: size.width, height: size.height),
                     animated: true)
            }
        }
    }
}

/// The pager's view controller, in SwiftUI.
struct PagerHost: UIViewControllerRepresentable {
    let pager: PagerController

    func makeUIViewController(context: Context) -> PagerController { pager }
    func updateUIViewController(_ controller: PagerController, context: Context) {}
}

/// The table of contents, as the audio player's chapter list draws it:
/// bordered rows, the chapter the reader is in tinted.
struct ContentsSheet: View {
    @Environment(\.dismiss) private var dismiss
    let entries: [ContentsEntry]
    /// The chapter the reader is in.
    let currentID: Int?
    let select: (ContentsEntry) -> Void

    var body: some View {
        VStack(spacing: 0) {
            header
            ScrollViewReader { scroller in
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(entries) { entry in
                            if entry.id != entries.first?.id {
                                Rectangle().fill(Theme.border).frame(height: 1)
                            }
                            row(entry, current: entry.id == currentID)
                                .id(entry.id)
                        }
                    }
                    .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
                    .padding(.horizontal, 16)
                    .padding(.bottom, 24)
                }
                .onAppear {
                    if let currentID { scroller.scrollTo(currentID, anchor: .center) }
                }
            }
        }
        .background(Theme.background)
    }

    private var header: some View {
        ZStack {
            Text("Contents")
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
                .accessibilityAddTraits(.isHeader)
            HStack {
                Spacer()
                Button("Done") { dismiss() }
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.primary)
                    .frame(minHeight: 44)
                    .accessibilityIdentifier("reader.contents.done")
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 18)
        .padding(.bottom, 10)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
    }

    private func row(_ entry: ContentsEntry, current: Bool) -> some View {
        let title = entry.title.isEmpty ? String(localized: "Untitled") : entry.title
        let percent = entry.position?.formatted(.percent.precision(.fractionLength(0)))
        return Button { select(entry) } label: {
            HStack(spacing: 12) {
                Text(title)
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(entry.target == nil ? Theme.mutedForeground : current ? Theme.primary : Theme.foreground)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if let percent {
                    Text(percent)
                        .font(.geist(.caption))
                        .monospacedDigit()
                        .foregroundStyle(current ? Theme.primary : Theme.mutedForeground)
                }
            }
            .padding(.leading, 12 + CGFloat(min(entry.depth, 4)) * 16)
            .padding(.trailing, 12)
            .padding(.vertical, 11)
            .frame(minHeight: 44)
            .background(current ? Theme.primary.opacity(0.1) : Color.clear)
            .contentShape(Rectangle())
        }
        .buttonStyle(RowPressStyle())
        // A heading, or a chapter this copy of the book doesn't have.
        .disabled(entry.target == nil)
        .accessibilityLabel(percent.map { "\(title), \($0)" } ?? title)
        .accessibilityAddTraits(current ? .isSelected : [])
        .accessibilityIdentifier("reader.contents.entry")
    }
}

/// A footnote's card beside its reference, over everything dimmed a
/// little, with an arrow to the reference. A tap outside closes it.
private struct NoteOverlay: View {
    let note: OpenNote
    let theme: ReaderSettings.Theme
    /// The page's frame on screen, which the note's place is given in.
    let page: CGRect
    let goToNote: () -> Void
    let close: () -> Void
    @State private var cardSize: CGSize = .zero

    var body: some View {
        GeometryReader { geometry in
            let screen = geometry.frame(in: .global)
            let width = min(page.width - NotePlacement.margin * 2, 380)
            let placement = NotePlacement(reference: note.rect, card: cardSize, in: page.size)
            ZStack(alignment: .topLeading) {
                Color.black.opacity(theme == .light || theme == .sepia ? 0.18 : 0.4)
                    .contentShape(Rectangle())
                    .onTapGesture(perform: close)
                    .accessibilityHidden(true)
                NoteCard(note: note, theme: theme, maxTextHeight: min(360, page.height * 0.45), goToNote: goToNote)
                    .frame(width: width)
                    .background {
                        NoteCardShape(arrowX: placement.arrowX, above: placement.above)
                            .fill(Color(theme.background))
                            .shadow(color: .black.opacity(0.25), radius: 18, y: 6)
                    }
                    .onGeometryChange(for: CGSize.self, of: \.size) { cardSize = $0 }
                    .offset(x: page.minX - screen.minX + placement.origin.x, y: page.minY - screen.minY + placement.origin.y)
                    // Placed once its height is known.
                    .opacity(cardSize == .zero ? 0 : 1)
                    .accessibilityAddTraits(.isModal)
                    .accessibilityAction(.escape, close)
            }
        }
        .ignoresSafeArea()
    }
}

/// The card with its arrow, on the edge toward the reference.
private struct NoteCardShape: Shape {
    let arrowX: CGFloat
    let above: Bool

    func path(in rect: CGRect) -> Path {
        let card = Path(roundedRect: rect, cornerRadius: NotePlacement.radius, style: .continuous)
        let half = NotePlacement.arrowWidth / 2
        let height = NotePlacement.arrowHeight
        let edge = above ? rect.maxY : rect.minY
        let tip = above ? edge + height : edge - height
        // Into the card a little, so the two join with no seam: one shape,
        // as two overlapping ones drawn opposite ways leave the overlap
        // unfilled.
        let base = above ? edge - 2 : edge + 2
        var arrow = Path()
        arrow.move(to: CGPoint(x: rect.minX + arrowX - half, y: base))
        arrow.addLine(to: CGPoint(x: rect.minX + arrowX, y: tip))
        arrow.addLine(to: CGPoint(x: rect.minX + arrowX + half, y: base))
        arrow.closeSubpath()
        return card.union(arrow)
    }
}

/// A footnote's text in the page's colors, with a way to the full note.
private struct NoteCard: View {
    let note: OpenNote
    let theme: ReaderSettings.Theme
    /// The text's height past which it scrolls.
    let maxTextHeight: CGFloat
    let goToNote: () -> Void
    @State private var textHeight: CGFloat = 0

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            // As tall as the text, up to maxTextHeight, then it scrolls.
            ScrollView {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(note.paragraphs.enumerated()), id: \.offset) { _, paragraph in
                        Text(paragraph)
                            // The note is the book's text, not the app's,
                            // with the page's spacing between its lines.
                            .font(note.font.font(size: note.size))
                            .lineSpacing(note.font.lineSpacing(for: note.lineHeight, size: note.size))
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .onGeometryChange(for: CGFloat.self, of: \.size.height) { textHeight = $0 }
            }
            .frame(height: min(textHeight, maxTextHeight))
            .scrollBounceBehavior(.basedOnSize)
            .accessibilityIdentifier("reader.note.text")
            Button(action: goToNote) {
                Label("Go to Note", icon: .arrowDownRight)
                    .font(.geist(.subheadline, weight: .semibold))
                    // A 44-point target without the room around it.
                    .hitArea(height: 20)
            }
            .foregroundStyle(Color(UIColor(hex: theme.colors.noteLink)))
            .accessibilityIdentifier("reader.note.go")
        }
        .foregroundStyle(Color(theme.foreground))
        .padding(.horizontal, 16)
        .padding(.top, 14)
        .padding(.bottom, 12)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("reader.note")
    }
}
