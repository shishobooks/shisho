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

    /// Where the reader is, as VoiceOver says it, and as the readout does.
    private var place: String {
        model.placeLabel(percent: percentShown, pagesLeft: pagesLeftLabel)
    }

    /// While the scrubber rests.
    private var pagesLeftLabel: AttributedString? {
        scrubbing == nil ? model.pagesLeftLabel(inBook: pagesLeftInBook) : nil
    }

    var body: some View {
        VStack(spacing: 4) {
            Scrubber(value: scrubbing ?? model.fraction, length: 1, onDrag: { scrubbing = $0 }, onCommit: { value in
                scrubbing = nil
                model.go(toFraction: value)
            }, step: 0.01, marks: model.marks)
            .accessibilityLabel(Text("Place in the book"))
            .accessibilityValue(Text(percentShown))
            .accessibilityIdentifier("reader.scrubber")
            HStack(spacing: 6) {
                Text(model.chapterTitle ?? "")
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
