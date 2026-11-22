import SwiftUI

/// The Library's Filter and Sort buttons, as the web gallery has them over
/// its grid, each opening its sheet. A dot marks each that's changed.
struct GalleryControls: View {
    @Environment(AppModel.self) private var model
    let sort: GallerySort
    /// The screen's, not these buttons': the heading lays them out twice
    /// to see which fits, and a copy it drops mustn't close an open sheet.
    @Binding var showingFilters: Bool
    @Binding var showingSort: Bool

    var body: some View {
        HStack(spacing: 8) {
            Button { showingFilters = true } label: {
                Label("Filter", systemImage: "line.3.horizontal.decrease")
            }
            .buttonStyle(SmallOutlineStyle())
            .overlay(alignment: .topTrailing) { if !model.filter.isEmpty { ChangedDot() } }
            .accessibilityValue(model.filter.isEmpty ? "" : String(localized: "Filters are on"))
            .accessibilityIdentifier("library.filters")
            Button { showingSort = true } label: {
                Label("Sort", systemImage: "arrow.up.arrow.down")
            }
            .buttonStyle(SmallOutlineStyle())
            .overlay(alignment: .topTrailing) { if sort.isDirty { ChangedDot() } }
            .accessibilityValue(sort.isDirty ? String(localized: "Differs from the saved sort") : "")
            .accessibilityIdentifier("library.sort")
        }
        // Its natural width, so the header stacks it under the heading
        // rather than squeeze its labels onto two lines.
        .fixedSize(horizontal: true, vertical: false)
    }
}

extension View {
    /// The Filter and Sort sheets GalleryControls opens.
    func gallerySheets(showingFilters: Binding<Bool>, showingSort: Binding<Bool>, sort: GallerySort) -> some View {
        sheet(isPresented: showingFilters) { FilterSheet() }
            .sheet(isPresented: showingSort) { SortSheet(sort: sort) }
    }
}

/// The web's dot on a control that's been changed from its default.
private struct ChangedDot: View {
    var body: some View {
        Circle()
            .fill(Theme.primary)
            .frame(width: 8, height: 8)
            .overlay(Circle().strokeBorder(Theme.background, lineWidth: 2).padding(-2))
            .padding(4)
            .accessibilityHidden(true)
    }
}

/// What a filter narrows by, with the icon and color the web marks its
/// section and chips with.
enum FilterKind {
    case readingStatus, rating, fileType, genre, tag, language

    var symbol: String {
        switch self {
        case .readingStatus: "book"
        case .rating: "star"
        case .fileType: "doc"
        case .genre: "bookmark"
        case .tag: "tag"
        case .language: "globe"
        }
    }

    var color: Color {
        switch self {
        case .readingStatus: Theme.chart1
        case .rating: Theme.ratingStrong
        case .fileType, .language: Theme.chart5
        case .genre: Theme.primary
        case .tag: Theme.chart2
        }
    }
}

enum FilterChoices {
    /// The Reading Statuses the filter offers, in the web's order.
    static let statuses = ["currently_reading", "paused", "to_read", "read", "did_not_finish", "reference", "none"]

    static func statusLabel(_ status: String) -> String {
        status == "none" ? String(localized: "No status") : ReadingStatusLabel.text(status)
    }

    static func languageName(_ tag: String) -> String {
        Locale.current.localizedString(forIdentifier: tag) ?? tag
    }
}

/// A filter that's on, which removes itself when tapped, as the web's
/// filter chips do.
struct FilterChip: View {
    let kind: FilterKind
    let label: String
    let remove: () -> Void

    var body: some View {
        Button(action: remove) {
            HStack(spacing: 6) {
                Image(systemName: kind.symbol)
                    .font(.geist(size: 10, relativeTo: .footnote, weight: .semibold))
                    .foregroundStyle(kind.color)
                Text(label)
                    .font(.geist(.footnote, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(1)
                Image(systemName: "xmark")
                    .font(.geist(size: 9, relativeTo: .footnote, weight: .bold))
                    .foregroundStyle(Theme.mutedForeground)
            }
            .padding(.horizontal, 9)
            .padding(.vertical, 5)
            .background(Theme.secondary, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
            .hitArea(height: 28)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(String(localized: "Remove \(label)"))
        .accessibilityIdentifier("filter.chip")
    }
}

/// The filters that are on, under the Library's heading, as the web's
/// "Filtering by" row has them.
struct ActiveFilterChips: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        if !model.filter.isEmpty {
            FlowLayout(spacing: 8) {
                Text("Filtering by")
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                ForEach(FilterChoices.statuses.filter(model.filter.statuses.contains), id: \.self) { status in
                    FilterChip(kind: .readingStatus, label: FilterChoices.statusLabel(status)) {
                        model.filter.statuses.remove(status)
                    }
                }
                if model.filter.minRating != nil || model.filter.maxRating != nil {
                    FilterChip(kind: .rating, label: Ratings.range(min: model.filter.minRating, max: model.filter.maxRating)) {
                        model.filter.minRating = nil
                        model.filter.maxRating = nil
                    }
                }
                ForEach(FileTypes.sorted(model.filter.fileTypes), id: \.self) { type in
                    FilterChip(kind: .fileType, label: FileTypes.label(type)) { model.filter.fileTypes.remove(type) }
                }
                ForEach(model.filter.genres.sorted(by: Self.byName), id: \.self) { genre in
                    FilterChip(kind: .genre, label: genre) { model.filter.genres.remove(genre) }
                }
                ForEach(model.filter.tags.sorted(by: Self.byName), id: \.self) { tag in
                    FilterChip(kind: .tag, label: tag) { model.filter.tags.remove(tag) }
                }
                if let language = model.filter.language {
                    FilterChip(kind: .language, label: FilterChoices.languageName(language)) { model.filter.language = nil }
                }
                Button("clear all") { model.filter = LibraryFilter() }
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                    .buttonStyle(.plain)
                    .hitArea(height: 16)
                    .accessibilityLabel("Clear all filters")
                    .accessibilityIdentifier("library.filters.clearAll")
            }
        }
    }

    static func byName(_ a: String, _ b: String) -> Bool {
        a.localizedStandardCompare(b) == .orderedAscending
    }
}

/// The sort picked on this device when it isn't the saved one, under the
/// Library's heading, as the web's "Sorted by" row has it.
struct SortedByChips: View {
    @Environment(AppModel.self) private var model
    let sort: GallerySort

    var body: some View {
        if sort.isDirty {
            let levels = sort.levels
            FlowLayout(spacing: 8) {
                Text("Sorted by:")
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                ForEach(Array(levels.enumerated()), id: \.element.field) { index, level in
                    Button {
                        var next = levels
                        next.remove(at: index)
                        model.sortSpec = sort.deviceSpec(for: next)
                    } label: {
                        HStack(spacing: 4) {
                            Text(LibrarySort.label(level.field))
                                .font(.geist(.footnote, weight: .medium))
                            Image(systemName: level.descending ? "arrow.down" : "arrow.up")
                                .font(.geist(size: 10, relativeTo: .footnote, weight: .semibold))
                            Image(systemName: "xmark")
                                .font(.geist(size: 9, relativeTo: .footnote, weight: .bold))
                                .foregroundStyle(Theme.mutedForeground)
                        }
                        .foregroundStyle(Theme.foreground)
                        .padding(.horizontal, 9)
                        .padding(.vertical, 5)
                        .background(Theme.secondary, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                        .hitArea(height: 28)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("library.sortedBy.\(level.field)")
                    .accessibilityLabel(String(localized: "\(LibrarySort.label(level.field)) \(level.descending ? String(localized: "descending") : String(localized: "ascending")), remove"))
                }
                Button("reset to default") { model.sortSpec = nil }
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                    .buttonStyle(.plain)
                    .hitArea(height: 16)
                    .accessibilityLabel("Reset to the saved sort")
                    .accessibilityIdentifier("library.sort.reset")
            }
        }
    }
}

// MARK: - Sheets

/// A sheet's heading as the web's drawers have it: the title on the left,
/// its actions on the right.
private struct SheetHeading<Actions: View>: View {
    let title: LocalizedStringKey
    @ViewBuilder let actions: Actions

    var body: some View {
        HStack(spacing: 14) {
            Text(title)
                .font(.geist(.title3, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            Spacer(minLength: 8)
            actions
        }
        .padding(.horizontal, 20)
        .padding(.top, 24)
        .padding(.bottom, 12)
    }
}

/// The small primary button, for a sheet's Done.
private struct SmallPrimaryStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.geist(.footnote, weight: .semibold))
            .foregroundStyle(Theme.primaryForeground)
            .padding(.horizontal, 14)
            .frame(minHeight: 34)
            .background(Theme.primary.opacity(configuration.isPressed ? 0.8 : 1), in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
            .hitArea(height: 34)
    }
}

/// A section of the filters, headed as the web's are: a tinted icon, the
/// name, and how many are picked.
private struct FilterSection<Content: View>: View {
    let kind: FilterKind
    let title: LocalizedStringKey
    var detail: String?
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: kind.symbol)
                    .font(.geist(size: 11, relativeTo: .footnote, weight: .semibold))
                    .foregroundStyle(kind.color)
                    .frame(width: 22, height: 22)
                    .background(kind.color.opacity(0.2), in: RoundedRectangle(cornerRadius: 5, style: .continuous))
                    .accessibilityHidden(true)
                Text(title)
                    .font(.geist(.footnote, weight: .semibold))
                    .accessibilityAddTraits(.isHeader)
                Spacer(minLength: 0)
                if let detail {
                    Text(detail)
                        .font(.geist(.caption2))
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            content
        }
    }
}

/// What a picker shows closed, as the web's select and combobox triggers:
/// the choice, or a muted placeholder, and an up-down chevron.
private struct TriggerLabel: View {
    let text: String
    var placeholder = false
    /// Opens a page rather than a menu, so it points the way it goes.
    var pushes = false

    var body: some View {
        HStack(spacing: 8) {
            Text(text)
                .font(.geist(.subheadline))
                .foregroundStyle(placeholder ? Theme.mutedForeground : Theme.foreground)
                .lineLimit(1)
            Spacer(minLength: 0)
            Image(systemName: pushes ? "chevron.right" : "chevron.up.chevron.down")
                .font(.geist(size: 11, relativeTo: .footnote, weight: .medium))
                .foregroundStyle(Theme.mutedForeground)
                .accessibilityHidden(true)
        }
        .padding(.horizontal, 12)
        .frame(maxWidth: .infinity, minHeight: 42)
        .background(Theme.background, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.input))
        .contentShape(Rectangle())
    }
}

/// Narrows the Library as the web gallery's filter sheet does.
private struct FilterSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var options: FilterOptions?

    private enum Picking: Hashable { case genres, tags }

    var body: some View {
        @Bindable var model = model
        NavigationStack {
            VStack(spacing: 0) {
                SheetHeading(title: "Filters") {
                    if !model.filter.isEmpty {
                        Button("Clear all") { model.filter = LibraryFilter() }
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            .buttonStyle(.plain)
                    .hitArea(height: 16)
                            .accessibilityIdentifier("filters.clear")
                    }
                    Button("Done") { dismiss() }
                        .buttonStyle(SmallPrimaryStyle())
                        .accessibilityIdentifier("filters.done")
                }
                ScrollView {
                    VStack(alignment: .leading, spacing: 26) {
                        statusSection
                        FilterSection(kind: .rating, title: "Rating") {
                            RatingRangeSlider(min: $model.filter.minRating, max: $model.filter.maxRating)
                        }
                        if let options {
                            if !options.fileTypes.isEmpty {
                                fileTypeSection(FileTypes.sorted(options.fileTypes))
                            }
                            if !options.genres.isEmpty {
                                namesSection(.genre, title: "Genres", selected: $model.filter.genres,
                                             add: String(localized: "Add genres…"), picking: .genres)
                            }
                            if !options.tags.isEmpty {
                                namesSection(.tag, title: "Tags", selected: $model.filter.tags,
                                             add: String(localized: "Add tags…"), picking: .tags)
                            }
                            if !options.languages.isEmpty {
                                languageSection(options.languages)
                            }
                        }
                    }
                    .padding(.horizontal, 20)
                    .padding(.top, 8)
                    .padding(.bottom, 32)
                }
            }
            .background(Theme.background)
            .toolbar(.hidden, for: .navigationBar)
            .navigationDestination(for: Picking.self) { picking in
                switch picking {
                case .genres:
                    NamePicker(title: "Genres", prompt: String(localized: "Search genres…"), empty: String(localized: "No genres found."),
                               options: options?.genres ?? [], selected: $model.filter.genres)
                case .tags:
                    NamePicker(title: "Tags", prompt: String(localized: "Search tags…"), empty: String(localized: "No tags found."),
                               options: options?.tags ?? [], selected: $model.filter.tags)
                }
            }
        }
        .tint(Theme.primary)
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .presentationBackground(Theme.background)
        .observing(model.session?.store, id: "\(model.session?.account.id ?? "")-\(model.libraryID)", into: $options) {
            [libraryID = model.libraryID == 0 ? nil : model.libraryID] db in
            try CatalogStore.filterOptions(db, libraryID: libraryID)
        }
    }

    private static func selected(_ count: Int) -> String? {
        count > 0 ? String(localized: "\(count) selected") : nil
    }

    private var statusSection: some View {
        @Bindable var model = model
        let chosen = FilterChoices.statuses.filter(model.filter.statuses.contains)
        return FilterSection(kind: .readingStatus, title: "Reading status", detail: Self.selected(chosen.count)) {
            Menu {
                ForEach(FilterChoices.statuses, id: \.self) { status in
                    Toggle(FilterChoices.statusLabel(status), isOn: Binding(
                        get: { model.filter.statuses.contains(status) },
                        set: { on in
                            if on { model.filter.statuses.insert(status) } else { model.filter.statuses.remove(status) }
                        }
                    ))
                }
            } label: {
                TriggerLabel(text: chosen.isEmpty ? String(localized: "Any status") : chosen.map(FilterChoices.statusLabel).joined(separator: ", "),
                             placeholder: chosen.isEmpty)
            }
            .menuActionDismissBehavior(.disabled)
            .accessibilityLabel("Reading status")
            .accessibilityValue(chosen.isEmpty ? String(localized: "Any status") : chosen.map(FilterChoices.statusLabel).joined(separator: ", "))
            .accessibilityIdentifier("filters.status")
        }
    }

    private func fileTypeSection(_ types: [String]) -> some View {
        @Bindable var model = model
        let chosen = types.filter(model.filter.fileTypes.contains)
        // "2 of 5", as the web counts them.
        return FilterSection(kind: .fileType, title: "File type",
                             detail: chosen.isEmpty ? nil : String(localized: "\(chosen.count) of \(types.count)")) {
            Menu {
                ForEach(types, id: \.self) { type in
                    Toggle(FileTypes.label(type), isOn: Binding(
                        get: { model.filter.fileTypes.contains(type) },
                        set: { on in
                            if on { model.filter.fileTypes.insert(type) } else { model.filter.fileTypes.remove(type) }
                        }
                    ))
                }
            } label: {
                TriggerLabel(text: chosen.isEmpty ? String(localized: "Any file type") : chosen.map(FileTypes.label).joined(separator: ", "),
                             placeholder: chosen.isEmpty)
            }
            .menuActionDismissBehavior(.disabled)
            .accessibilityLabel("File type")
            .accessibilityValue(chosen.isEmpty ? String(localized: "Any file type") : chosen.map(FileTypes.label).joined(separator: ", "))
            .accessibilityIdentifier("filters.fileType")
        }
    }

    /// Genres or tags: the ones picked as chips, and a button to a
    /// searchable list of the rest, since a Library can have thousands. The
    /// web's switch between tag namespaces waits for the device to have the
    /// Library's namespaces; until then every tag is listed by its full name.
    private func namesSection(_ kind: FilterKind, title: LocalizedStringKey, selected: Binding<Set<String>>,
                              add: String, picking: Picking) -> some View {
        FilterSection(kind: kind, title: title, detail: Self.selected(selected.wrappedValue.count)) {
            VStack(alignment: .leading, spacing: 10) {
                if !selected.wrappedValue.isEmpty {
                    FlowLayout(spacing: 8) {
                        ForEach(selected.wrappedValue.sorted(by: ActiveFilterChips.byName), id: \.self) { name in
                            FilterChip(kind: kind, label: name) { selected.wrappedValue.remove(name) }
                        }
                    }
                }
                NavigationLink(value: picking) {
                    TriggerLabel(text: add, placeholder: true, pushes: true)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier(picking == .genres ? "filters.genres" : "filters.tags")
            }
        }
    }

    private func languageSection(_ languages: [String]) -> some View {
        @Bindable var model = model
        return FilterSection(kind: .language, title: "Language") {
            Menu {
                Picker("Language", selection: $model.filter.language) {
                    Text("All Languages").tag(String?.none)
                    ForEach(languages, id: \.self) { language in
                        Text(FilterChoices.languageName(language)).tag(String?.some(language))
                    }
                }
            } label: {
                TriggerLabel(text: model.filter.language.map(FilterChoices.languageName) ?? String(localized: "All Languages"))
            }
            .accessibilityLabel("Language")
            .accessibilityValue(model.filter.language.map(FilterChoices.languageName) ?? String(localized: "All Languages"))
            .accessibilityIdentifier("filters.language")
        }
    }
}

/// Every genre or tag in the Library to search and check off, as the web's
/// combobox has them, but most used first rather than by name, which suits a
/// list of thousands on a phone.
private struct NamePicker: View {
    let title: LocalizedStringKey
    let prompt: String
    let empty: String
    let options: [FilterOption]
    @Binding var selected: Set<String>
    @State private var query = ""

    private var shown: [FilterOption] {
        let query = query.trimmingCharacters(in: .whitespaces)
        return query.isEmpty ? options : options.filter { $0.name.localizedStandardContains(query) }
    }

    var body: some View {
        List(shown) { option in
            // Ignoring case, as the filter matches names, so a pick spelled
            // differently in another Library still shows as on.
            let picked = selected.filter { $0.caseInsensitiveCompare(option.name) == .orderedSame }
            let on = !picked.isEmpty
            Button {
                if on { selected.subtract(picked) } else { selected.insert(option.name) }
            } label: {
                HStack(spacing: 12) {
                    Image(systemName: on ? "checkmark.square.fill" : "square")
                        .font(.geist(size: 17, relativeTo: .body))
                        .foregroundStyle(on ? Theme.primary : Theme.mutedForeground)
                    Text(option.name)
                        .font(.geist(.body))
                        .foregroundStyle(Theme.foreground)
                    Spacer(minLength: 8)
                    Text(verbatim: "\(option.count)")
                        .font(.geist(.caption))
                        .monospacedDigit()
                        .foregroundStyle(Theme.mutedForeground)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .listRowBackground(Theme.background)
            .listRowSeparatorTint(Theme.border)
            .accessibilityLabel(String(localized: "\(option.name), \(Counts.books(option.count))"))
            .accessibilityAddTraits(on ? [.isSelected] : [])
            .accessibilityIdentifier("filters.option")
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .background(Theme.background)
        .overlay {
            if shown.isEmpty {
                Text(empty)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar(.visible, for: .navigationBar)
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: prompt)
    }
}

/// A two-thumbed slider over Ratings from none to five stars in quarter
/// stars, as the web's Rating filter. A thumb at its end of the track
/// leaves that end open, so the whole track is any Rating, unrated Books
/// too. The range is set when a thumb is let go.
private struct RatingRangeSlider: View {
    @Binding var min: Int?
    @Binding var max: Int?
    /// The ends while a thumb is dragged, before they're set.
    @State private var dragging: (low: Int, high: Int)?
    /// Which thumb a drag moves: the low one when true.
    @State private var movingLow: Bool?

    private static let ceiling = 20
    private static let thumb: CGFloat = 26

    private var ends: (low: Int, high: Int) { dragging ?? (min ?? 0, max ?? Self.ceiling) }

    /// The range ends as the filter keeps them: nil at the track's ends,
    /// and a top of at least the lowest Rating there is.
    private static func range(_ low: Int, _ high: Int) -> (min: Int?, max: Int?) {
        (low == 0 ? nil : low, high == ceiling ? nil : Swift.max(high, 1))
    }

    var body: some View {
        let shown = Self.range(ends.low, ends.high)
        VStack(spacing: 6) {
            Text(Ratings.range(min: shown.min, max: shown.max))
                .font(.geist(.caption))
                .monospacedDigit()
                .foregroundStyle(Theme.mutedForeground)
                .frame(maxWidth: .infinity, alignment: .trailing)
                .accessibilityHidden(true)
            GeometryReader { geometry in
                let span = geometry.size.width - Self.thumb
                let x = { (value: Int) in Self.thumb / 2 + span * CGFloat(value) / CGFloat(Self.ceiling) }
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(Theme.rating.opacity(0.2))
                        .frame(height: 6)
                        .padding(.horizontal, Self.thumb / 2)
                    Capsule()
                        .fill(Theme.rating)
                        .frame(width: x(ends.high) - x(ends.low), height: 6)
                        .offset(x: x(ends.low))
                    thumb(low: true, at: x(ends.low))
                    thumb(low: false, at: x(ends.high))
                }
                .frame(maxHeight: .infinity)
                .contentShape(Rectangle())
                .gesture(DragGesture(minimumDistance: 0).onChanged { drag in
                    let value = Swift.min(Self.ceiling, Swift.max(0, Int(((drag.location.x - Self.thumb / 2) / span * CGFloat(Self.ceiling)).rounded())))
                    var (low, high) = ends
                    if movingLow == nil {
                        let toLow = abs(drag.location.x - x(low)), toHigh = abs(drag.location.x - x(high))
                        if toLow != toHigh {
                            movingLow = toLow < toHigh
                        } else if drag.translation.width != 0 {
                            // Thumbs on top of each other part the way the
                            // drag goes.
                            movingLow = drag.translation.width < 0
                        } else {
                            return
                        }
                    }
                    if movingLow == true { low = Swift.min(value, high) } else { high = Swift.max(value, low) }
                    dragging = (low, high)
                } .onEnded { _ in
                    if let dragging { commit(dragging.low, dragging.high) }
                    dragging = nil
                    movingLow = nil
                })
            }
            .frame(height: 44)
            HStack {
                Text("Any")
                Spacer()
                Text("Any")
            }
            .font(.geist(.caption2))
            .foregroundStyle(Theme.mutedForeground)
            .accessibilityHidden(true)
        }
        .sensoryFeedback(.selection, trigger: dragging.map { $0.low * 100 + $0.high })
    }

    private func commit(_ low: Int, _ high: Int) {
        let range = Self.range(low, high)
        min = range.min
        max = range.max
    }

    private func thumb(low: Bool, at x: CGFloat) -> some View {
        let value = low ? ends.low : ends.high
        let edge = low ? 0 : Self.ceiling
        return Circle()
            .fill(Theme.background)
            .overlay(Circle().strokeBorder(Theme.ratingStrong, lineWidth: 1.5))
            .frame(width: Self.thumb, height: Self.thumb)
            .shadow(color: .black.opacity(0.15), radius: 2, y: 1)
            // The 44-point target Apple asks for, around the smaller thumb.
            .frame(width: 44, height: 44)
            .contentShape(Rectangle())
            .offset(x: x - 22)
            .accessibilityElement()
            .accessibilityLabel(low ? String(localized: "Lowest rating") : String(localized: "Highest rating"))
            .accessibilityValue(value == edge ? String(localized: "Any") : Ratings.range(min: value, max: value))
            .accessibilityAdjustableAction { direction in
                let step = direction == .increment ? 1 : -1
                if low {
                    commit(Swift.min(ends.high, Swift.max(0, ends.low + step)), ends.high)
                } else {
                    commit(ends.low, Swift.min(Self.ceiling, Swift.max(ends.low, ends.high + step)))
                }
            }
            .accessibilityIdentifier(low ? "filters.rating.low" : "filters.rating.high")
    }
}

/// Orders the Library as the web gallery's sort sheet does: by one field
/// and then by others, each with its own direction, starting from the sort
/// saved on the web. The order picked stays on this device.
private struct SortSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let sort: GallerySort

    private var levels: [SortLevel] { sort.levels }

    private func apply(_ levels: [SortLevel]) {
        model.sortSpec = sort.deviceSpec(for: levels)
    }

    var body: some View {
        let used = Set(levels.map(\.field))
        let unused = LibrarySort.fields.filter { !used.contains($0) }
        VStack(spacing: 0) {
            SheetHeading(title: "Sort") {
                if sort.isDirty {
                    Button("Reset") { model.sortSpec = nil }
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .buttonStyle(.plain)
                    .hitArea(height: 16)
                        .accessibilityLabel("Reset to the saved sort")
                        .accessibilityIdentifier("sort.reset")
                }
                Button("Done") { dismiss() }
                    .buttonStyle(SmallPrimaryStyle())
                    .accessibilityIdentifier("sort.done")
            }
            List {
                ForEach(Array(levels.enumerated()), id: \.element.field) { index, level in
                    row(level, index: index, used: used)
                        .listRowInsets(EdgeInsets(top: 4, leading: 20, bottom: 4, trailing: 8))
                        .listRowBackground(Theme.background)
                        .listRowSeparator(.hidden)
                }
                .onMove { from, to in
                    var next = levels
                    next.move(fromOffsets: from, toOffset: to)
                    apply(next)
                }
                if !unused.isEmpty, levels.count < LibrarySort.maxLevels {
                    VStack(alignment: .leading, spacing: 10) {
                        Text(levels.isEmpty ? "Sort by…" : "Then by…")
                            .font(.geist(.caption, weight: .semibold))
                            .foregroundStyle(Theme.mutedForeground)
                        FlowLayout(spacing: 8) {
                            ForEach(unused, id: \.self) { field in
                                Button {
                                    apply(levels + [SortLevel(field: field, descending: LibrarySort.initiallyDescending(field))])
                                } label: {
                                    HStack(spacing: 5) {
                                        Image(systemName: "plus")
                                            .font(.geist(size: 11, relativeTo: .footnote, weight: .semibold))
                                        Text(LibrarySort.label(field))
                                    }
                                    .fixedSize()
                                }
                                .buttonStyle(SmallOutlineStyle())
                                .fixedSize()
                                .accessibilityLabel(String(localized: "Then by \(LibrarySort.label(field))"))
                                .accessibilityIdentifier("sort.add")
                            }
                        }
                    }
                    .padding(.top, 14)
                    .listRowInsets(EdgeInsets(top: 4, leading: 20, bottom: 24, trailing: 20))
                    .listRowBackground(Theme.background)
                    .listRowSeparator(.hidden)
                    .moveDisabled(true)
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            // Always ready to reorder by the handles, as the web's rows are.
            .environment(\.editMode, .constant(.active))
        }
        .background(Theme.background)
        .tint(Theme.primary)
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .presentationBackground(Theme.background)
    }

    /// One level, as the web's sort rows: its field, its direction, and a
    /// button to remove it. The list's handle reorders it.
    private func row(_ level: SortLevel, index: Int, used: Set<String>) -> some View {
        let label = LibrarySort.label(level.field)
        return HStack(spacing: 8) {
            Menu {
                Picker(String(localized: "Sort level \(index + 1) field"), selection: Binding(
                    get: { level.field },
                    set: { field in
                        var next = levels
                        next[index].field = field
                        apply(next)
                    }
                )) {
                    ForEach(LibrarySort.fields.filter { $0 == level.field || !used.contains($0) }, id: \.self) { field in
                        Text(LibrarySort.label(field)).tag(field)
                    }
                }
            } label: {
                TriggerLabel(text: label)
            }
            .accessibilityLabel(String(localized: "Sort level \(index + 1) field"))
            .accessibilityValue(label)
            .accessibilityIdentifier("sort.field")
            Button {
                var next = levels
                next[index].descending.toggle()
                apply(next)
            } label: {
                Image(systemName: level.descending ? "arrow.down" : "arrow.up")
                    .font(.geist(size: 15, relativeTo: .footnote, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                    .frame(minWidth: 44, minHeight: 44)
                    .background(Theme.background, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.input))
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(level.descending
                ? String(localized: "\(label), descending")
                : String(localized: "\(label), ascending"))
            .accessibilityHint("Toggles the direction")
            .accessibilityIdentifier("sort.direction")
            Button {
                var next = levels
                next.remove(at: index)
                apply(next)
            } label: {
                Image(systemName: "xmark")
                    .font(.geist(size: 13, relativeTo: .footnote, weight: .semibold))
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(minWidth: 44, minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(String(localized: "Remove \(label) sort level"))
            .accessibilityIdentifier("sort.remove")
        }
        // Names the row, so the list's handle reads "Reorder Date added",
        // and moves it without dragging, for VoiceOver.
        .accessibilityElement(children: .contain)
        .accessibilityLabel(label)
        .accessibilityActions {
            if index > 0 {
                Button(String(localized: "Move up")) {
                    var next = levels
                    next.swapAt(index, index - 1)
                    apply(next)
                }
            }
            if index < levels.count - 1 {
                Button(String(localized: "Move down")) {
                    var next = levels
                    next.swapAt(index, index + 1)
                    apply(next)
                }
            }
        }
    }
}
