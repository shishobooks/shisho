import SwiftUI
import UniformTypeIdentifiers

/// The reader's display settings, the web reader's list with its names and
/// steps (app/components/library/ReaderSettingsPanel.tsx). Each change
/// applies to the page at once and keeps the reader's place.
struct ReaderSettingsSheet: View {
    @Binding var settings: ReaderSettings
    let done: () -> Void
    private let fonts = ImportedFontStore.shared

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    fontRow
                    NumberSetting(setting: .size, value: $settings.size)
                    VStack(alignment: .leading, spacing: 4) {
                        SettingToggle(title: "Force font size", isOn: $settings.forceFontSize, identifier: "reader.forceFontSize")
                        Text("For books that set their own text sizes and ignore the font size.")
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                    }
                    NumberSetting(setting: .lineSpacing, value: $settings.lineSpacing)
                    NumberSetting(setting: .margins, value: $settings.margins)
                    SettingToggle(title: "Justify text", isOn: $settings.justify, identifier: "reader.justify")
                    ChoiceSet(title: "Theme", options: ReaderSettings.ThemeChoice.allCases, selection: $settings.themeChoice,
                              label: \.label, identifier: "reader.theme")
                    ChoiceSet(title: "Layout", options: ReaderSettings.Layout.allCases, selection: $settings.layout,
                              label: \.label, identifier: "reader.layout")
                    ChoiceSet(title: "Pages", options: ReaderSettings.Pages.allCases, selection: $settings.pages, label: \.label,
                              identifier: "reader.pages",
                              disabledReason: settings.layout == .scrolled ? "Only for Paginated" : nil)
                }
                .padding(.horizontal, 20)
                .padding(.top, 8)
                .padding(.bottom, 24)
                .frame(maxWidth: 560, alignment: .leading)
                .frame(maxWidth: .infinity)
            }
            .foregroundStyle(Theme.foreground)
            .background(Theme.background)
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(Theme.background, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done", action: done)
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                        .accessibilityIdentifier("reader.settings.done")
                }
            }
            .navigationDestination(for: FontListRoute.self) { _ in
                ReaderFontList(font: $settings.font, done: done)
            }
        }
        .tint(Theme.primary)
    }

    /// The chosen font, which opens the list of fonts.
    private var fontRow: some View {
        let name = ReaderFonts.name(of: settings.font, imported: fonts.fonts)
        return NavigationLink(value: FontListRoute()) {
            HStack(spacing: 8) {
                Text("Font")
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                Spacer(minLength: 8)
                Text(name)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .lineLimit(1)
                    .accessibilityIdentifier("reader.font.value")
                Icon(.chevronRight, size: 16, relativeTo: .subheadline)
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityHidden(true)
            }
            .padding(.horizontal, 12)
            .frame(minHeight: 44)
            .background(Theme.background, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
            .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .accessibilityLabel(Text("Font"))
        .accessibilityValue(Text(name))
        .accessibilityIdentifier("reader.font")
    }
}

private struct FontListRoute: Hashable {}

/// A setting that's on or off. The whole row flips it, not only the
/// switch, and it reads as a switch does: a toggle whose value is 1 or 0.
private struct SettingToggle: View {
    let title: LocalizedStringResource
    @Binding var isOn: Bool
    let identifier: String

    var body: some View {
        Button { isOn.toggle() } label: {
            HStack {
                Text(title)
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                Spacer(minLength: 8)
                // Drawn only: the row takes the tap.
                Toggle(isOn: $isOn) { Text(title) }
                    .labelsHidden()
                    .tint(Theme.primary)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
            }
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(title))
        .accessibilityValue(Text(verbatim: isOn ? "1" : "0"))
        .accessibilityAddTraits(.isToggle)
        .accessibilityIdentifier(identifier)
    }
}

/// A setting set by number: a slider for big moves and minus and plus for
/// one step, with the value shown, so a reader can set an exact value. The
/// value follows a drag, and the book reflows only when it ends.
private struct NumberSetting: View {
    let setting: ReaderSettings.Numeric
    @Binding var value: Double
    /// The slider's value while it's dragged.
    @State private var draft: Double?

    private var shown: Double { draft ?? value }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline) {
                Text(setting.label)
                    .font(.geist(.subheadline, weight: .medium))
                Spacer(minLength: 8)
                Text(setting.text(shown))
                    .font(.geist(.subheadline))
                    .monospacedDigit()
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityIdentifier("reader.\(setting.identifier).value")
            }
            // The slider says its value; the readout beside it would repeat
            // it.
            .accessibilityElement(children: .ignore)
            HStack(spacing: 0) {
                step(-1)
                Slider(value: Binding(get: { shown }, set: { draft = setting.snapped($0) }),
                       in: setting.range, step: setting.step) { editing in
                    guard !editing, let draft else { return }
                    value = draft
                    self.draft = nil
                }
                .tint(Theme.primary)
                .accessibilityLabel(Text(setting.label))
                .accessibilityValue(Text(setting.text(shown)))
                // VoiceOver's swipes move one step and apply at once.
                .accessibilityAdjustableAction { direction in
                    switch direction {
                    case .increment: value = setting.stepped(value, by: 1)
                    case .decrement: value = setting.stepped(value, by: -1)
                    @unknown default: break
                    }
                }
                .accessibilityIdentifier("reader.\(setting.identifier).slider")
                step(1)
            }
            if let ends = setting.ends {
                HStack {
                    Text(ends.0)
                    Spacer()
                    Text(ends.1)
                }
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
                .padding(.horizontal, 44)
                .accessibilityHidden(true)
            }
        }
    }

    private func step(_ direction: Int) -> some View {
        let atEnd = direction > 0 ? value >= setting.range.upperBound : value <= setting.range.lowerBound
        return Button {
            draft = nil
            value = setting.stepped(value, by: direction)
        } label: {
            Icon(direction > 0 ? .plus : .minus, size: 18, relativeTo: .subheadline)
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .foregroundStyle(atEnd ? Theme.mutedForeground : Theme.foreground)
        .disabled(atEnd)
        .accessibilityLabel(Text(direction > 0 ? setting.increaseLabel : setting.decreaseLabel))
        .accessibilityIdentifier("reader.\(setting.identifier).\(direction > 0 ? "increase" : "decrease")")
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
    /// Disables the choices and says why, beside the title.
    var disabledReason: LocalizedStringResource?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(title)
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                if let disabledReason {
                    Text(disabledReason)
                        .font(.geist(.caption))
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(.isHeader)
            // One row, as the web's, or one choice a row where the labels
            // don't fit, as at large text sizes.
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 8) { buttons }
                VStack(spacing: 0) { buttons }
            }
            .disabled(disabledReason != nil)
            .opacity(disabledReason != nil ? 0.5 : 1)
        }
    }

    @ViewBuilder
    private var buttons: some View {
        ForEach(options, id: \.self) { option in
            let chosen = option == selection
            Button { selection = option } label: {
                Text(option[keyPath: label])
                    .font(.geist(.footnote, weight: .medium))
                    .lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
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

/// The fonts to read in: the book's own, the bundled ones, and those
/// imported from Files, each name set in its own font, with a way to import
/// another and to remove an imported one.
private struct ReaderFontList: View {
    @Binding var font: ReaderFont
    let done: () -> Void
    private let store = ImportedFontStore.shared
    @State private var importing = false
    @State private var importError: String?
    /// Each font's preview, read from its file once.
    @State private var previews: [ReaderFont: Font] = [:]
    @ScaledMetric(relativeTo: .body) private var previewSize: CGFloat = 17

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                rows([(ReaderFont.book, ReaderFonts.name(of: .book, imported: []))], removable: false)
                section("Included") {
                    rows(BundledFont.allCases.map { (.bundled($0), $0.name) }, removable: false)
                }
                if !store.fonts.isEmpty {
                    section("Imported") {
                        rows(store.fonts.map { (.imported($0.id), $0.name) }, removable: true)
                    }
                }
                VStack(alignment: .leading, spacing: 6) {
                    Button { importing = true } label: {
                        Label("Import Font", icon: .upload)
                            .font(.geist(.subheadline, weight: .medium))
                    }
                    .buttonStyle(OutlineButtonStyle())
                    .accessibilityIdentifier("reader.font.import")
                    Text("TTF, OTF, or WOFF2. Kept on this device only.")
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 8)
            .padding(.bottom, 24)
            .frame(maxWidth: 560, alignment: .leading)
            .frame(maxWidth: .infinity)
        }
        .foregroundStyle(Theme.foreground)
        .background(Theme.background)
        .navigationTitle("Font")
        .navigationBarTitleDisplayMode(.inline)
        .toolbarBackground(Theme.background, for: .navigationBar)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Done", action: done)
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.primary)
                    .accessibilityIdentifier("reader.settings.done")
            }
        }
        .fileImporter(isPresented: $importing, allowedContentTypes: ReaderFontList.fontTypes) { result in
            guard case .success(let url) = result else { return }
            do {
                let imported = try store.importFont(from: url)
                font = .imported(imported.id)
            } catch {
                importError = (error as? FontImportError)?.message ?? String(localized: "The font couldn't be imported.")
            }
        }
        .alert("Couldn't Import the Font", isPresented: Binding(get: { importError != nil }, set: { if !$0 { importError = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(importError ?? "")
        }
        .task { loadPreviews() }
        .onChange(of: store.fonts) { _, _ in loadPreviews() }
        // Drawn again at the new size when the text size changes.
        .onChange(of: previewSize) { _, _ in
            previews = [:]
            loadPreviews()
        }
    }

    /// TTF and OTF are the system's; WOFF2 is the type the app declares
    /// (UTImportedTypeDeclarations in project.yml).
    static let fontTypes: [UTType] = [.font, UTType(importedAs: "org.w3.woff2")]

    private func loadPreviews() {
        var loaded = previews
        let choices = BundledFont.allCases.map(ReaderFont.bundled) + store.fonts.map { .imported($0.id) }
        for choice in choices where loaded[choice] == nil {
            if let preview = ReaderFonts.noteFont(for: choice, store: store) {
                loaded[choice] = preview.font(size: previewSize)
            }
        }
        previews = loaded
    }

    private func section(_ title: LocalizedStringResource, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(.geist(.subheadline, weight: .medium))
                .foregroundStyle(Theme.mutedForeground)
                .accessibilityAddTraits(.isHeader)
            content()
        }
    }

    /// Bordered rows, as the reader's contents draws them.
    private func rows(_ choices: [(ReaderFont, String)], removable: Bool) -> some View {
        VStack(spacing: 0) {
            ForEach(Array(choices.enumerated()), id: \.element.0) { index, choice in
                if index > 0 {
                    Rectangle().fill(Theme.border).frame(height: 1)
                }
                row(choice.0, name: choice.1, removable: removable)
            }
        }
        .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
    }

    private func row(_ choice: ReaderFont, name: String, removable: Bool) -> some View {
        let chosen = choice == font
        return HStack(spacing: 0) {
            Button { font = choice } label: {
                HStack(spacing: 12) {
                    Text(name)
                        .font(previews[choice] ?? .geist(.body))
                        .lineLimit(1)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    if chosen {
                        Icon(.check, size: 18, relativeTo: .body)
                            .foregroundStyle(Theme.primary)
                            .accessibilityHidden(true)
                    }
                }
                .padding(.leading, 12)
                .padding(.trailing, removable ? 4 : 12)
                .padding(.vertical, 10)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(RowPressStyle())
            .accessibilityLabel(Text(name))
            .accessibilityAddTraits(chosen ? .isSelected : [])
            .accessibilityIdentifier("reader.font.\(choice.rawValue)")
            if removable, case .imported(let id) = choice {
                Button {
                    if store.remove(id), chosen { font = .book }
                } label: {
                    Icon(.x, size: 18, relativeTo: .body)
                        .foregroundStyle(Theme.mutedForeground)
                        .frame(minWidth: 44, minHeight: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(PressableStyle())
                .accessibilityLabel(Text("Remove \(name)"))
                .accessibilityIdentifier("reader.font.remove")
            }
        }
        .background(chosen ? Theme.primary.opacity(0.1) : Color.clear)
    }
}
