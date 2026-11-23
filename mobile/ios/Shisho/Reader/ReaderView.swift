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
            // A sheet takes the appearance it was opened in; this keeps it
            // in step with a theme chosen while it's open.
            .preferredColorScheme(colorScheme)
        }
        .sheet(isPresented: $showingSettings) {
            ReaderSettingsSheet(settings: Bindable(model).settings) { showingSettings = false }
                .presentationDetents([.height(250)])
                .presentationDragIndicator(.visible)
                .presentationBackgroundInteraction(.enabled)
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
            VStack(spacing: 14) {
                Image(systemName: "exclamationmark.triangle")
                    .font(.system(size: 30))
                    .accessibilityHidden(true)
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
                    Image(systemName: "chevron.left")
                        .font(.system(size: 20, weight: .semibold))
                    Text("Book")
                        .font(.geist(.subheadline, weight: .medium))
                }
                .foregroundStyle(Theme.primary)
                .frame(minWidth: 44, minHeight: 44, alignment: .leading)
                .padding(.leading, 6)
            }
            .accessibilityLabel(Text("Close the book"))
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
                    Image(systemName: "list.bullet")
                        .frame(width: 44, height: 44)
                }
                // The book's own controls only once it shows.
                .disabled(model.contents.isEmpty || model.phase != .reading)
                .accessibilityLabel(Text("Contents"))
                .accessibilityIdentifier("reader.contents")
                Button { showingSettings = true } label: {
                    Image(systemName: "gearshape")
                        .frame(width: 44, height: 44)
                }
                .disabled(model.phase != .reading)
                .accessibilityLabel(Text("Settings"))
                .accessibilityIdentifier("reader.settings")
            }
            .font(.system(size: 17, weight: .medium))
            .foregroundStyle(Theme.foreground)
            .frame(maxWidth: .infinity, alignment: .trailing)
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 2)
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
            .frame(maxWidth: .infinity)
            .frame(height: 22)
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

/// The table of contents, as a sheet with the chapter the reader is in
/// marked.
private struct ContentsSheet: View {
    @Environment(\.dismiss) private var dismiss
    let model: ReaderModel
    let select: (ContentsEntry) -> Void

    var body: some View {
        NavigationStack {
            List(model.contents) { entry in
                let current = entry.title == model.chapterTitle
                Button { select(entry) } label: {
                    HStack(spacing: 10) {
                        Text(entry.title.isEmpty ? String(localized: "Untitled") : entry.title)
                            .font(.geist(.body, weight: current ? .semibold : .regular))
                            .foregroundStyle(entry.target == nil ? Theme.mutedForeground : current ? Theme.primary : Theme.foreground)
                        Spacer(minLength: 8)
                        if let position = entry.position {
                            Text(position.formatted(.percent.precision(.fractionLength(0))))
                                .font(.geist(.footnote))
                                .monospacedDigit()
                                .foregroundStyle(Theme.mutedForeground)
                        }
                    }
                    .padding(.leading, CGFloat(min(entry.depth, 4)) * 16)
                }
                .disabled(entry.target == nil)
                .listRowBackground(current ? Theme.primary.opacity(0.08) : Theme.background)
                .accessibilityAddTraits(current ? .isSelected : [])
                .accessibilityIdentifier("reader.contents.entry")
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .background(Theme.background)
            .navigationTitle(Text("Contents"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                }
            }
        }
    }
}

/// The settings this version offers: theme and layout, as the web reader's
/// settings set them, with its button sets. The rest of its list comes with
/// #920.
private struct ReaderSettingsSheet: View {
    @Binding var settings: ReaderSettings
    let done: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            ZStack {
                Text("Settings")
                    .font(.geist(.subheadline, weight: .semibold))
                    .accessibilityAddTraits(.isHeader)
                HStack {
                    Spacer()
                    Button("Done", action: done)
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                        .accessibilityIdentifier("reader.settings.done")
                }
            }
            .padding(.top, 14)
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

/// One setting's choices as the web app's button set: the chosen one
/// filled, the others outlined.
private struct ChoiceSet<Option: Hashable>: View {
    let title: LocalizedStringResource
    let options: [Option]
    @Binding var selection: Option
    let label: KeyPath<Option, LocalizedStringResource>
    let identifier: String

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title)
                .font(.geist(.footnote, weight: .medium))
                .foregroundStyle(Theme.mutedForeground)
            HStack(spacing: 8) {
                ForEach(options, id: \.self) { option in
                    let chosen = option == selection
                    Button { selection = option } label: {
                        Text(option[keyPath: label])
                            .font(.geist(.footnote, weight: .medium))
                            .frame(maxWidth: .infinity, minHeight: 34)
                            .foregroundStyle(chosen ? Theme.primaryForeground : Theme.foreground)
                            .background(chosen ? Theme.primary : Theme.background,
                                        in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                            .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                                .strokeBorder(chosen ? Color.clear : Theme.border))
                    }
                    .buttonStyle(PressableStyle())
                    .accessibilityAddTraits(chosen ? .isSelected : [])
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier(identifier)
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
                            .font(.system(.callout, design: .serif))
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .scrollBounceBehavior(.basedOnSize)
            .accessibilityIdentifier("reader.note.text")
            Button(action: goToNote) {
                Label("Go to Note", systemImage: "arrow.down.right")
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
