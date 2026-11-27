import Charts
import ShishoAPI
import SwiftUI

// The web draws these sections too, in
// app/components/pages/ReadingStatsPage.tsx and
// app/components/library/ReadingStatsSections.tsx, with the same titles,
// notes, and colors. A change to what a section shows or says changes both;
// app/utils/appParity.test.ts fails until it does.

/// Reading Stats as the web shows them, for this year at first, another
/// year, or all time: from the copy last fetched, with reading on this
/// device the server hasn't counted yet laid over it, so an hour read on a
/// plane counts at once. Breakdowns by Book, pace, and words read catch up
/// at the next sync.
struct ReadingStatsView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    /// The year shown, or nil for all time. This year at first, as on the web.
    @State private var year: Int? = Calendar.current.component(.year, from: Date())
    @State private var stats: ReadingStats??
    @State private var loading = false
    @State private var tab = Tab.overview

    enum Tab: Hashable {
        case overview, calendar
    }

    /// The Calendar tab.
    private var calendar: Bool { tab == .calendar }

    var body: some View {
        ScrollViewReader { proxy in
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Picker(String(localized: "View"), selection: $tab) {
                    Text("Overview").tag(Tab.overview)
                    Text("Calendar").tag(Tab.calendar)
                }
                .pickerStyle(.segmented)
                .padding(.top, 8)
                .padding(.bottom, 12)
                .accessibilityIdentifier("stats.tabs")
                Text(calendar ? String(localized: "Your reading, day by day. Only you can see this.")
                    : String(localized: "Your reading for \(rangeLabel). Only you can see this."))
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .padding(.top, 4)
                if calendar {
                    ReadingCalendarView()
                } else {
                    overview
                }
            }
            .padding(.horizontal, GridLayout.margin)
            .padding(.bottom, 32)
            .frame(maxWidth: 900)
            .frame(maxWidth: .infinity)
        }
        .environment(\.revealCard, CardRevealer(proxy: proxy))
        }
        .miniPlayerClearance()
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle("Reading Stats")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            // The range is the Overview's; the Calendar has its own pickers.
            if !calendar {
                ToolbarItem(placement: .topBarTrailing) { rangePicker }
            }
        }
        .observing(model.session?.store, id: year, into: $stats) { [year, deviceID = model.session?.account.deviceID ?? ""] db in
            try CatalogStore.stats(db, year: year, ownDeviceID: deviceID)
        }
        .task(id: year) {
            // A range never fetched is fetched now, when it can be. The
            // range is checked here, since the page may still show the
            // last one's.
            let cached = try? await model.session?.store.pool.read { [year] db in
                try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM stats_cache WHERE range_key = ?)", arguments: [year ?? 0])
            }
            guard cached != true else { return }
            loading = model.connection == .online
            await model.loadStats(year: year)
            loading = false
        }
    }

    @ViewBuilder
    private var overview: some View {
        if case .some(.some(let shown)) = stats {
            if StatsText.hasActivity(shown.stats) {
                content(shown)
            } else {
                EmptyNotice(title: year.map { String(localized: "No reading in \(String($0)) yet") } ?? String(localized: "No stats yet"),
                            message: String(localized: "Reading Stats chart the books you finish and the time you spend reading, in Shisho's readers and on your synced devices. They fill in as you read."),
                            icon: .chartColumn)
                    .accessibilityIdentifier("stats.empty")
            }
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

    private var rangeLabel: String {
        year.map { String($0) } ?? String(localized: "all time")
    }

    private var years: [Int] {
        var years = (stats ?? nil)?.stats.years ?? []
        let now = Calendar.current.component(.year, from: Date())
        if !years.contains(now) { years.append(now) }
        if let year, !years.contains(year) { years.append(year) }
        return years.sorted(by: >)
    }

    /// The years, newest first, then all time, as the web's range picker
    /// lists them.
    private var rangePicker: some View {
        Menu {
            Picker(String(localized: "Range"), selection: $year) {
                ForEach(years, id: \.self) { year in
                    Text(verbatim: String(year)).tag(Int?.some(year))
                }
                Text("All time").tag(Int?.none)
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
                .padding(.top, 8)
                .accessibilityIdentifier("stats.unsynced")
        }
        StatTiles(totals: stats.totals, columns: sizeClass == .regular ? 3 : 2)
            .padding(.top, 16)
        charts(stats)
        StatsCard(title: String(localized: "Pace")) {
            PaceList(pace: stats.pace)
        }
        StatsCard(title: String(localized: "Where you read")) {
            PlaceList(places: StatsText.places(stats.places, ownDeviceID: model.session?.account.deviceID ?? ""))
        }
        .accessibilityIdentifier("stats.places")
        breakdowns(stats)
        Text("Updated \(shown.fetchedAt.formatted(.relative(presentation: .named)))")
            .font(.geist(.caption))
            .foregroundStyle(Theme.mutedForeground)
            .padding(.top, 20)
    }

    @ViewBuilder
    private func charts(_ stats: Shishoapp.StatsResponse) -> some View {
        if !stats.periods.isEmpty {
            StatsCard(title: year == nil ? String(localized: "Books finished by year") : String(localized: "Books finished by month")) {
                FinishedChart(periods: stats.periods)
            }
            .accessibilityIdentifier("stats.finishedChart")
            StatsCard(title: year == nil ? String(localized: "Reading and listening by year")
                : String(localized: "Reading and listening by month")) {
                TimeChart(periods: stats.periods)
            }
            .accessibilityIdentifier("stats.timeChart")
        }
    }

    @ViewBuilder
    private func breakdowns(_ stats: Shishoapp.StatsResponse) -> some View {
        StatsCard(title: String(localized: "Genres")) {
            CountList(books: stats.totals.finishedBooks, rows: stats.genres.map { CountList.Row(label: $0.name, books: $0.books, opens: .statsFinished(.genre($0.name), year: year)) },
                      noun: StatsText.Noun(one: String(localized: "genre"), many: String(localized: "genres")),
                      emptyMessage: String(localized: "No finished books with genres."), identifier: "stats.genres")
        }
        StatsCard(title: String(localized: "Tags")) {
            CountList(books: stats.totals.finishedBooks, rows: stats.tags.map { CountList.Row(label: $0.name, books: $0.books, opens: .statsFinished(.tag($0.name), year: year)) },
                      noun: StatsText.Noun(one: String(localized: "tag"), many: String(localized: "tags")),
                      emptyMessage: String(localized: "No finished books with tags."), identifier: "stats.tags")
        }
        StatsCard(title: String(localized: "Authors")) {
            CountList(books: stats.totals.finishedBooks, rows: stats.authors.map { CountList.Row(label: $0.name, books: $0.books, opens: .statsFinished(.author($0.name), year: year)) },
                      noun: StatsText.Noun(one: String(localized: "author"), many: String(localized: "authors")),
                      emptyMessage: String(localized: "No finished books with authors."), identifier: "stats.authors")
        }
        StatsCard(title: String(localized: "Kind")) {
            CountList(books: stats.totals.finishedBooks, rows: stats.kinds.map { CountList.Row(label: StatsText.kind($0.kind), books: $0.books) },
                      noun: StatsText.Noun(one: String(localized: "kind"), many: String(localized: "kinds")),
                      emptyMessage: String(localized: "No finished books."), identifier: "stats.kinds")
        }
        StatsCard(title: String(localized: "Ratings")) {
            CountList(books: stats.totals.finishedBooks, rows: stats.ratings.sorted { $0.rating > $1.rating }.map {
                CountList.Row(label: StatsText.stars($0.rating), books: $0.books, opens: .statsFinished(.rating($0.rating), year: year))
            },
                      noun: StatsText.Noun(one: String(localized: "rating"), many: String(localized: "ratings")),
                      emptyMessage: String(localized: "No finished books are rated."), identifier: "stats.ratings")
        }
    }
}

extension EnvironmentValues {
    /// Scrolls Reading Stats so the view with id shows from its top, as a
    /// breakdown's search does to show its matches above the keyboard.
    @Entry var revealCard: CardRevealer?
}

/// Scrolls Reading Stats to a card. A value rather than a closure, so the
/// views reading it aren't redrawn each time the screen is: a closure is
/// never equal to the last one.
struct CardRevealer: Equatable {
    let proxy: ScrollViewProxy

    func reveal(_ id: String) {
        withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(id, anchor: .top) }
    }

    /// The screen's one scroll view's, for as long as the screen shows.
    static func == (lhs: CardRevealer, rhs: CardRevealer) -> Bool { true }
}

/// The colors every chart and bar is drawn in, the web's `--chart-series-1`
/// to `-3` in `app/index.css`: reading in the first, purple, and listening in
/// the second, magenta. `app/utils/appParity.test.ts` checks the asset
/// catalog's copies match the web's.
enum StatsColors {
    static let reading = Color("ChartSeries1")
    static let listening = Color("ChartSeries2")
}

// MARK: - Tiles

/// The range's summary figures, as the web's tiles show them.
private struct StatTiles: View {
    let totals: Shishoapp.StatsTotals
    let columns: Int

    var body: some View {
        LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 10), count: columns), spacing: 10) {
            StatTile(label: String(localized: "Books finished"), value: totals.finished.formatted(),
                     note: totals.rereads > 0 ? StatsText.rereads(totals.rereads) : nil, identifier: "stats.finished")
            StatTile(label: String(localized: "Did Not Finish"), value: totals.didNotFinish.formatted(), identifier: "stats.dnf")
            StatTile(label: String(localized: "Reading time"), value: StatsText.totalDuration(totals.readingSeconds),
                     note: StatsText.totalHours(totals.readingSeconds), identifier: "stats.reading")
            StatTile(label: String(localized: "Listening time"), value: StatsText.totalDuration(totals.listeningSeconds),
                     note: StatsText.totalHours(totals.listeningSeconds), identifier: "stats.listening")
            StatTile(label: String(localized: "Words read"), value: StatsText.compact(totals.wordsRead),
                     note: wordsNote, identifier: "stats.words")
            AverageRatingTile(stars: totals.averageRating)
        }
    }

    /// The exact count, when the tile shortened it.
    private var wordsNote: String? {
        StatsText.compact(totals.wordsRead) == totals.wordsRead.formatted()
            ? nil : String(localized: "\(totals.wordsRead.formatted()) words")
    }
}

private struct StatTile: View {
    let label: String
    let value: String
    var note: String?
    let identifier: String

    var body: some View {
        StatTileFrame(label: label, note: note) {
            Text(value)
        }
        .accessibilityIdentifier(identifier)
    }
}

private struct AverageRatingTile: View {
    let stars: Double?

    var body: some View {
        StatTileFrame(label: String(localized: "Average Rating"), note: nil) {
            if let stars {
                HStack(spacing: 4) {
                    Text(verbatim: stars.formatted(.number.precision(.fractionLength(0...2))))
                    Icon(.starFill, size: 18, relativeTo: .title2)
                        .foregroundStyle(Theme.rating)
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(String(localized: "Rated \(StatsText.stars(stars))"))
            } else {
                Text("None")
            }
        }
        .accessibilityIdentifier("stats.rating")
    }
}

private struct StatTileFrame<Value: View>: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let label: String
    let note: String?
    @ViewBuilder let value: Value

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label)
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
            value
                .font(.geist(.title2, weight: .semibold))
                .monospacedDigit()
                .foregroundStyle(Theme.foreground)
                // A second line at the largest text sizes, where "4d 11h 40m"
                // doesn't fit a tile even shrunk.
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                .minimumScaleFactor(0.7)
            if let note {
                Text(note)
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .frame(maxWidth: .infinity, minHeight: 64, alignment: .topLeading)
        .padding(12)
        .cardStyle(radius: Theme.radius + 2)
        .accessibilityElement(children: .combine)
    }
}

// MARK: - Cards

/// A titled card, as the web's `StatsCard`.
struct StatsCard<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
                .accessibilityAddTraits(.isHeader)
            content
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .cardStyle(radius: Theme.radius + 2)
        .padding(.top, 16)
    }
}

/// Reading and listening, as the web's legend names them.
private struct StatsLegend: View {
    var body: some View {
        HStack(spacing: 12) {
            swatch(StatsColors.reading, String(localized: "Reading"))
            swatch(StatsColors.listening, String(localized: "Listening"))
        }
        .font(.geist(.caption))
        .foregroundStyle(Theme.mutedForeground)
        .accessibilityHidden(true)
    }

    private func swatch(_ color: Color, _ label: String) -> some View {
        HStack(spacing: 4) {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 8, height: 8)
            Text(label)
        }
    }
}

private struct EmptyNote: View {
    let text: String

    var body: some View {
        Text(text)
            .font(.geist(.subheadline))
            .foregroundStyle(Theme.mutedForeground)
    }
}

/// A track under a figure, filled to its share of the largest.
private struct ShareBar: View {
    struct Segment {
        /// Its share of the track, 0 to 1.
        let share: Double
        let color: Color
    }

    let segments: [Segment]

    var body: some View {
        GeometryReader { geometry in
            HStack(spacing: 0) {
                ForEach(Array(segments.enumerated()), id: \.offset) { _, segment in
                    Rectangle()
                        .fill(segment.color)
                        .frame(width: geometry.size.width * max(0, min(1, segment.share)))
                }
                Spacer(minLength: 0)
            }
            .background(Theme.secondary)
            .clipShape(Capsule())
        }
        .frame(height: 6)
        .accessibilityHidden(true)
    }
}

// MARK: - Charts

/// The range's total above a chart. It stays put while the chart is
/// scrubbed; each period's own figures show in a tooltip over the chart.
private struct ChartTotal<Detail: View>: View {
    let heading: String
    let hint: String
    @ViewBuilder let detail: Detail

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(heading)
                .font(.geist(.title3, weight: .semibold))
                .monospacedDigit()
                .foregroundStyle(Theme.foreground)
            detail
            Text(hint)
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}

/// How a chart's total says it can be scrubbed.
private func chartHint(byMonth: Bool) -> String {
    byMonth ? String(localized: "Press and hold the chart to see each month.")
        : String(localized: "Press and hold the chart to see each year.")
}

extension View {
    /// Picks the period under the finger while it's held on the chart and
    /// moved across it, and lets go as it lifts, as hovering does on the
    /// web. A swipe that doesn't hold first still scrolls the page.
    fileprivate func scrubs(_ selected: Binding<String?>) -> some View {
        chartOverlay { proxy in
            GeometryReader { geometry in
                HoldToScrub { point in
                    guard let point, let plot = proxy.plotFrame else {
                        selected.wrappedValue = nil
                        return
                    }
                    let x = point.x - geometry[plot].origin.x
                    if let label: String = proxy.value(atX: x), label != selected.wrappedValue {
                        selected.wrappedValue = label
                        UISelectionFeedbackGenerator().selectionChanged()
                    }
                }
            }
        }
    }
}

/// A press and hold that reports where the finger is until it lifts (nil),
/// holding the page still meanwhile. UIKit's, since SwiftUI's own drag
/// inside a ScrollView keeps a swipe that starts on it from scrolling.
private struct HoldToScrub: UIViewRepresentable {
    let changed: (CGPoint?) -> Void

    func makeUIView(context: Context) -> UIView {
        let view = UIView()
        view.backgroundColor = .clear
        let hold = UILongPressGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.held(_:)))
        hold.minimumPressDuration = 0.2
        view.addGestureRecognizer(hold)
        return view
    }

    func updateUIView(_ view: UIView, context: Context) {
        context.coordinator.changed = changed
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(changed: changed)
    }

    @MainActor
    final class Coordinator: NSObject {
        var changed: (CGPoint?) -> Void
        private weak var page: UIScrollView?

        init(changed: @escaping (CGPoint?) -> Void) {
            self.changed = changed
        }

        @objc func held(_ hold: UILongPressGestureRecognizer) {
            switch hold.state {
            case .began:
                // The page stays put while the finger scrubs.
                var view = hold.view?.superview
                while let current = view, !(current is UIScrollView) { view = current.superview }
                page = view as? UIScrollView
                page?.panGestureRecognizer.isEnabled = false
                changed(hold.location(in: hold.view))
            case .changed:
                changed(hold.location(in: hold.view))
            default:
                page?.panGestureRecognizer.isEnabled = true
                changed(nil)
            }
        }
    }
}

/// An invisible mark at a period's column that holds its tooltip beside
/// the column, at the top of the plot, on the side with more room, as the
/// web's, so it doesn't cover the bar held. Drawn after the bars so it sits
/// over them.
private func tooltipAnchor<Tooltip: View>(_ label: String, in periods: [Shishoapp.StatsPeriod], top: Double,
                                          @ViewBuilder _ tooltip: @escaping () -> Tooltip) -> some ChartContent {
    let index = periods.firstIndex { StatsText.periodLabel($0) == label } ?? 0
    return RectangleMark(x: .value("Period", label), yStart: .value("Value", 0.0), yEnd: .value("Value", top))
        .foregroundStyle(.clear)
        .annotation(position: index * 2 < periods.count ? .trailing : .leading, alignment: .top, spacing: 2,
                    overflowResolution: .init(x: .fit(to: .chart), y: .fit(to: .chart)), content: tooltip)
}

/// A tooltip over a chart, as the web's: the period's figure in bold, its
/// name, and any breakdown.
private struct ChartTooltip<Detail: View>: View {
    let heading: String
    let title: String
    @ViewBuilder let detail: Detail

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(heading)
                .font(.geist(.subheadline, weight: .semibold))
                .monospacedDigit()
                .foregroundStyle(Theme.foreground)
            Text(title)
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
            detail
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .fixedSize()
        .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
        .shadow(color: .black.opacity(0.12), radius: 6, y: 2)
    }
}

/// One row of a tooltip's breakdown, as the web's `TooltipSeries`.
private struct TooltipSeries: View {
    let color: Color
    let label: String
    let value: String

    var body: some View {
        HStack(spacing: 6) {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 8, height: 8)
            Text(label)
            Spacer(minLength: 12)
            Text(value).monospacedDigit()
        }
        .font(.geist(.caption))
        .foregroundStyle(Theme.mutedForeground)
    }
}

/// Books finished in each month or year.
private struct FinishedChart: View {
    let periods: [Shishoapp.StatsPeriod]
    @State private var selected: String?

    var body: some View {
        // A step more when the busiest period is exactly a tick, so its bar
        // never reaches the top, as the web's chart has it.
        let ticks = ChartTicks.roomy(Double(periods.map(\.finished).max() ?? 0))
        let top = ticks.last ?? 1
        let picked = selected.flatMap { label in periods.first { StatsText.periodLabel($0) == label } }
        VStack(alignment: .leading, spacing: 10) {
            ChartTotal(heading: Counts.books(periods.reduce(0) { $0 + $1.finished }),
                       hint: chartHint(byMonth: periods.first?.month != nil)) { EmptyView() }
            Chart {
                // Drawn first, so it sits behind the bars.
                if let picked {
                    RectangleMark(x: .value("Period", StatsText.periodLabel(picked)),
                                  yStart: .value("Books", 0.0), yEnd: .value("Books", top))
                        .foregroundStyle(Theme.secondary)
                }
                ForEach(periods, id: \.self) { period in
                    BarMark(x: .value("Period", StatsText.periodLabel(period)), y: .value("Books", Double(period.finished)))
                        .foregroundStyle(StatsColors.reading)
                        .cornerRadius(3)
                }
                // Last, so the tooltip draws over the bars.
                if let picked {
                    tooltipAnchor(StatsText.periodLabel(picked), in: periods, top: top) {
                        ChartTooltip(heading: Counts.books(picked.finished), title: StatsText.periodTitle(picked)) { EmptyView() }
                    }
                }
            }
            // The periods in order, whatever order the marks are drawn in.
            .chartXScale(domain: periods.map(StatsText.periodLabel))
            .scrubs($selected)
            .chartYScale(domain: 0...top)
            .chartYAxis {
                AxisMarks(position: .leading, values: ticks) { value in
                    AxisGridLine()
                    AxisValueLabel { Text(verbatim: (value.as(Double.self) ?? 0).formatted()) }
                }
            }
            .chartXAxis { periodAxis(count: periods.count) }
            .frame(height: 180)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(String(localized: "Books finished"))
            .accessibilityValue(periods.map { "\(StatsText.periodTitle($0)): \(Counts.books($0.finished))" }.joined(separator: ", "))
        }
    }
}

/// Reading and listening time in each month or year, listening stacked on
/// reading, on an axis of round hours, or minutes when no period reaches
/// an hour, as the web's.
private struct TimeChart: View {
    let periods: [Shishoapp.StatsPeriod]
    @State private var selected: String?

    private struct Segment: Identifiable {
        let id: String
        let label: String
        let kind: String
        let value: Double
    }

    private func segments(unit: Double) -> [Segment] {
        periods.flatMap { period -> [Segment] in
            let label = StatsText.periodLabel(period)
            return [
                Segment(id: "\(label)-r", label: label, kind: String(localized: "Reading"), value: Double(period.readingSeconds) / unit),
                Segment(id: "\(label)-l", label: label, kind: String(localized: "Listening"), value: Double(period.listeningSeconds) / unit),
            ]
        }
    }

    var body: some View {
        let axis = ChartTicks.duration(Double(periods.map { $0.readingSeconds + $0.listeningSeconds }.max() ?? 0), roomy: true)
        let top = axis.ticks.last ?? 1
        let picked = selected.flatMap { label in periods.first { StatsText.periodLabel($0) == label } }
        VStack(alignment: .leading, spacing: 10) {
            total
            Chart {
                // Drawn first, so it sits behind the bars.
                if let picked {
                    RectangleMark(x: .value("Period", StatsText.periodLabel(picked)),
                                  yStart: .value("Time", 0.0), yEnd: .value("Time", top))
                        .foregroundStyle(Theme.secondary)
                }
                ForEach(segments(unit: axis.unit)) { segment in
                    BarMark(x: .value("Period", segment.label), y: .value("Time", segment.value))
                        .foregroundStyle(by: .value("Kind", segment.kind))
                }
                // Last, so the tooltip draws over the bars.
                if let picked {
                    tooltipAnchor(StatsText.periodLabel(picked), in: periods, top: top) { tooltip(picked) }
                }
            }
            .chartForegroundStyleScale([String(localized: "Reading"): StatsColors.reading,
                                        String(localized: "Listening"): StatsColors.listening])
            .chartLegend(.hidden)
            // The periods in order, whatever order the marks are drawn in.
            .chartXScale(domain: periods.map(StatsText.periodLabel))
            .scrubs($selected)
            .chartYScale(domain: 0...top)
            .chartYAxis {
                AxisMarks(position: .leading, values: axis.ticks) { value in
                    AxisGridLine()
                    AxisValueLabel { Text(verbatim: axis.label(value.as(Double.self) ?? 0)) }
                }
            }
            .chartXAxis { periodAxis(count: periods.count) }
            .frame(height: 180)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(String(localized: "Reading and listening time"))
            .accessibilityValue(periods.map(describe).joined(separator: ", "))
        }
    }

    private func describe(_ period: Shishoapp.StatsPeriod) -> String {
        let reading = FileFacts.duration(Double(period.readingSeconds))
        let listening = FileFacts.duration(Double(period.listeningSeconds))
        return String(localized: "\(StatsText.periodTitle(period)): \(reading) reading, \(listening) listening")
    }

    private func tooltip(_ period: Shishoapp.StatsPeriod) -> some View {
        ChartTooltip(heading: FileFacts.duration(Double(period.readingSeconds + period.listeningSeconds)),
                     title: StatsText.periodTitle(period)) {
            VStack(alignment: .leading, spacing: 2) {
                TooltipSeries(color: StatsColors.reading, label: String(localized: "Reading"),
                              value: FileFacts.duration(Double(period.readingSeconds)))
                TooltipSeries(color: StatsColors.listening, label: String(localized: "Listening"),
                              value: FileFacts.duration(Double(period.listeningSeconds)))
            }
            .padding(.top, 4)
        }
    }

    /// The range's total, with its reading and listening, which double as
    /// the chart's legend.
    private var total: some View {
        let reading = periods.reduce(0) { $0 + $1.readingSeconds }
        let listening = periods.reduce(0) { $0 + $1.listeningSeconds }
        return ChartTotal(heading: FileFacts.duration(Double(reading + listening)), hint: chartHint(byMonth: periods.first?.month != nil)) {
            HStack(spacing: 12) {
                series(StatsColors.reading, String(localized: "Reading"), reading)
                series(StatsColors.listening, String(localized: "Listening"), listening)
            }
            .padding(.vertical, 2)
        }
    }

    private func series(_ color: Color, _ label: String, _ seconds: Int) -> some View {
        HStack(spacing: 4) {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 8, height: 8)
            Text(verbatim: "\(label) \(FileFacts.duration(Double(seconds)))")
                .monospacedDigit()
        }
        .font(.geist(.caption))
        .foregroundStyle(Theme.mutedForeground)
    }
}

/// Each period's name under its bar: all twelve months, or every few years
/// when years crowd the axis.
private func periodAxis(count: Int) -> some AxisContent {
    AxisMarks { value in
        if count <= 12 || value.index % max(1, count / 6) == 0 {
            AxisValueLabel(centered: true) {
                Text(verbatim: value.as(String.self) ?? "")
                    .font(.geist(size: 10, relativeTo: .caption2))
                    // Twelve names share the chart's width, so they stop
                    // growing before they run into each other.
                    .dynamicTypeSize(...DynamicTypeSize.xxLarge)
            }
        }
    }
}

/// Y-axis ticks the web's charts use (`chartTicks.ts`): from 0 to a clean
/// value at or above the most, at most five, on a step of 1, 2, or 5 times
/// a power of ten.
enum ChartTicks {
    static func nice(_ max: Double) -> [Double] {
        guard max > 0 else { return [0, 1] }
        var magnitude = 1.0
        var step = 1.0
        while true {
            if let fit = [1.0, 2.0, 5.0].first(where: { (max / ($0 * magnitude)).rounded(.up) <= 4 }) {
                step = fit * magnitude
                break
            }
            magnitude *= 10
        }
        var ticks: [Double] = []
        var value = 0.0
        while value < max + step {
            ticks.append(value)
            if value >= max { break }
            value += step
        }
        return ticks
    }

    /// nice, with one step more when max lands exactly on the top tick, so
    /// the tallest bar never reaches the top, as the web's `roomyTicks`.
    static func roomy(_ max: Double) -> [Double] {
        let ticks = nice(max)
        guard max > 0, let top = ticks.last, top == max, ticks.count > 1 else { return ticks }
        return ticks + [top + (ticks[1] - ticks[0])]
    }

    /// A time axis in hours from an hour up, and in minutes below it: its
    /// ticks are in that unit, labeled "2h" or "30m", as the web's
    /// `durationTicks`.
    struct DurationAxis {
        /// Seconds in a tick's unit.
        let unit: Double
        let ticks: [Double]

        func label(_ value: Double) -> String {
            value == 0 ? "0" : "\(value.formatted())\(unit == 3600 ? "h" : "m")"
        }
    }

    static func duration(_ maxSeconds: Double, roomy: Bool = false) -> DurationAxis {
        let unit: Double = maxSeconds >= 3600 ? 3600 : 60
        return DurationAxis(unit: unit, ticks: roomy ? self.roomy(maxSeconds / unit) : nice(maxSeconds / unit))
    }
}

// MARK: - Lists

private struct PaceList: View {
    let pace: Shishoapp.StatsPace

    var body: some View {
        let rows = StatsText.pace(pace)
        if rows.isEmpty {
            EmptyNote(text: String(localized: "Not enough timed reading yet."))
        } else {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(rows, id: \.kind) { row in
                    HStack(alignment: .firstTextBaseline) {
                        Text(row.kind)
                        Spacer(minLength: 12)
                        Text(row.value)
                            .monospacedDigit()
                            .multilineTextAlignment(.trailing)
                    }
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.foreground)
                    .accessibilityElement(children: .combine)
                }
                Text("From timed reading, leaving out skips ahead and time with a book left open, judged against your own usual pace.")
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                    .padding(.top, 4)
            }
        }
    }
}

/// Where the time went: each place with a bar of its reading and listening
/// as its share of all the time, so the bars add up to a whole, as the
/// web's.
private struct PlaceList: View {
    let places: [StatsText.Place]

    var body: some View {
        if places.isEmpty {
            EmptyNote(text: String(localized: "No reading time recorded."))
        } else {
            let total = Double(max(1, places.reduce(0) { $0 + $1.seconds }))
            VStack(alignment: .leading, spacing: 12) {
                StatsLegend()
                ForEach(Array(places.enumerated()), id: \.offset) { _, place in
                    VStack(alignment: .leading, spacing: 5) {
                        HStack(alignment: .firstTextBaseline) {
                            Text(place.label)
                                .font(.geist(.subheadline))
                                .foregroundStyle(Theme.foreground)
                                .lineLimit(1)
                            Spacer(minLength: 8)
                            Text(verbatim: "\(StatsText.placeTime(place)) · \(StatsText.share(Double(place.seconds) / total))")
                                .font(.geist(.caption))
                                .monospacedDigit()
                                .foregroundStyle(Theme.mutedForeground)
                        }
                        ShareBar(segments: [.init(share: Double(place.readingSeconds) / total, color: StatsColors.reading),
                                            .init(share: Double(place.listeningSeconds) / total, color: StatsColors.listening)])
                    }
                    .accessibilityElement(children: .combine)
                }
            }
        }
    }
}

/// A breakdown of finished books, most first: the first few, and all of
/// them on "Show all", scrolling inside the card with a search so hundreds
/// of tags stay usable, as the web's. Each row's bar is its share of the
/// finished books, so 2 mysteries of 3 books fill two thirds; a book can
/// count under several rows, so the shares needn't add up to a whole. A
/// row that opens a page lists the finished books it counts, as the web's
/// rows filter its Finished section.
private struct CountList: View {
    struct Row {
        let label: String
        let books: Int
        var opens: Route?
    }

    @Environment(AppModel.self) private var model

    /// The distinct finished books the rows count from, or nil from a
    /// server that doesn't say, when the largest row stands in.
    let books: Int?
    let rows: [Row]
    let noun: StatsText.Noun
    let emptyMessage: String
    let identifier: String

    @Environment(\.revealCard) private var revealCard
    @FocusState private var searching: Bool
    @State private var expanded = false
    @State private var search = ""
    /// The list's height with its first few rows, which it keeps expanded,
    /// so "Show all" never changes the card's size.
    @State private var collapsedHeight: CGFloat?

    private static let visibleRows = 5

    private var shown: [Row] {
        guard expanded else { return Array(rows.prefix(Self.visibleRows)) }
        let needle = search.trimmingCharacters(in: .whitespaces)
        return needle.isEmpty ? rows : rows.filter { $0.label.localizedCaseInsensitiveContains(needle) }
    }

    var body: some View {
        if rows.isEmpty {
            EmptyNote(text: emptyMessage)
        } else {
            VStack(alignment: .leading, spacing: 10) {
                if expanded {
                    TextField(String(localized: "Search \(noun.many)"), text: $search)
                        .font(.geist(.subheadline))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .padding(.horizontal, 10)
                        .frame(minHeight: 34)
                        .background(Theme.secondary, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                        .focused($searching)
                        .accessibilityIdentifier("\(identifier).search")
                    ScrollView {
                        list
                    }
                    .frame(maxHeight: .infinity, alignment: .top)
                    .scrollBounceBehavior(.basedOnSize)
                } else {
                    list
                }
                if expanded || rows.count > Self.visibleRows {
                    Button {
                        expanded.toggle()
                        search = ""
                    } label: {
                        Text(expanded ? String(localized: "Show fewer") : String(localized: "Show all \(noun.count(rows.count))"))
                            .font(.geist(.footnote, weight: .medium))
                            .foregroundStyle(Theme.primary)
                            .frame(minHeight: 32)
                    }
                    .buttonStyle(.plain)
                    .hitArea(height: 32)
                    .accessibilityIdentifier("\(identifier).toggle")
                }
            }
            .frame(height: expanded ? collapsedHeight : nil, alignment: .top)
            .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { height in
                if !expanded { collapsedHeight = height }
            }
            .id(identifier)
            // Typing in the search, the whole list shows, not only the
            // field the keyboard makes room for: the matches are under it.
            .onChange(of: searching) { _, searching in
                guard searching else { return }
                Task {
                    // After the keyboard's own scrolling, which would undo it.
                    try? await Task.sleep(for: .milliseconds(350))
                    revealCard?.reveal(identifier)
                }
            }
        }
    }

    private var list: some View {
        // Never less than a row's own count, so no bar overflows.
        let total = StatsText.BreakdownTotal(finished: books, largest: rows.map(\.books).max() ?? 0)
        return LazyVStack(alignment: .leading, spacing: 12) {
            ForEach(Array(shown.enumerated()), id: \.offset) { _, row in
                if let route = row.opens {
                    Button {
                        model.push(route)
                    } label: {
                        rowContent(row, total: total, opens: true)
                    }
                    .buttonStyle(.plain)
                    .hitArea(height: 32)
                    .accessibilityLabel(rowLabel(row, total: total))
                    .accessibilityIdentifier("\(identifier).row")
                } else {
                    rowContent(row, total: total, opens: false)
                        .accessibilityElement(children: .combine)
                }
            }
            if shown.isEmpty {
                EmptyNote(text: String(localized: "No \(noun.many) match."))
            }
        }
    }

    private func rowContent(_ row: Row, total: StatsText.BreakdownTotal, opens: Bool) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(alignment: .firstTextBaseline) {
                Text(row.label)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(1)
                Spacer(minLength: 8)
                Text(verbatim: total.text(row.books))
                    .font(.geist(.caption))
                    .monospacedDigit()
                    .foregroundStyle(Theme.mutedForeground)
                if opens {
                    Icon(.chevronRight, size: 13, relativeTo: .caption)
                        .foregroundStyle(Theme.mutedForeground)
                        .alignmentGuide(.firstTextBaseline) { $0[VerticalAlignment.center] + 4 }
                        .accessibilityHidden(true)
                }
            }
            ShareBar(segments: [.init(share: total.share(row.books), color: StatsColors.reading)])
        }
        .contentShape(Rectangle())
    }

    /// A row as VoiceOver reads it: "Mystery, 2 books, 67%".
    private func rowLabel(_ row: Row, total: StatsText.BreakdownTotal) -> String {
        "\(row.label), \(total.text(row.books).replacingOccurrences(of: " · ", with: ", "))"
    }
}

/// How Reading Stats words its figures and rows, as the web's
/// (`placeRows`, `paceRows`, `KIND_LABELS`, and the formatters in
/// `app/utils/format.ts`).
enum StatsText {
    struct Place: Equatable {
        let label: String
        let readingSeconds: Int
        let listeningSeconds: Int

        var seconds: Int { readingSeconds + listeningSeconds }
    }

    /// A breakdown's noun, as "Show all 12 genres" counts it.
    struct Noun {
        let one: String
        let many: String

        func count(_ count: Int) -> String {
            "\(count.formatted()) \(count == 1 ? one : many)"
        }
    }

    /// Whether the range has anything to chart, as the web's `hasActivity`.
    static func hasActivity(_ stats: Shishoapp.StatsResponse) -> Bool {
        let totals = stats.totals
        return totals.finished > 0 || totals.didNotFinish > 0 || totals.readingSeconds > 0 || totals.listeningSeconds > 0
            || totals.wordsRead > 0
    }

    /// Where reading happened, most time first: each Device by its name,
    /// and the web's or the app's reader and player apart, by the web's
    /// names, for time no Device is named for.
    static func places(_ places: [Shishoapp.StatsPlace], ownDeviceID: String) -> [Place] {
        places.flatMap { place -> [Place] in
            if let name = place.deviceName, !name.isEmpty {
                let label = place.deviceId == ownDeviceID ? String(localized: "\(name) (this device)") : name
                return [Place(label: label, readingSeconds: place.readingSeconds, listeningSeconds: place.listeningSeconds)]
            }
            if place.deviceId == ownDeviceID, !ownDeviceID.isEmpty {
                return [Place(label: String(localized: "This device"), readingSeconds: place.readingSeconds,
                              listeningSeconds: place.listeningSeconds)]
            }
            let reader: String
            let player: String?
            switch place.source {
            case .webReader: (reader, player) = (String(localized: "Shisho web reader"), String(localized: "Shisho web player"))
            case .shishoApp: (reader, player) = (String(localized: "Shisho app reader"), String(localized: "Shisho app player"))
            case .kobo: (reader, player) = ("Kobo", nil)
            case .koreader: (reader, player) = ("KOReader", nil)
            case .unknown(let raw): (reader, player) = (raw, nil)
            }
            guard let player else {
                return [Place(label: reader, readingSeconds: place.readingSeconds, listeningSeconds: place.listeningSeconds)]
            }
            return [Place(label: reader, readingSeconds: place.readingSeconds, listeningSeconds: 0),
                    Place(label: player, readingSeconds: 0, listeningSeconds: place.listeningSeconds)]
                .filter { $0.seconds > 0 }
        }
        .sorted { $0.seconds > $1.seconds }
    }

    /// A place's time, as "2h 5m reading, 40m listening".
    static func placeTime(_ place: Place) -> String {
        var parts: [String] = []
        if place.readingSeconds > 0 { parts.append(String(localized: "\(FileFacts.duration(Double(place.readingSeconds))) reading")) }
        if place.listeningSeconds > 0 {
            parts.append(String(localized: "\(FileFacts.duration(Double(place.listeningSeconds))) listening"))
        }
        return parts.isEmpty ? FileFacts.duration(0) : parts.joined(separator: ", ")
    }

    /// What a breakdown's rows are shares of: the distinct finished books,
    /// so 2 mysteries of 3 books fill two thirds, never less than a row's own
    /// count, so no bar overflows. A server that doesn't say how many books
    /// leaves only the largest row to measure against, which says nothing
    /// as a percent, so the rows give their counts alone.
    struct BreakdownTotal: Equatable {
        let finished: Int?
        let largest: Int

        /// A row's share of the books, 0 to 1, for its bar.
        func share(_ books: Int) -> Double {
            Double(books) / Double(max(1, finished ?? 0, largest))
        }

        /// "2 books · 67%", or "2 books" without the books to measure by.
        func text(_ books: Int) -> String {
            guard finished != nil else { return Counts.books(books) }
            return "\(Counts.books(books)) · \(StatsText.share(share(books)))"
        }
    }

    /// A share of the whole, as "26%", or "<1%" for a sliver, as the web's.
    static func share(_ fraction: Double) -> String {
        let percent = fraction * 100
        return percent > 0 && percent < 1 ? "<1%" : "\(Int(percent.rounded()))%"
    }

    /// Each kind's pace, only those with enough timed reading to say.
    static func pace(_ pace: Shishoapp.StatsPace) -> [(kind: String, value: String)] {
        var rows: [(kind: String, value: String)] = []
        if let wpm = pace.wordsPerMinute {
            rows.append((String(localized: "Ebooks"), String(localized: "\(Int(wpm.rounded()).formatted()) words per minute")))
        }
        if let pages = pace.comicPagesPerHour {
            rows.append((String(localized: "Comics"), String(localized: "\(Int(pages.rounded()).formatted()) pages per hour")))
        }
        if let pages = pace.pdfPagesPerHour {
            rows.append((String(localized: "PDFs"), String(localized: "\(Int(pages.rounded()).formatted()) pages per hour")))
        }
        if let speed = pace.listeningSpeed {
            rows.append((String(localized: "Audiobooks"),
                         String(localized: "\(speed.formatted(.number.precision(.fractionLength(0...1))))× real time")))
        }
        return rows
    }

    static func kind(_ kind: String) -> String {
        switch kind {
        case "ebook": String(localized: "Ebooks")
        case "comic": String(localized: "Comics")
        case "audiobook": String(localized: "Audiobooks")
        default: String(localized: "Unknown")
        }
    }

    /// Rereads among the finished, as "2 rereads".
    static func rereads(_ count: Int) -> String {
        // Spelled out, since inflection takes "reread" for a verb and gets
        // both wrong.
        count == 1 ? String(localized: "1 reread") : String(localized: "\(count.formatted()) rereads")
    }

    /// A Rating in stars, as "4.5 stars" or "1 star".
    static func stars(_ stars: Double) -> String {
        let number = stars.formatted(.number.precision(.fractionLength(0...2)))
        return stars == 1 ? String(localized: "\(number) star") : String(localized: "\(number) stars")
    }

    /// A headline total of time, in days once it reaches one, as the web's
    /// `formatTotalDuration`: a day here is 24 hours spent, so the hours
    /// show beside it (totalHours).
    static func totalDuration(_ seconds: Int) -> String {
        guard seconds >= 86400 else { return FileFacts.duration(Double(seconds)) }
        return "\(seconds / 86400)d \((seconds % 86400) / 3600)h \((seconds % 3600) / 60)m"
    }

    static func totalHours(_ seconds: Int) -> String? {
        seconds >= 86400 ? FileFacts.duration(Double(seconds)) : nil
    }

    /// A count shortened as the web's `formatCompactNumber`: "2.1M".
    static func compact(_ count: Int) -> String {
        count.formatted(.number.notation(.compactName).precision(.fractionLength(0...1)))
    }

    /// A period under its bar: "Mar", or "2026".
    static func periodLabel(_ period: Shishoapp.StatsPeriod) -> String {
        guard let month = period.month else { return String(period.year) }
        return Calendar.current.shortMonthSymbols[max(0, min(11, month - 1))]
    }

    /// A period with its year: "March 2026", or "2026".
    static func periodTitle(_ period: Shishoapp.StatsPeriod) -> String {
        guard let month = period.month else { return String(period.year) }
        return "\(Calendar.current.monthSymbols[max(0, min(11, month - 1))]) \(period.year)"
    }
}
