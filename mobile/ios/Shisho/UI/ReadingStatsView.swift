import Charts
import ShishoAPI
import SwiftUI

/// Reading Stats as the web shows them, for all time or one year: from the
/// copy last fetched, with reading on this device the server hasn't counted
/// yet laid over it, so an hour read on a plane counts at once. Breakdowns
/// by Book, pace, and words read catch up at the next sync.
struct ReadingStatsView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    /// The year shown, or nil for all time.
    @State private var year: Int?
    @State private var stats: ReadingStats??
    @State private var loading = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                if case .some(.some(let shown)) = stats {
                    content(shown)
                } else if case .some(.none) = stats {
                    if loading {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                            .padding(.top, 80)
                    } else {
                        EmptyNotice(title: String(localized: "Reading Stats need a connection"),
                                    message: String(localized: "They show here once the app has synced with your server, and work offline after."),
                                    icon: .chartColumn)
                            .accessibilityIdentifier("stats.unavailable")
                    }
                }
            }
            .padding(.horizontal, GridLayout.margin)
            .padding(.bottom, 32)
            .frame(maxWidth: 900)
            .frame(maxWidth: .infinity)
        }
        .miniPlayerClearance()
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle("Reading Stats")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) { rangePicker }
        }
        .observing(model.session?.store, id: year, into: $stats) { [year, deviceID = model.session?.account.deviceID ?? ""] db in
            try CatalogStore.stats(db, year: year, ownDeviceID: deviceID)
        }
        .task(id: year) {
            // A range never fetched is fetched now, when it can be.
            guard case .some(.none) = stats else { return }
            loading = model.connection == .online
            await model.loadStats(year: year)
            loading = false
        }
    }

    private var years: [Int] {
        var years = (stats ?? nil)?.stats.years ?? []
        let now = Calendar.current.component(.year, from: Date())
        if !years.contains(now) { years.append(now) }
        if let year, !years.contains(year) { years.append(year) }
        return years.sorted(by: >)
    }

    private var rangePicker: some View {
        Menu {
            Picker(String(localized: "Range"), selection: $year) {
                Text("All time").tag(Int?.none)
                ForEach(years, id: \.self) { year in
                    Text(verbatim: String(year)).tag(Int?.some(year))
                }
            }
        } label: {
            HStack(spacing: 4) {
                Text(year.map { String($0) } ?? String(localized: "All time"))
                    .font(.geist(.subheadline, weight: .medium))
                Icon(.chevronDown, size: 12, relativeTo: .subheadline)
                    .accessibilityHidden(true)
            }
            .foregroundStyle(Theme.foreground)
            .frame(minHeight: 44)
        }
        .accessibilityLabel(String(localized: "Range"))
        .accessibilityValue(year.map { String($0) } ?? String(localized: "All time"))
        .accessibilityIdentifier("stats.range")
    }

    @ViewBuilder
    private func content(_ shown: ReadingStats) -> some View {
        let stats = shown.stats
        if shown.unsyncedSeconds > 0 {
            Label(verbatim: String(localized: "Includes \(FileFacts.duration(Double(shown.unsyncedSeconds))) on this device not synced yet"),
                  icon: .cloudUpload, size: 14, relativeTo: .footnote)
                .font(.geist(.footnote))
                .foregroundStyle(Theme.mutedForeground)
                .padding(.top, 4)
                .accessibilityIdentifier("stats.unsynced")
        }
        let columns = Array(repeating: GridItem(.flexible(), spacing: 10), count: sizeClass == .regular ? 3 : 2)
        LazyVGrid(columns: columns, spacing: 10) {
            StatTile(label: String(localized: "Finished"), value: stats.totals.finished.formatted(),
                     detail: stats.totals.rereads > 0 ? String(localized: "\(stats.totals.rereads) rereads") : nil, identifier: "stats.finished")
            StatTile(label: String(localized: "Did Not Finish"), value: stats.totals.didNotFinish.formatted(), identifier: "stats.dnf")
            StatTile(label: String(localized: "Reading time"), value: FileFacts.duration(Double(stats.totals.readingSeconds)),
                     identifier: "stats.reading")
            StatTile(label: String(localized: "Listening time"), value: FileFacts.duration(Double(stats.totals.listeningSeconds)),
                     identifier: "stats.listening")
            StatTile(label: String(localized: "Words read"), value: stats.totals.wordsRead.formatted(.number.notation(.compactName)),
                     identifier: "stats.words")
            StatTile(label: String(localized: "Average rating"),
                     value: stats.totals.averageRating.map { $0.formatted(.number.precision(.fractionLength(0...2))) } ?? "–",
                     identifier: "stats.rating")
        }
        .padding(.top, 12)

        StatsSection(title: String(localized: "Pace")) {
            PaceRow(label: String(localized: "Reading"), value: stats.pace.wordsPerMinute.map { String(localized: "\(Int($0.rounded())) words a minute") })
            PaceRow(label: String(localized: "Comics"), value: stats.pace.comicPagesPerHour.map { String(localized: "\(Int($0.rounded())) pages an hour") })
            PaceRow(label: String(localized: "PDFs"), value: stats.pace.pdfPagesPerHour.map { String(localized: "\(Int($0.rounded())) pages an hour") })
            PaceRow(label: String(localized: "Listening"),
                    value: stats.pace.listeningSpeed.map { String(localized: "\($0.formatted(.number.precision(.fractionLength(1))))× speed") })
        }

        if !stats.periods.isEmpty {
            StatsSection(title: year == nil ? String(localized: "By year") : String(localized: "By month")) {
                PeriodChart(periods: stats.periods, byMonth: year != nil)
            }
        }

        if !stats.places.isEmpty {
            StatsSection(title: String(localized: "Where you read")) {
                ForEach(Array(stats.places.enumerated()), id: \.offset) { _, place in
                    CountRow(name: PlaceName.text(place, ownDeviceID: model.session?.account.deviceID ?? ""),
                             value: FileFacts.duration(Double(place.readingSeconds + place.listeningSeconds)))
                }
            }
            .accessibilityIdentifier("stats.places")
        }
        breakdown(String(localized: "Genres"), stats.genres)
        breakdown(String(localized: "Authors"), stats.authors)
        breakdown(String(localized: "Tags"), stats.tags)
        if !stats.ratings.isEmpty {
            StatsSection(title: String(localized: "Ratings")) {
                ForEach(stats.ratings.sorted { $0.rating > $1.rating }, id: \.rating) { row in
                    CountRow(name: String(localized: "\(row.rating.formatted(.number.precision(.fractionLength(0...2)))) stars"),
                             value: Counts.books(row.books))
                }
            }
        }
        Text("Updated \(shown.fetchedAt.formatted(.relative(presentation: .named)))")
            .font(.geist(.caption))
            .foregroundStyle(Theme.mutedForeground)
            .padding(.top, 20)
    }

    @ViewBuilder
    private func breakdown(_ title: String, _ rows: [Shishoapp.StatsCount]) -> some View {
        if !rows.isEmpty {
            StatsSection(title: title) {
                ForEach(rows.prefix(8), id: \.name) { row in
                    CountRow(name: row.name, value: Counts.books(row.books))
                }
            }
        }
    }
}

private struct StatTile: View {
    let label: String
    let value: String
    var detail: String?
    let identifier: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label)
                .font(.geist(.footnote))
                .foregroundStyle(Theme.mutedForeground)
            Text(value)
                .font(.geist(.title2, weight: .semibold))
                .monospacedDigit()
                .foregroundStyle(Theme.foreground)
            if let detail {
                Text(detail)
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .cardStyle(radius: Theme.radius + 2)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }
}

private struct StatsSection<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title)
                .font(.geist(.headline, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            VStack(alignment: .leading, spacing: 8) {
                content
            }
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .cardStyle(radius: Theme.radius + 2)
        }
        .padding(.top, 24)
    }
}

private struct PaceRow: View {
    let label: String
    let value: String?

    var body: some View {
        CountRow(name: label, value: value ?? String(localized: "Not enough timed reading yet"), muted: value == nil)
    }
}

private struct CountRow: View {
    let name: String
    let value: String
    var muted = false

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(name)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.foreground)
            Spacer(minLength: 12)
            Text(value)
                .font(.geist(.subheadline))
                .monospacedDigit()
                .foregroundStyle(muted ? Theme.mutedForeground : Theme.foreground)
                .multilineTextAlignment(.trailing)
        }
        .frame(minHeight: 28)
        .accessibilityElement(children: .combine)
    }
}

/// Reading and listening time by month or year, as hours.
private struct PeriodChart: View {
    let periods: [Shishoapp.StatsPeriod]
    let byMonth: Bool

    private struct Bar: Identifiable {
        let id: String
        let label: String
        let kind: String
        let hours: Double
    }

    private var bars: [Bar] {
        periods.flatMap { period -> [Bar] in
            let label = byMonth
                ? (period.month.map { Calendar.current.shortMonthSymbols[max(0, min(11, $0 - 1))] } ?? String(period.year))
                : String(period.year)
            return [
                Bar(id: "\(label)-r", label: label, kind: String(localized: "Reading"), hours: Double(period.readingSeconds) / 3600),
                Bar(id: "\(label)-l", label: label, kind: String(localized: "Listening"), hours: Double(period.listeningSeconds) / 3600),
            ]
        }
    }

    var body: some View {
        Chart(bars) { bar in
            BarMark(x: .value("Period", bar.label), y: .value("Hours", bar.hours))
                .foregroundStyle(by: .value("Kind", bar.kind))
        }
        .chartForegroundStyleScale([String(localized: "Reading"): Theme.primary,
                                    String(localized: "Listening"): Theme.primary.opacity(0.45)])
        .frame(height: 180)
        .accessibilityLabel(String(localized: "Reading and listening time by period"))
    }
}

/// Where reading happened, as the web's Reading Stats names each place.
enum PlaceName {
    static func text(_ place: Shishoapp.StatsPlace, ownDeviceID: String) -> String {
        if let name = place.deviceName, !name.isEmpty {
            return place.deviceId == ownDeviceID ? String(localized: "\(name) (this device)") : name
        }
        if place.deviceId == ownDeviceID, !ownDeviceID.isEmpty { return String(localized: "This device") }
        switch place.source {
        case .webReader: return String(localized: "Shisho reader")
        case .kobo: return "Kobo"
        case .koreader: return "KOReader"
        case .shishoApp: return String(localized: "Shisho app")
        case .unknown(let raw): return raw
        }
    }
}
