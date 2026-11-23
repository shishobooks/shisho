import SwiftUI
import UIKit

/// The reader for a Reflowable File: the book's own page, with controls
/// over it that a tap in the middle shows and hides. The page stays the
/// book's; the controls take the reader's theme.
struct ReaderView: View {
    let model: ReaderModel
    @State private var showingContents = false
    @State private var showingSettings = false
    /// The page's frame on screen.
    @State private var pagerFrame: CGRect = .zero
    /// The readout counts pages left in the book rather than the chapter,
    /// as the bar does.
    @AppStorage(ReaderBottomBar.pagesLeftInBookKey) private var pagesLeftInBook = false
    /// The settings sheet's height, grown with the text in it.
    @ScaledMetric(relativeTo: .subheadline) private var settingsHeight: CGFloat = 250
    let close: () -> Void

    /// The app's appearance, which the bars and sheets take.
    @Environment(\.colorScheme) private var appScheme
    private var theme: ReaderSettings.Theme { model.settings.theme }
    private var foreground: Color { Color(theme.foreground) }
    /// The reader's controls and sheets are dark under the Dark and Black
    /// themes.

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
                    // Hidden pages are still read by VoiceOver.
                    .accessibilityHidden(model.phase != .reading)
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
                        ReaderBottomBar(model: model).transition(.move(edge: .bottom).combined(with: .opacity))
                    } else {
                        readout
                    }
                }
            }
            if let pull = model.pull {
                PullHint(direction: pull.direction, progress: pull.progress,
                         chapter: model.chapterTitle(toward: pull.direction), foreground: foreground)
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
        // Hidden while reading, bars or not: the page under it may be in a
        // theme the app's appearance doesn't show on, and showing it with
        // the bars would resize the pages on iPhones whose status bar takes
        // its own room.
        .statusBarHidden(model.phase == .reading)
        .task {
            model.settings.appDark = appScheme == .dark
            await model.load()
        }
        .onChange(of: appScheme) { _, scheme in
            model.settings.appDark = scheme == .dark
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didReceiveMemoryWarningNotification)) { _ in
            model.memoryWarning()
        }
        .sheet(isPresented: $showingContents) {
            ContentsSheet(model: model) { entry in
                showingContents = false
                model.go(to: entry)
            }
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
            .presentationBackground(Theme.background)
            .tint(Theme.primary)
        }
        .sheet(isPresented: $showingSettings) {
            ReaderSettingsSheet(settings: Bindable(model).settings) { showingSettings = false }
                .presentationDetents([.height(settingsHeight)])
                .presentationDragIndicator(.visible)
                .presentationBackground(Theme.background)
        }
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

    /// The bars over the page take the app's look, in the reader theme's
    /// appearance: a native navigation bar with the web app's type and
    /// colors (variant E).
    /// Solid, not a blur: a blur over the live pages costs every frame
    /// they or the bars move.
    static var barBackground: some View {
        Theme.background
    }

    private var topBar: some View {
        HStack(spacing: 0) {
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
            .frame(maxWidth: .infinity, alignment: .leading)
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
            .frame(maxWidth: 200)
            .accessibilityElement(children: .combine)
            HStack(spacing: 0) {
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
            .frame(maxWidth: .infinity, alignment: .trailing)
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 2)
        // Past this the bar would cover the page; the buttons show large
        // on a long press instead.
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .background(ReaderView.barBackground.ignoresSafeArea(edges: .top))
        .overlay(alignment: .bottom) { Theme.border.frame(height: 1) }
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
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(readoutLabel))
        .accessibilityHint(pagesLeftInBook ? Text("Counts pages left in the chapter") : Text("Counts pages left in the book"))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("reader.progress")
    }

    private var readoutLabel: String {
        model.placeLabel(percent: percentShown, pagesLeft: pagesLeftLabel)
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
                .contentShape(Rectangle())
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

    var body: some View {
        VStack(spacing: 4) {
            // Back and forward through jumps either side of the bar, as the
            // web reader has them: always there, and disabled when there's
            // nowhere to go, so they're where the reader looks after paging
            // around a jump.
            HStack(spacing: 2) {
                historyButton(.undo2, label: "Back to previous location", enabled: model.history.canGoBack,
                              identifier: "reader.back", action: model.goBack)
                Scrubber(value: scrubbing ?? model.fraction, length: 1, onDrag: { scrubbing = $0 }, onCommit: { value in
                    scrubbing = nil
                    model.go(toFraction: value)
                }, step: 0.01, marks: model.marks)
                .accessibilityLabel(Text("Place in the book"))
                .accessibilityValue(Text(scrubValue))
                .accessibilityIdentifier("reader.scrubber")

                historyButton(.redo2, label: "Forward to next location", enabled: model.history.canGoForward,
                              identifier: "reader.forward", action: model.goForward)
            }
            HStack(spacing: 6) {
                Text((scrubbing.map(model.chapterTitle(at:)) ?? model.chapterTitle) ?? "")
                    .lineLimit(1)
                    // Read with the percentage, as the place.
                    .accessibilityHidden(true)
                Spacer(minLength: 12)
                if let pagesLeft = pagesLeftLabel {
                    Button { pagesLeftInBook.toggle() } label: {
                        Text(pagesLeft)
                            .lineLimit(1)
                            .contentTransition(.numericText())
                            .frame(minHeight: 32)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityHint(pagesLeftInBook ? Text("Counts pages left in the chapter") : Text("Counts pages left in the book"))
                    .accessibilityIdentifier("reader.pagesLeft")
                    Text(verbatim: "·")
                        .accessibilityHidden(true)
                }
                Text(percentShown)
                    .monospacedDigit()
                    .accessibilityLabel(Text(place))
                    .accessibilityIdentifier("reader.progress")
            }
            .font(.geist(.caption))
            .foregroundStyle(Theme.mutedForeground)
            .accessibilityElement(children: .contain)
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .padding(.bottom, 6)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .background(ReaderView.barBackground.ignoresSafeArea(edges: .bottom))
        .overlay(alignment: .top) { Theme.border.frame(height: 1) }
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
private struct ScrubLabel: View {
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
                .accessibilityLabel(Text("Image"))
                .accessibilityAddTraits(.isImage)
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
private struct PagerHost: UIViewControllerRepresentable {
    let pager: PagerController

    func makeUIViewController(context: Context) -> PagerController { pager }
    func updateUIViewController(_ controller: PagerController, context: Context) {}
}

/// The table of contents, as the audio player's chapter list draws it:
/// bordered rows, the chapter the reader is in tinted.
private struct ContentsSheet: View {
    @Environment(\.dismiss) private var dismiss
    let model: ReaderModel
    let select: (ContentsEntry) -> Void

    var body: some View {
        let currentID = model.currentEntry?.id
        VStack(spacing: 0) {
            header
            ScrollViewReader { scroller in
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(model.contents) { entry in
                            if entry.id != model.contents.first?.id {
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

/// The settings this version offers: theme and layout, as the web reader's
/// settings set them. The rest of its list comes with #920.
private struct ReaderSettingsSheet: View {
    @Binding var settings: ReaderSettings
    let done: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            ZStack {
                Text("Settings")
                    .font(.geist(.subheadline, weight: .semibold))
                    .accessibilityAddTraits(.isHeader)
                HStack {
                    Spacer()
                    Button("Done", action: done)
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                        .frame(minHeight: 44)
                        .accessibilityIdentifier("reader.settings.done")
                }
            }
            .padding(.top, 10)
            .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
            ChoiceSet(title: "Theme", options: ReaderSettings.ThemeChoice.allCases, selection: $settings.themeChoice, label: \.label,
                      identifier: "reader.theme")
            ChoiceSet(title: "Layout", options: ReaderSettings.Layout.allCases, selection: $settings.layout, label: \.label,
                      identifier: "reader.layout")
            Spacer(minLength: 0)
        }
        .foregroundStyle(Theme.foreground)
        .padding(.horizontal, 20)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// One setting's choices as the web reader's settings draw them: a row of
/// buttons, the chosen one filled. Not the player's SegmentedChoice, which
/// is the web app's tab switch, a different control there.
private struct ChoiceSet<Option: Hashable & RawRepresentable<String>>: View {
    let title: LocalizedStringResource
    let options: [Option]
    @Binding var selection: Option
    let label: KeyPath<Option, LocalizedStringResource>
    let identifier: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(.geist(.subheadline, weight: .medium))
                .foregroundStyle(Theme.foreground)
                .accessibilityAddTraits(.isHeader)
            HStack(spacing: 8) {
                ForEach(options, id: \.self) { option in
                    let chosen = option == selection
                    Button { selection = option } label: {
                        Text(option[keyPath: label])
                            .font(.geist(.footnote, weight: .medium))
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)
                            .padding(.horizontal, 6)
                            .frame(maxWidth: .infinity, minHeight: 36)
                            .foregroundStyle(chosen ? Theme.primaryForeground : Theme.foreground)
                            .background(chosen ? Theme.primary : Theme.background,
                                        in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                            .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                                .strokeBorder(chosen ? Color.clear : Theme.border))
                            // The button is drawn at the web's height; it
                            // takes taps over 44 points.
                            .padding(.vertical, 4)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(PressableStyle())
                    .accessibilityAddTraits(chosen ? .isSelected : [])
                    .accessibilityIdentifier("\(identifier).\(option.rawValue)")
                }
            }
        }
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
                            .font(note.font.font(relativeTo: .callout))
                            .lineSpacing(note.font.lineSpacing(for: note.lineHeight, relativeTo: .callout,
                                                               base: Font.textStyleSize(.callout)))
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
