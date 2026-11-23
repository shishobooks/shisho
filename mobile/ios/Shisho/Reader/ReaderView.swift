import SwiftUI
import UIKit

/// The reader for a Reflowable File: the book's own page, with controls
/// over it that a tap in the middle shows and hides. The page stays the
/// book's; the controls take the reader's theme.
struct ReaderView: View {
    let model: ReaderModel
    @State private var showingContents = false
    @State private var showingSettings = false
    /// The scrubber's place while it's being dragged.
    @State private var scrubbing: Double?
    /// The settings sheet's height, grown with the text in it.
    @ScaledMetric(relativeTo: .subheadline) private var settingsHeight: CGFloat = 250
    let close: () -> Void

    private var theme: ReaderSettings.Theme { model.settings.theme }
    private var foreground: Color { Color(theme.foreground) }
    /// The reader's controls and sheets are dark under the Dark and Black
    /// themes.
    private var colorScheme: ColorScheme { theme.isDark ? .dark : .light }

    var body: some View {
        ZStack {
            Color(theme.background).ignoresSafeArea()
            if let source = model.source {
                PagerHost(pager: source.pager)
                    // Try Again makes a new source, with new pages.
                    .id(ObjectIdentifier(source))
                    .padding(.top, 8)
                    .padding(.bottom, 28)
                    .popover(item: noteBinding, attachmentAnchor: .rect(.rect(model.note?.rect.offsetBy(dx: 0, dy: 8) ?? .zero)),
                             arrowEdge: .bottom) { note in
                        NotePopover(note: note, theme: theme) { model.goToNote() }
                            .presentationCompactAdaptation(.popover)
                    }
                    .accessibilityIdentifier("reader.page")
                    .opacity(model.phase == .reading ? 1 : 0)
                    // Hidden pages are still read by VoiceOver.
                    .accessibilityHidden(model.phase != .reading)
            }
            status
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
                        bottomBar.transition(.move(edge: .bottom).combined(with: .opacity))
                    } else {
                        readout
                    }
                }
            }
        }
        .animation(.easeOut(duration: 0.18), value: model.chromeVisible)
        .animation(.easeOut(duration: 0.2), value: model.offer)
        .preferredColorScheme(colorScheme)
        .task { await model.load() }
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
            // A sheet takes the appearance it was opened in; this keeps it
            // in step with a theme chosen while it's open.
            .preferredColorScheme(colorScheme)
        }
        .sheet(isPresented: $showingSettings) {
            ReaderSettingsSheet(settings: Bindable(model).settings) { showingSettings = false }
                .presentationDetents([.height(settingsHeight)])
                .presentationDragIndicator(.visible)
                .presentationBackground(Theme.background)
                .preferredColorScheme(colorScheme)
        }
    }

    private var noteBinding: Binding<OpenNote?> {
        Binding(get: { model.note }, set: { if $0 == nil { model.closeNote() } })
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

    // MARK: - Controls

    /// The bars over the page take the app's look, in the reader theme's
    /// appearance: a native navigation bar with the web app's type and
    /// colors (variant E).
    private var barBackground: some View {
        Theme.background.opacity(0.92).background(.ultraThinMaterial)
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
        .background(barBackground.ignoresSafeArea(edges: .top))
        .overlay(alignment: .bottom) { Theme.border.frame(height: 1) }
    }

    private var percentShown: String {
        (scrubbing ?? model.fraction).formatted(.percent.precision(.fractionLength(0)))
    }

    /// Where the reader is, as the controls and VoiceOver say it.
    private var place: String {
        model.chapterTitle.map { "\($0) · \(percentShown)" } ?? percentShown
    }

    private var bottomBar: some View {
        VStack(spacing: 4) {
            Scrubber(value: scrubbing ?? model.fraction, length: 1, onDrag: { scrubbing = $0 }, onCommit: { value in
                scrubbing = nil
                model.go(toFraction: value)
            }, step: 0.01, marks: model.marks)
            .accessibilityLabel(Text("Place in the book"))
            .accessibilityValue(Text(percentShown))
            .accessibilityIdentifier("reader.scrubber")
            HStack {
                Text(model.chapterTitle ?? "")
                    .lineLimit(1)
                Spacer(minLength: 12)
                Text(percentShown)
                    .monospacedDigit()
            }
            .font(.geist(.caption))
            .foregroundStyle(Theme.mutedForeground)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text(place))
            .accessibilityIdentifier("reader.progress")
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .padding(.bottom, 6)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .background(barBackground.ignoresSafeArea(edges: .bottom))
        .overlay(alignment: .top) { Theme.border.frame(height: 1) }
    }

    /// Where the reader is, small under the page while the controls are
    /// hidden.
    private var readout: some View {
        Text(place)
            .font(.geist(.caption2, weight: .medium))
            .monospacedDigit()
            .foregroundStyle(foreground.opacity(0.55))
            .lineLimit(1)
            .padding(.horizontal, 24)
            .frame(maxWidth: .infinity, minHeight: 22)
            .accessibilityLabel(Text(place))
            .accessibilityIdentifier("reader.progress")
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
                    VStack(spacing: 0) {
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
            ChoiceSet(title: "Theme", options: ReaderSettings.Theme.allCases, selection: $settings.theme, label: \.label,
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

/// A footnote beside its reference, in the page's colors, with a way to
/// the full note.
private struct NotePopover: View {
    let note: OpenNote
    let theme: ReaderSettings.Theme
    let goToNote: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ScrollView {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(note.paragraphs.enumerated()), id: \.offset) { _, paragraph in
                        Text(paragraph)
                            // The note is the book's text, not the app's.
                            .font(note.font.font(relativeTo: .callout))
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .scrollBounceBehavior(.basedOnSize)
            .accessibilityIdentifier("reader.note.text")
            Button(action: goToNote) {
                Label("Go to Note", icon: .arrowDownRight)
                    .font(.geist(.subheadline, weight: .semibold))
            }
            .foregroundStyle(Color(UIColor(hex: theme.colors.noteLink)))
            .accessibilityIdentifier("reader.note.go")
        }
        .foregroundStyle(Color(theme.foreground))
        .padding(16)
        .frame(idealWidth: 320, maxWidth: 360, maxHeight: 320)
        .fixedSize(horizontal: false, vertical: true)
        .background(Color(theme.background))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("reader.note")
    }
}
