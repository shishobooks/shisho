import SwiftUI

/// The Library's Filters and Sort buttons, as the web gallery has them over
/// its grid, each opening its sheet.
struct GalleryControls: View {
    @Environment(AppModel.self) private var model
    @State private var showingFilters = false
    @State private var showingSort = false

    var body: some View {
        HStack(spacing: 8) {
            Button { showingFilters = true } label: {
                HStack(spacing: 5) {
                    Image(systemName: "line.3.horizontal.decrease")
                        .accessibilityHidden(true)
                    Text("Filters")
                    if model.filter.count > 0 {
                        Text(verbatim: "\(model.filter.count)")
                            .font(.geist(.caption2, weight: .bold))
                            .foregroundStyle(Theme.primaryForeground)
                            .frame(minWidth: 16, minHeight: 16)
                            .background(Theme.primary, in: Circle())
                    }
                }
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityLabel(model.filter.count > 0 ? String(localized: "Filters, \(model.filter.count) on") : String(localized: "Filters"))
            .accessibilityIdentifier("library.filters")
            Button { showingSort = true } label: {
                Label("Sort", systemImage: "arrow.up.arrow.down")
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityIdentifier("library.sort")
        }
        // Its natural width, so the header stacks it under the heading
        // rather than squeeze its labels onto two lines.
        .fixedSize(horizontal: true, vertical: false)
        .sheet(isPresented: $showingFilters) { FilterSheet() }
        .sheet(isPresented: $showingSort) { SortSheet() }
    }
}

/// Narrows the Library as the web gallery's filters do: by Reading Status,
/// Rating, File Type, genre, tag, and language.
private struct FilterSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var options: FilterOptions?

    private static let statuses = ["currently_reading", "paused", "to_read", "read", "did_not_finish", "reference", "none"]
    /// The least Rating, in quarter stars, from any to four and a half.
    private static let ratings: [Int?] = [nil, 4, 8, 12, 16, 18]

    var body: some View {
        @Bindable var model = model
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    section(String(localized: "Reading status")) {
                        ForEach(Self.statuses, id: \.self) { status in
                            chip(status == "none" ? String(localized: "No status") : ReadingStatusLabel.text(status),
                                 on: model.filter.statuses.contains(status)) {
                                model.filter.statuses.formSymmetricDifference([status])
                            }
                        }
                    }
                    section(String(localized: "Rating")) {
                        ForEach(Self.ratings, id: \.self) { rating in
                            chip(rating.map { String(localized: "\(Ratings.text($0))+ stars") } ?? String(localized: "Any"),
                                 on: model.filter.minRating == rating) {
                                model.filter.minRating = rating
                            }
                        }
                    }
                    if let options {
                        if !options.fileTypes.isEmpty {
                            section(String(localized: "File type")) {
                                ForEach(options.fileTypes, id: \.self) { type in
                                    chip(FileTypes.label(type), on: model.filter.fileTypes.contains(type)) {
                                        model.filter.fileTypes.formSymmetricDifference([type])
                                    }
                                }
                            }
                        }
                        if !options.genres.isEmpty {
                            section(String(localized: "Genres")) {
                                ForEach(options.genres, id: \.self) { genre in
                                    chip(genre, on: model.filter.genres.contains(genre)) {
                                        model.filter.genres.formSymmetricDifference([genre])
                                    }
                                }
                            }
                        }
                        if !options.tags.isEmpty {
                            section(String(localized: "Tags")) {
                                ForEach(options.tags, id: \.self) { tag in
                                    chip(tag, on: model.filter.tags.contains(tag)) {
                                        model.filter.tags.formSymmetricDifference([tag])
                                    }
                                }
                            }
                        }
                        if !options.languages.isEmpty {
                            section(String(localized: "Language")) {
                                ForEach(options.languages, id: \.self) { language in
                                    chip(Locale.current.localizedString(forIdentifier: language) ?? language,
                                         on: model.filter.languages.contains(language)) {
                                        model.filter.languages.formSymmetricDifference([language])
                                    }
                                }
                            }
                        }
                    }
                }
                .padding(20)
            }
            .background(Theme.background)
            .navigationTitle("Filters")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Clear") { model.filter = LibraryFilter() }
                        .disabled(model.filter.isEmpty)
                        .accessibilityIdentifier("filters.clear")
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("filters.done")
                }
            }
        }
        .tint(Theme.primary)
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .observing(model.session?.store, id: "\(model.session?.account.id ?? "")-\(model.libraryID)", into: $options) {
            [libraryID = model.libraryID == 0 ? nil : model.libraryID] db in
            try CatalogStore.filterOptions(db, libraryID: libraryID)
        }
    }

    private func section(_ title: String, @ViewBuilder chips: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title)
                .font(.geist(.subheadline, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            FlowLayout(spacing: 8) { chips() }
        }
    }

    /// A choice that's on or off, as the web's filter chips are.
    private func chip(_ title: String, on: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
                .font(.geist(.footnote, weight: .medium))
                .padding(.horizontal, 12)
                .frame(minHeight: 32)
                .background(on ? Theme.primary : Theme.secondary, in: Capsule())
                .foregroundStyle(on ? Theme.primaryForeground : Theme.foreground)
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(on ? [.isSelected] : [])
        .accessibilityIdentifier("filter.chip")
    }
}

/// Orders the Library by one field, as the web gallery's sort does, or by
/// the sort the user saved for it on the web.
private struct SortSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss

    private static let fields: [(String, String)] = [
        ("date_added", String(localized: "Date added")),
        ("title", String(localized: "Title")),
        ("author", String(localized: "Author")),
        ("series", String(localized: "Series")),
        ("date_released", String(localized: "Release date")),
        ("page_count", String(localized: "Page count")),
        ("duration", String(localized: "Duration")),
        ("reading_status", String(localized: "Reading status")),
        ("rating", String(localized: "Rating")),
        ("date_finished", String(localized: "Date finished")),
    ]

    private var field: String? { model.sortSpec?.split(separator: ":").first.map(String.init) }
    private var descending: Bool { model.sortSpec?.hasSuffix(":desc") ?? true }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    row(String(localized: "Saved sort"), selected: model.sortSpec == nil) { model.sortSpec = nil }
                } footer: {
                    Text("The order you saved for this Library on the web.")
                }
                Section {
                    ForEach(Self.fields, id: \.0) { field, title in
                        row(title, selected: self.field == field) {
                            model.sortSpec = "\(field):\(descending ? "desc" : "asc")"
                        }
                    }
                }
                if let field {
                    Section {
                        Picker("Direction", selection: Binding(
                            get: { descending },
                            set: { model.sortSpec = "\(field):\($0 ? "desc" : "asc")" }
                        )) {
                            Text("Ascending").tag(false)
                            Text("Descending").tag(true)
                        }
                        .pickerStyle(.segmented)
                        .accessibilityIdentifier("sort.direction")
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .background(Theme.background)
            .navigationTitle("Sort")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("sort.done")
                }
            }
        }
        .tint(Theme.primary)
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private func row(_ title: String, selected: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack {
                Text(title)
                    .font(.geist(.body))
                    .foregroundStyle(Theme.foreground)
                Spacer()
                if selected {
                    Image(systemName: "checkmark")
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                }
            }
        }
        .listRowBackground(Theme.card)
        .accessibilityAddTraits(selected ? [.isSelected] : [])
        .accessibilityIdentifier("sort.option")
    }
}
