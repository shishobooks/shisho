import SwiftUI

/// Search inside the Book: a search field, then each result in its
/// sentence, under its chapter. Choosing one closes the sheet and goes
/// there, leaving the search bar over the page to step through the rest.
struct ReaderSearchSheet: View {
    let search: ReaderSearch
    let choose: (Int) -> Void
    /// Next (true) or previous from the keyboard.
    let step: (Bool) -> Void
    let done: () -> Void
    @FocusState private var fieldFocused: Bool
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        NavigationStack {
            ScrollViewReader { scroller in
                ScrollView {
                    // Every row in the one lazy stack, so a word found
                    // thousands of times builds only the rows on screen.
                    LazyVStack(alignment: .leading, spacing: 0) {
                        status
                        ForEach(search.groups) { group in
                            Text(group.chapter.isEmpty ? String(localized: "Untitled section") : group.chapter)
                                .font(.geist(.footnote, weight: .medium))
                                .foregroundStyle(Theme.mutedForeground)
                                .padding(.top, 16)
                                .padding(.bottom, 6)
                                .accessibilityAddTraits(.isHeader)
                            ForEach(group.results, id: \.index) { item in
                                row(item.result, index: item.index)
                                    .overlay(alignment: .top) {
                                        if item.index != group.results.first?.index {
                                            Rectangle().fill(Theme.border).frame(height: 1)
                                        }
                                    }
                                    .id(item.index)
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.bottom, 24)
                    .frame(maxWidth: 680, alignment: .leading)
                    .frame(maxWidth: .infinity)
                }
                .scrollDismissesKeyboard(.immediately)
                // The field stays above the results as they scroll.
                .safeAreaInset(edge: .top, spacing: 0) { field }
                .onAppear {
                    // Back from the page, the chosen result is in view;
                    // otherwise the keyboard comes up with the sheet.
                    if let current = search.current {
                        scroller.scrollTo(current, anchor: .center)
                    } else {
                        fieldFocused = true
                    }
                }
            }
            .foregroundStyle(Theme.foreground)
            .background(Theme.background)
            .navigationTitle("Search")
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(Theme.background, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done", action: done)
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                        .accessibilityIdentifier("reader.search.done")
                }
            }
        }
        .tint(Theme.primary)
        // The reader's shortcuts, which a sheet over it would otherwise
        // take: Command-F returns to the field, Command-G and
        // Shift-Command-G step and show the page, and Escape closes search.
        .background {
            Group {
                Button("Search in Book") { fieldFocused = true }
                    .keyboardShortcut("f", modifiers: .command)
                Button("Next Result") { step(true) }
                    .keyboardShortcut("g", modifiers: .command)
                    .disabled(!search.active)
                Button("Previous Result") { step(false) }
                    .keyboardShortcut("g", modifiers: [.command, .shift])
                    .disabled(!search.active)
                Button("Close Search") {
                    search.close()
                    done()
                }
                .keyboardShortcut(.escape, modifiers: [])
            }
            .frame(width: 0, height: 0)
            .opacity(0)
            .accessibilityHidden(true)
        }
    }

    /// The app's own field rather than the system's searchable one, which
    /// hides the title and Done while it has the keyboard.
    private var field: some View {
        HStack(spacing: 8) {
            Icon(.search, size: 16, relativeTo: .body)
                .foregroundStyle(Theme.mutedForeground)
                .accessibilityHidden(true)
            TextField("Search in book", text: Binding(get: { search.query }, set: { search.changeQuery($0) }))
                .font(.geist(.body))
                .focused($fieldFocused)
                .submitLabel(.search)
                .onSubmit { search.submit() }
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .accessibilityIdentifier("reader.search.field")
            if !search.query.isEmpty {
                Button {
                    search.changeQuery("")
                    fieldFocused = true
                } label: {
                    Icon(.x, size: 16, relativeTo: .body)
                        .foregroundStyle(Theme.mutedForeground)
                        .frame(minWidth: 44, minHeight: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(PressableStyle())
                .accessibilityLabel(Text("Clear search"))
                .accessibilityIdentifier("reader.search.clear")
            }
        }
        .padding(.leading, 12)
        .frame(minHeight: 44)
        .background(Theme.background, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
            .strokeBorder(fieldFocused ? Theme.primary : Theme.input))
        .padding(.horizontal, 16)
        .padding(.top, 4)
        .padding(.bottom, 8)
        .frame(maxWidth: 680)
        .frame(maxWidth: .infinity)
        .background(Theme.background)
    }

    @ViewBuilder
    private var status: some View {
        if let text = search.status {
            HStack(spacing: 8) {
                if search.searching {
                    ProgressView()
                        .controlSize(.small)
                        .accessibilityHidden(true)
                }
                Text(text)
                    .monospacedDigit()
            }
            .font(.geist(.footnote))
            .foregroundStyle(Theme.mutedForeground)
            .padding(.top, 8)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("reader.search.status")
        }
    }

    private func row(_ result: PageMessage.SearchResult, index: Int) -> some View {
        let current = index == search.current
        return Button { choose(index) } label: {
            excerpt(result)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.foreground)
                .multilineTextAlignment(.leading)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 12)
                .padding(.vertical, 10)
                .frame(minHeight: 44)
                .background(current ? Theme.muted : Color.clear)
                .contentShape(Rectangle())
        }
        .buttonStyle(RowPressStyle())
        .accessibilityLabel(Text(verbatim: result.pre + result.match + result.post))
        .accessibilityAddTraits(current ? .isSelected : [])
        .accessibilityIdentifier("reader.search.result")
    }

    /// The match in its sentence, marked as the web's list marks it: Tailwind
    /// yellow-300, or yellow-700 in Dark Mode.
    private func excerpt(_ result: PageMessage.SearchResult) -> Text {
        var match = AttributedString(result.match)
        match.backgroundColor = colorScheme == .dark
            ? Color(red: 161 / 255, green: 98 / 255, blue: 7 / 255)
            : Color(red: 253 / 255, green: 224 / 255, blue: 71 / 255)
        return Text(AttributedString(result.pre) + match + AttributedString(result.post))
    }
}

/// The search over the page once a result is chosen, as a browser's find
/// bar: where the result is among them, back to the list, previous and
/// next, and close. It stays while the reader turns pages.
struct ReaderSearchBar: View {
    let search: ReaderSearch
    let showResults: () -> Void

    var body: some View {
        HStack(spacing: 0) {
            button(.search, label: "Results", identifier: "reader.search.results", action: showResults)
            Text(search.status ?? "")
                .font(.geist(.footnote, weight: .medium))
                .monospacedDigit()
                .foregroundStyle(Theme.foreground)
                .lineLimit(1)
                .frame(maxWidth: .infinity)
                .accessibilityIdentifier("reader.search.position")
            button(.chevronUp, label: "Previous result", identifier: "reader.search.previous", action: search.previous)
            button(.chevronDown, label: "Next result", identifier: "reader.search.next", action: search.next)
            button(.x, label: "Close search", identifier: "reader.search.close", action: search.close)
        }
        .padding(.horizontal, 4)
        .frame(maxWidth: 420)
        .background(Theme.background, in: Capsule())
        .overlay(Capsule().strokeBorder(Theme.border))
        .shadow(color: .black.opacity(0.12), radius: 8, y: 2)
        .padding(.horizontal, 16)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(Text("Search in book"))
    }

    private func button(_ icon: Lucide, label: LocalizedStringKey, identifier: String,
                        action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Icon(icon, size: 18)
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .foregroundStyle(Theme.foreground)
        .accessibilityLabel(Text(label))
        .accessibilityShowsLargeContentViewer()
        .accessibilityIdentifier(identifier)
    }
}
