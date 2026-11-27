import ShishoAPI
import SwiftUI

/// Reading Stats' Calendar, as the web's Calendar tab: a heatmap of the last
/// 12 months or a year picked beside it, a month calendar that goes back to
/// the first month with any reading, and the day picked under it. Each part
/// shows the copy last fetched, fetched again whenever the server can be
/// reached, so the tab works offline once seen (CapabilityReadingCalendar),
/// with this device's reading not synced yet laid over it as the Overview
/// lays it (CalendarLayering). A part never fetched shows that reading
/// alone. It sits in the Reading Stats page's scroll view.
struct ReadingCalendarView: View {
    @Environment(AppModel.self) private var model
    /// The heatmap's year, or nil for the last 12 months.
    @State private var heatmapYear: Int?
    /// The month shown, or nil for this one.
    @State private var month: YearMonth?
    @State private var selectedDay: String?
    @State private var heatmap: CalendarShown<Shishoapp.StatsHeatmapResponse>??
    /// The last heatmap shown, so a year picked with no copy, offline, still
    /// offers the picker back and the month calendar, which has copies of
    /// its own.
    @State private var lastHeatmap: Shishoapp.StatsHeatmapResponse?
    /// The earliest day a heatmap the server sent starts its years at, so a
    /// year with no copy, shown from this device's reading alone, still
    /// offers every year before it.
    @State private var firstDayKnown: String?
    @State private var calendar: CalendarShown<Shishoapp.StatsCalendarResponse>??
    @State private var detail: CalendarShown<CalendarDayDetail>??
    @State private var books: [Int: BookSummary]?
    /// Copies being fetched, and ones a fetch was tried for, by key, so a
    /// copy that can't come says so rather than spinning.
    @State private var fetching: Set<CalendarCacheKey> = []
    @State private var tried: Set<CalendarCacheKey> = []

    private var heatmapKey: CalendarCacheKey { .heatmap(year: heatmapYear) }

    /// Today as the server last said, or the device's.
    private var today: String {
        (heatmap ?? nil)?.value.today ?? CalendarLayout.format(Self.localToday())
    }

    private var shownMonth: YearMonth {
        month ?? CalendarLayout.yearMonth(today)
    }

    private var monthKey: CalendarCacheKey { .month(year: shownMonth.year, month: shownMonth.month) }

    var body: some View {
        let deviceID = model.session?.account.deviceID ?? ""
        let deviceName = Installation.deviceName
        VStack(alignment: .leading, spacing: 0) {
            if case .some(.some(let shown)) = heatmap {
                heatmapSection(shown.value, state: .shown(deviceOnly: shown.deviceOnly))
                monthSection(shown.value)
            } else if let lastHeatmap {
                let unavailable = heatmap != nil && !waiting(for: heatmapKey)
                heatmapSection(lastHeatmap, state: unavailable ? .unavailable : .waiting)
                monthSection(lastHeatmap)
            } else if case .some(.none) = heatmap {
                if waiting(for: heatmapKey) {
                    ProgressView()
                        .frame(maxWidth: .infinity)
                        .padding(.top, 80)
                } else {
                    EmptyNotice(title: String(localized: "The calendar needs a connection"),
                                message: String(localized: "It shows here once the app has reached your server, and works offline after."),
                                icon: .chartColumn)
                        .accessibilityIdentifier("calendar.unavailable")
                }
            }
        }
        .observing(model.session?.store, id: heatmapYear, into: $heatmap) { [heatmapYear] db in
            try CatalogStore.calendarHeatmap(db, year: heatmapYear)
        }
        .onChange(of: heatmap) { _, shown in
            guard case .some(.some(let shown)) = shown else { return }
            lastHeatmap = shown.value
            if !shown.deviceOnly, let first = shown.value.firstDay, firstDayKnown.map({ first < $0 }) ?? true {
                firstDayKnown = first
            }
        }
        .observing(model.session?.store, id: shownMonth, into: $calendar) { [shownMonth] db in
            try CatalogStore.calendarMonth(db, year: shownMonth.year, month: shownMonth.month)
        }
        .observing(model.session?.store, id: selectedDay, into: $detail) { [selectedDay] db in
            guard let selectedDay else { return nil }
            return try CatalogStore.calendarDay(db, day: selectedDay, ownDeviceID: deviceID, deviceName: deviceName)
        }
        .observing(model.session?.store, id: bookIDs, into: $books) { [bookIDs] db in
            try CatalogStore.calendarBooks(db, ids: bookIDs)
        }
        .task(id: Trigger(key: heatmapKey, online: model.connection == .online, syncing: model.isSyncing)) {
            await fetch(heatmapKey)
        }
        .task(id: Trigger(key: monthKey, online: model.connection == .online, syncing: model.isSyncing)) {
            await fetch(monthKey)
        }
        .task(id: Trigger(key: selectedDay.map { .day($0) }, online: model.connection == .online, syncing: model.isSyncing)) {
            if let selectedDay { await fetch(.day(selectedDay)) }
        }
    }

    // MARK: - Fetching

    /// What a fetch waits on: a key, fetched again when the server can be
    /// reached again and after each sync, which may have sent reading.
    private struct Trigger: Hashable {
        let key: CalendarCacheKey?
        let online: Bool
        let syncing: Bool
    }

    private func waiting(for key: CalendarCacheKey) -> Bool {
        fetching.contains(key) || (model.connection == .online && !tried.contains(key))
    }

    private func fetch(_ key: CalendarCacheKey) async {
        guard let session = model.session, model.connection == .online, model.offers(.readingCalendar) else {
            tried.insert(key)
            return
        }
        // A sync may be sending reading the copy should count, so the fetch
        // waits for it to end, which runs this again.
        guard !model.isSyncing else { return }
        fetching.insert(key)
        defer {
            fetching.remove(key)
            tried.insert(key)
        }
        let began = Date()
        do {
            switch key {
            case .heatmap(let year):
                let response = try await session.client.statsHeatmap(year: year)
                try session.store.saveCalendar(response, key: key, fetchedAt: began)
            case .month(let year, let month):
                let response = try await session.client.statsCalendar(year: year, month: month)
                try session.store.saveCalendar(response, key: key, fetchedAt: began)
            case .day(let day):
                let response = try await session.client.statsDay(day: day)
                try session.store.saveCalendar(response, key: key, fetchedAt: began,
                                               zone: response.timeZone.isEmpty ? nil : response.timeZone)
            }
        } catch is CancellationError {
        } catch {
            await Diagnostics.shared.log(.app, "reading calendar not fetched: \(Diagnostics.describe(error))")
        }
    }

    // MARK: - Selecting

    /// The Books the month and the day name, to find in the catalog.
    private var bookIDs: [Int] {
        var ids = Set<Int>()
        for ref in (calendar ?? nil)?.value.books ?? [] { if let id = ref.bookId { ids.insert(id) } }
        if let day = (detail ?? nil)?.value.response {
            for ref in day.finished { if let id = ref.bookId { ids.insert(id) } }
            for session in day.sessions { if let id = session.book.bookId { ids.insert(id) } }
        }
        return ids.sorted()
    }

    /// Opens day, from the heatmap or the calendar, moving the calendar to
    /// its month. A day the heatmap doesn't show brings up its year.
    private func select(_ day: String?, heatmap shown: Shishoapp.StatsHeatmapResponse) {
        guard let day else {
            selectedDay = nil
            return
        }
        let target = CalendarLayout.yearMonth(day)
        month = target == CalendarLayout.yearMonth(shown.today) ? nil : target
        selectedDay = day
        if day < shown.from || day > shown.to {
            heatmapYear = target.year
        }
    }

    private static func localToday() -> Date {
        let parts = Calendar.current.dateComponents([.year, .month, .day], from: Date())
        var utc = Calendar(identifier: .gregorian)
        utc.timeZone = TimeZone(identifier: "UTC")!
        return utc.date(from: parts) ?? Date()
    }

    // MARK: - The heatmap

    /// Every year from this one back to the first with any reading.
    private func years(_ shown: Shishoapp.StatsHeatmapResponse) -> [Int] {
        let thisYear = CalendarLayout.yearMonth(shown.today).year
        let first = [shown.firstDay, firstDayKnown].compactMap { $0 }.min()
        let firstYear = first.map { CalendarLayout.yearMonth($0).year } ?? thisYear
        var years = Array(stride(from: thisYear, through: min(firstYear, thisYear), by: -1))
        if let heatmapYear, !years.contains(heatmapYear) { years.append(heatmapYear) }
        return years
    }

    private var periodName: String {
        heatmapYear.map { String($0) } ?? String(localized: "Last 12 months")
    }

    private func heatmapPicker(_ shown: Shishoapp.StatsHeatmapResponse) -> some View {
        Menu {
            Picker(String(localized: "Heatmap year"), selection: $heatmapYear) {
                Text("Last 12 months").tag(Int?.none)
                ForEach(years(shown), id: \.self) { year in
                    Text(verbatim: String(year)).tag(Int?.some(year))
                }
            }
        } label: {
            HStack(spacing: 4) {
                Text(periodName)
                    .font(.geist(.subheadline, weight: .medium))
                Icon(.chevronDown, size: 12, relativeTo: .subheadline)
                    .accessibilityHidden(true)
            }
            .foregroundStyle(Theme.foreground)
            .frame(minHeight: 44)
        }
        .accessibilityLabel(String(localized: "Heatmap year"))
        .accessibilityValue(periodName)
        .accessibilityIdentifier("calendar.heatmapYear")
    }

    /// What the heatmap's card holds: the heatmap, or, for a year with no
    /// copy, a wait or a note under the picker, which stays to pick another.
    private enum HeatmapState {
        case shown(deviceOnly: Bool)
        case waiting
        case unavailable
    }

    /// The heatmap for shown, or, unless state is shown, a card in its place
    /// with the picker beside it offering shown's years.
    private func heatmapSection(_ shown: Shishoapp.StatsHeatmapResponse, state: HeatmapState) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .center) {
                Text("Reading by day")
                    .font(.geist(.headline, weight: .semibold))
                    .accessibilityAddTraits(.isHeader)
                Spacer(minLength: 8)
                heatmapPicker(shown)
            }
            switch state {
            case .shown(let deviceOnly):
                Heatmap(shown: shown, heatmapYear: heatmapYear, selectedDay: selectedDay) { day in
                    select(day, heatmap: shown)
                }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .leading)
                .cardStyle(radius: Theme.radius + 2)
                if deviceOnly { DeviceOnlyNote() }
            case .waiting:
                ProgressView()
                    .frame(maxWidth: .infinity, minHeight: 120)
                    .cardStyle(radius: Theme.radius + 2)
            case .unavailable:
                Text("This heatmap shows once the app can reach your server.")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .multilineTextAlignment(.center)
                    .padding(14)
                    .frame(maxWidth: .infinity, minHeight: 120)
                    .cardStyle(radius: Theme.radius + 2)
                    .accessibilityIdentifier("calendar.heatmapUnavailable")
            }
        }
        .padding(.top, 16)
    }

    // MARK: - The month

    private func monthSection(_ shown: Shishoapp.StatsHeatmapResponse) -> some View {
        let last = CalendarLayout.yearMonth(shown.today)
        // Back to the first month with any reading, and at least as far as
        // the heatmap shows, so every square it shows opens its day.
        var first = min(last, CalendarLayout.yearMonth(shown.from))
        if let firstDay = shown.firstDay { first = min(first, CalendarLayout.yearMonth(firstDay)) }
        let current = min(max(shownMonth, first), last)
        let shownCalendar = calendar ?? nil
        let copy = shownCalendar?.value
        let colors = CalendarLayout.assignBookColors(copy?.books.map(\.key) ?? [])
        return VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 4) {
                Text(Self.monthTitle(current))
                    .font(.geist(.headline, weight: .semibold))
                    .accessibilityAddTraits(.isHeader)
                Spacer(minLength: 8)
                monthButton(.chevronLeft, label: String(localized: "Previous month"), identifier: "calendar.previous",
                            enabled: current > first) { move(by: -1, from: current, last: last) }
                monthButton(.chevronRight, label: String(localized: "Next month"), identifier: "calendar.next",
                            enabled: current < last) { move(by: 1, from: current, last: last) }
            }
            if let copy {
                MonthGrid(calendar: copy, month: current, today: shown.today, selectedDay: selectedDay, colors: colors) { day in
                    select(selectedDay == day ? nil : day, heatmap: shown)
                }
                // The last month's bars stay, dimmed, until the next one's
                // come.
                .opacity(copy.year == current.year && copy.month == current.month ? 1 : 0.6)
                if shownCalendar?.deviceOnly == true { DeviceOnlyNote() }
            } else if waiting(for: monthKey) {
                ProgressView()
                    .frame(maxWidth: .infinity, minHeight: 120)
            } else {
                Text("This month shows once the app can reach your server.")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(maxWidth: .infinity, minHeight: 80)
                    .accessibilityIdentifier("calendar.monthUnavailable")
            }
            if let selectedDay {
                Divider().overlay(Theme.border)
                let shownDay = detail ?? nil
                // The month's note covers a day it shows alone too.
                DayDetail(day: selectedDay, detail: shownDay?.value, waiting: waiting(for: .day(selectedDay)),
                          deviceOnly: shownDay?.deviceOnly == true && shownCalendar?.deviceOnly != true,
                          colors: colors, books: books ?? [:]) {
                    self.selectedDay = nil
                }
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .cardStyle(radius: Theme.radius + 2)
        .padding(.top, 24)
    }

    private func move(by step: Int, from current: YearMonth, last: YearMonth) {
        let next = YearMonth(index: current.index + step)
        month = next == last ? nil : next
        selectedDay = nil
    }

    private func monthButton(_ icon: Lucide, label: String, identifier: String, enabled: Bool,
                             action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Icon(icon, size: 18, relativeTo: .headline)
                .foregroundStyle(enabled ? Theme.foreground : Theme.mutedForeground.opacity(0.5))
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
        .accessibilityLabel(label)
        .accessibilityIdentifier(identifier)
    }

    static func monthTitle(_ month: YearMonth) -> String {
        guard let date = CalendarLayout.date(String(format: "%04d-%02d-01", month.year, month.month)) else { return "" }
        return date.formatted(Date.FormatStyle(timeZone: .gmt).month(.wide).year())
    }
}

// MARK: - Formatting

/// How the calendar words days and times, as the web's `format.ts`.
enum CalendarText {
    private static func style(_ base: Date.FormatStyle) -> Date.FormatStyle {
        var style = base
        style.timeZone = .gmt
        return style
    }

    /// "Wednesday, November 25, 2026".
    static func longDay(_ day: String) -> String {
        guard let date = CalendarLayout.date(day) else { return day }
        return date.formatted(style(Date.FormatStyle(date: .complete, time: .omitted)))
    }

    /// "Nov 25".
    static func shortDay(_ day: String) -> String {
        guard let date = CalendarLayout.date(day) else { return day }
        return date.formatted(style(Date.FormatStyle().month(.abbreviated).day()))
    }

    /// "Nov".
    static func shortMonth(_ month: YearMonth) -> String {
        guard let date = CalendarLayout.date(String(format: "%04d-%02d-01", month.year, month.month)) else { return "" }
        return date.formatted(style(Date.FormatStyle().month(.abbreviated)))
    }

    /// A total that may run past a day: "6h 43m", or "2d 4h 10m (52h 10m)".
    static func total(_ seconds: Int) -> String {
        guard seconds >= 86400 else { return FileFacts.duration(Double(seconds)) }
        let days = seconds / 86400, hours = (seconds % 86400) / 3600, minutes = (seconds % 3600) / 60
        return "\(days)d \(hours)h \(minutes)m (\(FileFacts.duration(Double(seconds))))"
    }

    /// An instant's time in zone, with its date when it falls on another
    /// day than day: "7:00 AM", or "Nov 24, 11:30 PM".
    static func time(_ value: String, day: String, zone: TimeZone) -> String {
        guard let instant = CalendarLayout.instant(value) else { return "" }
        var timeStyle = Date.FormatStyle(date: .omitted, time: .shortened)
        timeStyle.timeZone = zone
        let time = instant.formatted(timeStyle)
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        let parts = calendar.dateComponents([.year, .month, .day], from: instant)
        let onDay = String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
        return onDay == day ? time : String(localized: "\(shortDay(onDay)), \(time)")
    }

    /// The hour's label on the day's timeline: "12 AM", "6 AM".
    static func hour(_ hour: Int) -> String {
        guard let date = CalendarLayout.date("2000-01-01")?.addingTimeInterval(Double(hour % 24) * 3600) else { return "" }
        return date.formatted(style(Date.FormatStyle().hour()))
    }

    static func percent(_ value: Double) -> String {
        "\(Int((value * 100).rounded()))%"
    }

    /// How far a Session took the reader, or where it stayed.
    static func progress(_ session: Shishoapp.StatsDaySession) -> String {
        let from = percent(session.startPercentage), to = percent(session.endPercentage)
        return from == to ? String(localized: "at \(from)") : String(localized: "\(from) to \(to)")
    }

    /// Where a Session happened: its Device by name, or the reader or
    /// player it came from, marked when its time was estimated.
    static func source(_ session: Shishoapp.StatsDaySession) -> String {
        let listening = session.kind == .listening
        let name: String
        if session.deviceId != nil, let device = session.deviceName, !device.isEmpty {
            name = device
        } else {
            switch session.source {
            case .webReader: name = listening ? String(localized: "Shisho web player") : String(localized: "Shisho web reader")
            case .shishoApp: name = listening ? String(localized: "Shisho app player") : String(localized: "Shisho app reader")
            case .kobo: name = "Kobo"
            case .koreader: name = "KOReader"
            case .unknown(let raw): name = raw
            }
        }
        return session.measured ? name : String(localized: "\(name) (estimated)")
    }

    static func title(_ ref: Shishoapp.StatsBookRef) -> String {
        ref.title.isEmpty ? String(localized: "Unknown Book") : ref.title
    }
}

// MARK: - The heatmap

/// The heatmap's days, one square each, darker for more reading and
/// listening. A tap opens a day; a press and hold shows each day's time
/// under the finger as it moves, as hovering does on the web. It's drawn
/// for sighted readers: VoiceOver reads its total and busiest day, and the
/// calendar below reaches every day.
private struct Heatmap: View {
    let shown: Shishoapp.StatsHeatmapResponse
    let heatmapYear: Int?
    let selectedDay: String?
    let onSelect: (String) -> Void

    /// The day under a held finger.
    @State private var held: String?
    /// Where the grid sits in the heatmap as it scrolls, kept out of the
    /// view's state so scrolling doesn't redraw every square.
    @State private var grid = GridFrame()
    nonisolated private static let space = "calendar.heatmap"

    @ScaledMetric(relativeTo: .caption2) private var cell: CGFloat = 16
    private let gap: CGFloat = 3
    /// Months are labeled at least this many weeks apart.
    private let monthLabelGap = 3

    private var byDay: [String: Int] {
        Dictionary(shown.days.map { ($0.day, $0.readingSeconds + $0.listeningSeconds) }, uniquingKeysWith: +)
    }

    private var step: CGFloat { cell + gap }

    private var period: String {
        heatmapYear.map { String($0) } ?? String(localized: "the last 12 months")
    }

    // The web's bg-muted and bg-reading at a quarter, half, three quarters,
    // and full.
    static let shades: [Color] = [Theme.secondary, StatsColors.reading.opacity(0.25), StatsColors.reading.opacity(0.5),
                                  StatsColors.reading.opacity(0.75), StatsColors.reading]

    var body: some View {
        let byDay = byDay
        let level = CalendarLayout.heatLevels(Array(byDay.values))
        let weeks = CalendarLayout.heatmapWeeks(from: shown.from, to: shown.to)
        let total = byDay.values.reduce(0, +)
        let busiest = byDay.max { $0.value != $1.value ? $0.value < $1.value : $0.key > $1.key }
        VStack(alignment: .leading, spacing: 8) {
            Text(total > 0 ? String(localized: "\(CalendarText.total(total)) in \(period)") : String(localized: "No reading in \(period)"))
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
            HStack(alignment: .top, spacing: 4) {
                weekdayLabels
                ScrollViewReader { proxy in
                    ScrollView(.horizontal, showsIndicators: false) {
                        VStack(alignment: .leading, spacing: gap) {
                            monthLabels(weeks)
                            HStack(alignment: .top, spacing: gap) {
                                ForEach(Array(weeks.enumerated()), id: \.offset) { index, week in
                                    VStack(spacing: gap) {
                                        ForEach(0..<7, id: \.self) { row in
                                            square(week[row], seconds: week[row].flatMap { byDay[$0] } ?? 0, level: level)
                                        }
                                    }
                                    .id(index)
                                }
                            }
                            .onGeometryChange(for: CGRect.self) { $0.frame(in: .named(Self.space)) } action: { [grid] frame in
                                grid.frame = frame
                            }
                            .overlay { scrub(weeks) }
                        }
                        .padding(.trailing, 1)
                    }
                    .onAppear { scroll(proxy, weeks: weeks) }
                    .onChange(of: shown.from) { scroll(proxy, weeks: weeks) }
                }
            }
            HStack(spacing: 4) {
                Spacer()
                Text("Less")
                ForEach(0..<5, id: \.self) { shade in
                    RoundedRectangle(cornerRadius: 2).fill(Self.shades[shade]).frame(width: 10, height: 10)
                }
                Text("More")
            }
            .font(.geist(.caption))
            .foregroundStyle(Theme.mutedForeground)
        }
        .coordinateSpace(.named(Self.space))
        // Over everything, so the tooltip covers the squares around the
        // one held.
        .overlay { tooltip(weeks) }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(String(localized: "Reading by day"))
        .accessibilityValue(summary(total: total, busiest: busiest))
        .accessibilityIdentifier("calendar.heatmap")
    }

    private func summary(total: Int, busiest: (key: String, value: Int)?) -> String {
        guard total > 0, let busiest else { return String(localized: "No reading in \(period)") }
        return String(localized: "\(CalendarText.total(total)) in \(period). The most was \(FileFacts.duration(Double(busiest.value))) on \(CalendarText.longDay(busiest.key)).")
    }

    /// Scrolls the year to the selected day, or today, so a phone opens on
    /// the latest weeks.
    private func scroll(_ proxy: ScrollViewProxy, weeks: [CalendarLayout.Week]) {
        let lastSoFar = shown.today < shown.to ? shown.today : shown.to
        let focus = selectedDay.flatMap { $0 >= shown.from && $0 <= shown.to ? $0 : nil } ?? lastSoFar
        guard let index = weeks.firstIndex(where: { $0.contains(focus) }) else { return }
        proxy.scrollTo(index, anchor: index == weeks.count - 1 ? .trailing : .center)
    }

    private var weekdayLabels: some View {
        VStack(alignment: .leading, spacing: gap) {
            // The month labels' row.
            Color.clear.frame(height: labelHeight)
            ForEach(0..<7, id: \.self) { row in
                Text(row % 2 == 1 ? Calendar.current.shortStandaloneWeekdaySymbols[row] : "")
                    .font(.geist(size: 10, relativeTo: .caption2))
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(height: cell, alignment: .center)
            }
        }
        .fixedSize()
    }

    @ScaledMetric(relativeTo: .caption2) private var labelHeight: CGFloat = 14

    private func monthLabels(_ weeks: [CalendarLayout.Week]) -> some View {
        var labels: [(col: Int, text: String)] = []
        for (index, week) in weeks.enumerated() {
            guard let first = week.first(where: { $0?.hasSuffix("-01") == true }) ?? nil else { continue }
            // A label in the last week would run past the grid.
            if index == weeks.count - 1 { continue }
            if let last = labels.last, index - last.col < monthLabelGap { continue }
            labels.append((index, CalendarText.shortMonth(CalendarLayout.yearMonth(first))))
        }
        let width = CGFloat(weeks.count) * (cell + gap) - gap
        return ZStack(alignment: .topLeading) {
            Color.clear.frame(width: max(width, 0), height: labelHeight)
            ForEach(labels, id: \.col) { label in
                Text(label.text)
                    .font(.geist(size: 10, relativeTo: .caption2))
                    .foregroundStyle(Theme.mutedForeground)
                    .fixedSize()
                    .offset(x: CGFloat(label.col) * (cell + gap))
            }
        }
    }

    /// Taps and holds on the grid. One UIKit view takes both, since it sits
    /// over the squares and a SwiftUI tap under it would never fire.
    private func scrub(_ weeks: [CalendarLayout.Week]) -> some View {
        HeatmapGestures { point in
            if let day = CalendarLayout.heatmapDay(at: point, in: weeks, stride: step), day <= shown.today {
                onSelect(day)
            }
        } held: { point in
            guard let point else {
                held = nil
                return
            }
            // On a blank square, or past the grid, the last day stays.
            if let day = CalendarLayout.heatmapDay(at: point, in: weeks, stride: step), day != held {
                held = day
                UISelectionFeedbackGenerator().selectionChanged()
            }
        }
    }

    @ViewBuilder
    private func tooltip(_ weeks: [CalendarLayout.Week]) -> some View {
        if let held, let col = weeks.firstIndex(where: { $0.contains(held) }),
           let row = weeks[col].firstIndex(of: held) {
            let anchor = CGRect(x: grid.frame.minX + CGFloat(col) * step, y: grid.frame.minY + CGFloat(row) * step,
                                width: cell, height: cell)
            let entries = shown.days.filter { $0.day == held }
            TooltipPlacement(anchor: anchor) {
                HeatmapTooltip(day: held, future: held > shown.today,
                               reading: entries.reduce(0) { $0 + $1.readingSeconds },
                               listening: entries.reduce(0) { $0 + $1.listeningSeconds })
            }
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }
    }

    @ViewBuilder
    private func square(_ day: String?, seconds: Int, level: (Int) -> Int) -> some View {
        if let day {
            let future = day > shown.today
            RoundedRectangle(cornerRadius: 2)
                .fill(Self.shades[level(seconds)])
                .overlay {
                    if selectedDay == day || held == day {
                        RoundedRectangle(cornerRadius: 2).strokeBorder(Theme.foreground, lineWidth: 1.5)
                    }
                }
                .frame(width: cell, height: cell)
                .opacity(future ? 0.5 : 1)
        } else {
            Color.clear.frame(width: cell, height: cell)
        }
    }
}

/// Where the heatmap's grid sits as it scrolls. A class, so writing it
/// doesn't redraw the heatmap; the tooltip reads it when a hold moves.
@MainActor
private final class GridFrame {
    var frame: CGRect = .zero
}

/// Places its content, a tooltip, by the square at anchor, inside the
/// space it's given (`CalendarLayout.tooltipOrigin`).
private struct TooltipPlacement: Layout {
    let anchor: CGRect
    private let gap: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        proposal.replacingUnspecifiedDimensions()
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        for subview in subviews {
            let size = subview.sizeThatFits(.unspecified)
            let origin = CalendarLayout.tooltipOrigin(size: size, anchor: anchor, in: CGRect(origin: .zero, size: bounds.size), gap: gap)
            subview.place(at: CGPoint(x: bounds.minX + origin.x, y: bounds.minY + origin.y), proposal: ProposedViewSize(size))
        }
    }
}

/// A day's tooltip on the heatmap, in the look of Reading Stats' chart
/// tooltips: its time in bold, the day, and its reading and listening.
private struct HeatmapTooltip: View {
    let day: String
    let future: Bool
    let reading: Int
    let listening: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            if future {
                heading(CalendarText.longDay(day))
            } else {
                heading(reading + listening > 0 ? FileFacts.duration(Double(reading + listening)) : String(localized: "No reading"))
                Text(CalendarText.longDay(day))
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                if reading > 0, listening > 0 {
                    VStack(alignment: .leading, spacing: 2) {
                        series(StatsColors.reading, String(localized: "Reading"), reading)
                        series(StatsColors.listening, String(localized: "Listening"), listening)
                    }
                    .padding(.top, 4)
                }
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .fixedSize()
        .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
        .shadow(color: .black.opacity(0.12), radius: 6, y: 2)
    }

    private func heading(_ text: String) -> some View {
        Text(text)
            .font(.geist(.subheadline, weight: .semibold))
            .monospacedDigit()
            .foregroundStyle(Theme.foreground)
    }

    private func series(_ color: Color, _ label: String, _ seconds: Int) -> some View {
        HStack(spacing: 6) {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 8, height: 8)
            Text(label)
            Spacer(minLength: 12)
            Text(FileFacts.duration(Double(seconds))).monospacedDigit()
        }
        .font(.geist(.caption))
        .foregroundStyle(Theme.mutedForeground)
    }
}

/// A tap, and a press and hold that reports where the finger is until it
/// lifts (nil). UIKit's, since SwiftUI's own drag inside a ScrollView keeps
/// a swipe that starts on it from scrolling. While held, every scroll view
/// around it, the heatmap's own and the page's, stays put.
private struct HeatmapGestures: UIViewRepresentable {
    let tapped: (CGPoint) -> Void
    let held: (CGPoint?) -> Void

    func makeUIView(context: Context) -> UIView {
        let view = UIView()
        view.backgroundColor = .clear
        let hold = UILongPressGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.didHold(_:)))
        hold.minimumPressDuration = 0.2
        view.addGestureRecognizer(hold)
        let tap = UITapGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.didTap(_:)))
        // A hold that lifts in place isn't a tap.
        tap.require(toFail: hold)
        view.addGestureRecognizer(tap)
        return view
    }

    func updateUIView(_ view: UIView, context: Context) {
        context.coordinator.tapped = tapped
        context.coordinator.changed = held
    }

    static func dismantleUIView(_ view: UIView, coordinator: Coordinator) {
        coordinator.release()
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(tapped: tapped, changed: held)
    }

    @MainActor
    final class Coordinator: NSObject {
        var tapped: (CGPoint) -> Void
        var changed: (CGPoint?) -> Void
        /// The scroll views a hold stopped, to let go of as it ends.
        private var stopped: [UIScrollView] = []

        init(tapped: @escaping (CGPoint) -> Void, changed: @escaping (CGPoint?) -> Void) {
            self.tapped = tapped
            self.changed = changed
        }

        @objc func didTap(_ tap: UITapGestureRecognizer) {
            if tap.state == .ended { tapped(tap.location(in: tap.view)) }
        }

        @objc func didHold(_ hold: UILongPressGestureRecognizer) {
            switch hold.state {
            case .began:
                var view = hold.view?.superview
                while let current = view {
                    if let scroll = current as? UIScrollView, scroll.panGestureRecognizer.isEnabled {
                        scroll.panGestureRecognizer.isEnabled = false
                        stopped.append(scroll)
                    }
                    view = current.superview
                }
                changed(hold.location(in: hold.view))
            case .changed:
                changed(hold.location(in: hold.view))
            default:
                release()
                changed(nil)
            }
        }

        func release() {
            for scroll in stopped { scroll.panGestureRecognizer.isEnabled = true }
            stopped = []
        }
    }
}

// MARK: - The month

/// Lanes of bars a week shows; more Books than that on a day are counted.
private let maxLanes = 3

/// A month of reading: a bar on each day for every Book read that day, one
/// bar across consecutive days, in the Book's own color, and a check on the
/// day a Read-through finished.
private struct MonthGrid: View {
    let calendar: Shishoapp.StatsCalendarResponse
    let month: YearMonth
    let today: String
    let selectedDay: String?
    let colors: [String: String]
    let onSelect: (String) -> Void

    @Environment(\.horizontalSizeClass) private var sizeClass
    @ScaledMetric(relativeTo: .caption) private var dayRow: CGFloat = 24
    @ScaledMetric(relativeTo: .caption2) private var moreRow: CGFloat = 14

    /// A bar's height: a thin line on a phone, with its title on wider
    /// screens, as on the web.
    private var titled: Bool { sizeClass == .regular }
    private var laneHeight: CGFloat { titled ? 16 : 12 }
    private var barHeight: CGFloat { titled ? 16 : 6 }

    private var days: [String: Shishoapp.StatsCalendarDay] {
        Dictionary(calendar.days.map { ($0.day, $0) }, uniquingKeysWith: { first, _ in first })
    }

    private var refs: [String: Shishoapp.StatsBookRef] {
        Dictionary(calendar.books.map { ($0.key, $0) }, uniquingKeysWith: { first, _ in first })
    }

    var body: some View {
        let days = days
        let refs = refs
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 0) {
                ForEach(0..<7, id: \.self) { col in
                    Text(Calendar.current.shortStandaloneWeekdaySymbols[col])
                        .font(.geist(size: 11, relativeTo: .caption2))
                        .foregroundStyle(Theme.mutedForeground)
                        .frame(maxWidth: .infinity)
                }
            }
            .accessibilityHidden(true)
            VStack(spacing: 0) {
                let weeks = CalendarLayout.monthWeeks(year: month.year, month: month.month)
                ForEach(Array(weeks.enumerated()), id: \.offset) { index, week in
                    if index > 0 { Rectangle().fill(Theme.border).frame(height: 1) }
                    weekRow(week, days: days, refs: refs)
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 6))
            .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(Theme.border))
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("calendar.month")
            VStack(alignment: .leading, spacing: 4) {
                Text("Each book keeps its own color.")
                HStack(spacing: 4) {
                    FinishMarker(color: StatsColors.reading, check: .white, size: 14)
                    Text("marks the day you finished one.")
                }
                .accessibilityElement(children: .combine)
            }
            .font(.geist(.caption))
            .foregroundStyle(Theme.mutedForeground)
        }
    }

    private func weekRow(_ week: CalendarLayout.Week, days: [String: Shishoapp.StatsCalendarDay],
                         refs: [String: Shishoapp.StatsBookRef]) -> some View {
        let layout = CalendarLayout.layoutWeek(week, days: days, maxLanes: maxLanes)
        let height = dayRow + CGFloat(maxLanes) * (laneHeight + 2) + moreRow
        return HStack(spacing: 0) {
            ForEach(0..<7, id: \.self) { col in
                if col > 0 { Rectangle().fill(Theme.border).frame(width: 1) }
                dayCell(week[col], days: days, refs: refs)
            }
        }
        .frame(minHeight: height)
        .overlay {
            GeometryReader { geometry in
                let column = geometry.size.width / 7
                ZStack(alignment: .topLeading) {
                    ForEach(Array(layout.items.enumerated()), id: \.offset) { _, item in
                        bar(item, column: column, refs: refs)
                    }
                    ForEach(0..<7, id: \.self) { col in
                        if layout.hidden[col] > 0 {
                            Text(verbatim: "+\(layout.hidden[col])")
                                .font(.geist(size: 10, relativeTo: .caption2))
                                .foregroundStyle(Theme.mutedForeground)
                                .frame(width: column, height: moreRow, alignment: .leading)
                                .padding(.leading, 4)
                                .offset(x: CGFloat(col) * column, y: geometry.size.height - moreRow - 2)
                        }
                    }
                    if let selectedDay, let col = week.firstIndex(of: selectedDay) {
                        Rectangle()
                            .strokeBorder(Theme.primary, lineWidth: 2)
                            .frame(width: column, height: geometry.size.height)
                            .offset(x: CGFloat(col) * column)
                    }
                }
            }
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }
    }

    @ViewBuilder
    private func bar(_ item: CalendarLayout.WeekItem, column: CGFloat, refs: [String: Shishoapp.StatsBookRef]) -> some View {
        let hex = colors[item.key] ?? CalendarLayout.bookColor(item.key)
        let color = CalendarLayout.color(hex)
        let text = CalendarLayout.color(CalendarLayout.textOn(hex))
        let y = dayRow + CGFloat(item.lane) * (laneHeight + 2)
        let markerSize = titled ? 16.0 : 12.0
        if !item.markerOnly {
            let leading: CGFloat = item.continuesBefore ? 0 : 4
            let trailing: CGFloat = item.continuesAfter ? 0 : 4
            let width = CGFloat(item.end - item.start + 1) * column - leading - trailing
            UnevenRoundedRectangle(topLeadingRadius: item.continuesBefore ? 0 : 2, bottomLeadingRadius: item.continuesBefore ? 0 : 2,
                                   bottomTrailingRadius: item.continuesAfter ? 0 : 2, topTrailingRadius: item.continuesAfter ? 0 : 2)
                .fill(color)
                .overlay(alignment: .leading) {
                    if titled {
                        Text(CalendarText.title(refs[item.key] ?? Shishoapp.StatsBookRef(key: item.key, removed: true)))
                            .font(.geist(size: 10, relativeTo: .caption2, weight: .medium))
                            .foregroundStyle(text)
                            .lineLimit(1)
                            .padding(.horizontal, 4)
                    }
                }
                .frame(width: max(width, 2), height: barHeight)
                .offset(x: CGFloat(item.start) * column + leading, y: y + (laneHeight - barHeight) / 2)
        }
        ForEach(item.finishes, id: \.self) { col in
            FinishMarker(color: color, check: text, size: markerSize)
                .offset(x: CGFloat(col + 1) * column - markerSize - (titled ? 4 : 1), y: y + (laneHeight - markerSize) / 2)
        }
    }

    @ViewBuilder
    private func dayCell(_ day: String?, days: [String: Shishoapp.StatsCalendarDay],
                         refs: [String: Shishoapp.StatsBookRef]) -> some View {
        if let day {
            let books = days[day]?.books ?? []
            let isToday = day == today
            Button { onSelect(day) } label: {
                VStack(alignment: .leading, spacing: 0) {
                    Text(verbatim: String(Int(day.suffix(2)) ?? 0))
                        .font(.geist(size: 12, relativeTo: .caption, weight: isToday ? .semibold : .regular))
                        .monospacedDigit()
                        .foregroundStyle(isToday ? Theme.primaryForeground : Theme.mutedForeground)
                        .frame(minWidth: 20, minHeight: 20)
                        .padding(.horizontal, isToday ? 2 : 0)
                        .background {
                            if isToday { Capsule().fill(Theme.primary) }
                        }
                        .padding(.leading, 2)
                        .padding(.top, 2)
                    Spacer(minLength: 0)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .background(selectedDay == day ? Theme.secondary : Color.clear)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(label(day, books: books, refs: refs))
            .accessibilityAddTraits(selectedDay == day ? .isSelected : [])
            .accessibilityIdentifier("calendar.day.\(day)")
        } else {
            Theme.muted.opacity(0.3)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .accessibilityHidden(true)
        }
    }

    /// The day, its time, and each Book on it, marked when finished.
    private func label(_ day: String, books: [Shishoapp.StatsCalendarDayBook], refs: [String: Shishoapp.StatsBookRef]) -> String {
        guard !books.isEmpty else { return String(localized: "\(CalendarText.longDay(day)). No reading") }
        let seconds = books.reduce(0) { $0 + $1.seconds }
        var parts = [CalendarText.longDay(day)]
        if seconds > 0 { parts.append(FileFacts.duration(Double(seconds))) }
        for book in books {
            let title = CalendarText.title(refs[book.key] ?? Shishoapp.StatsBookRef(key: book.key, removed: true))
            parts.append(book.finished ? String(localized: "\(title), finished") : title)
        }
        return parts.joined(separator: ". ")
    }
}

/// A check in a Book's color on the day a Read-through of it finished.
private struct FinishMarker: View {
    let color: Color
    let check: Color
    let size: CGFloat

    var body: some View {
        Circle()
            .fill(color)
            .overlay {
                Icon(.check, size: size * 0.65, relativeTo: nil)
                    .foregroundStyle(check)
            }
            .frame(width: size, height: size)
            .background(Circle().stroke(Theme.card, lineWidth: 2))
            .accessibilityHidden(true)
    }
}

// MARK: - The day

/// One day's Reading Sessions and finishes, under the calendar.
private struct DayDetail: View {
    let day: String
    let detail: CalendarDayDetail?
    let waiting: Bool
    /// The day was never fetched, so it shows this device's reading alone.
    let deviceOnly: Bool
    let colors: [String: String]
    let books: [Int: BookSummary]
    let onClose: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .center, spacing: 8) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(CalendarText.longDay(day))
                        .font(.geist(.subheadline, weight: .semibold))
                        .accessibilityAddTraits(.isHeader)
                    if let detail = detail?.response, detail.day == day {
                        let total = detail.sessions.reduce(0) { $0 + $1.seconds }
                        if total > 0 {
                            Text("Total \(FileFacts.duration(Double(total)))")
                                .font(.geist(.caption))
                                .foregroundStyle(Theme.mutedForeground)
                        }
                    }
                }
                Spacer(minLength: 8)
                Button(action: onClose) {
                    Icon(.x, size: 16, relativeTo: .subheadline)
                        .foregroundStyle(Theme.mutedForeground)
                        .frame(minWidth: 44, minHeight: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(String(localized: "Close day"))
                .accessibilityIdentifier("calendar.detail.close")
            }
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("calendar.detail")
    }

    @ViewBuilder
    private var content: some View {
        if let shown = self.detail, shown.response.day == day {
            let detail = shown.response
            let zone = TimeZone(identifier: detail.timeZone) ?? .gmt
            let lanes = CalendarLayout.dayLanes(detail.sessions, day: day, zone: zone)
            if deviceOnly { DeviceOnlyNote() }
            if detail.sessions.isEmpty, detail.finished.isEmpty {
                Text("No reading on this day.")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
            } else {
                if !detail.finished.isEmpty {
                    VStack(alignment: .leading, spacing: 6) {
                        ForEach(detail.finished, id: \.key) { ref in
                            let pending = shown.pendingFinished.contains(ref.key)
                            HStack(spacing: 8) {
                                Text("Finished")
                                    .font(.geist(.caption))
                                    .foregroundStyle(Theme.mutedForeground)
                                    .accessibilityValue(pending ? String(localized: "Not synced yet") : "")
                                bookName(ref)
                                if pending {
                                    PendingMark()
                                        .accessibilityHidden(true)
                                }
                            }
                        }
                    }
                }
                if !lanes.isEmpty {
                    DayTimeline(lanes: lanes, colors: colors)
                    VStack(alignment: .leading, spacing: 10) {
                        ForEach(lanes) { lane in
                            laneCard(lane, zone: zone, pending: shown.pendingSessions)
                        }
                    }
                }
            }
        } else if waiting {
            ProgressView()
                .frame(maxWidth: .infinity, minHeight: 60)
        } else {
            Text("This day shows once the app can reach your server.")
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
                .accessibilityIdentifier("calendar.dayUnavailable")
        }
    }

    private func color(_ key: String) -> Color {
        CalendarLayout.color(colors[key] ?? CalendarLayout.bookColor(key))
    }

    private func laneCard(_ lane: CalendarLayout.DayLane, zone: TimeZone, pending: Set<Int>) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 8) {
                bookName(lane.book)
                Spacer(minLength: 8)
                Text(FileFacts.duration(Double(lane.seconds)))
                    .font(.geist(.caption))
                    .monospacedDigit()
                    .foregroundStyle(Theme.mutedForeground)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            ForEach(lane.sessions) { placed in
                Rectangle().fill(Theme.border).frame(height: 1)
                sessionRow(placed.session, zone: zone, pending: pending.contains(placed.session.id))
            }
        }
        .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(Theme.border))
    }

    /// A Session's times, time on the day, progress, and where it was
    /// read, marked when it's this device's and not synced yet.
    private func sessionRow(_ session: Shishoapp.StatsDaySession, zone: TimeZone, pending: Bool) -> some View {
        let when = String(localized: "\(CalendarText.time(session.startedAt, day: day, zone: zone)) to \(CalendarText.time(session.endedAt, day: day, zone: zone))")
        let text = [when, FileFacts.duration(Double(session.seconds)), CalendarText.progress(session), CalendarText.source(session)]
            .joined(separator: " · ")
        return HStack(alignment: .firstTextBaseline, spacing: 6) {
            Text(text)
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
                .frame(maxWidth: .infinity, alignment: .leading)
            if pending { PendingMark() }
        }
        .padding(.leading, 30)
        .padding(.trailing, 12)
        .padding(.vertical, 6)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(text)
        .accessibilityValue(pending ? String(localized: "Not synced yet") : "")
        // A Session not synced yet has no id on the server; its own is
        // negative, so it's named apart.
        .accessibilityIdentifier(pending ? "calendar.session.pending.\(-session.id)" : "calendar.session.\(session.id)")
    }

    /// A Book's swatch and title, opening its page when it's in the
    /// catalog, and why it can't when it was removed or is out of reach.
    @ViewBuilder
    private func bookName(_ ref: Shishoapp.StatsBookRef) -> some View {
        let summary = ref.bookId.flatMap { books[$0] }
        HStack(spacing: 8) {
            RoundedRectangle(cornerRadius: 2)
                .fill(color(ref.key))
                .frame(width: 10, height: 10)
                .accessibilityHidden(true)
            BookLink(summary: summary, title: CalendarText.title(ref))
            if ref.removed, !ref.title.isEmpty {
                badge(String(localized: "Removed from library"))
            } else if !ref.removed, ref.bookId == nil {
                badge(String(localized: "No access to this library"))
            }
        }
    }

    private func badge(_ text: String) -> some View {
        Text(text)
            .font(.geist(size: 11, relativeTo: .caption2))
            .foregroundStyle(Theme.mutedForeground)
            .lineLimit(1)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .overlay(Capsule().strokeBorder(Theme.border))
            .layoutPriority(1)
    }
}

/// Says a part of the calendar the app never fetched shows only this
/// device's reading not synced yet, until the app syncs.
private struct DeviceOnlyNote: View {
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            PendingMark()
                .accessibilityHidden(true)
            Text("Only this device's reading sessions show here until the app syncs.")
                .font(.geist(.caption))
                .foregroundStyle(Theme.mutedForeground)
                .fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("calendar.deviceOnly")
    }
}

/// A Book's title, opening its page when it's in the catalog.
private struct BookLink: View {
    @Environment(AppModel.self) private var model
    let summary: BookSummary?
    let title: String

    var body: some View {
        if let summary {
            Button { model.push(.book(summary)) } label: {
                Text(title)
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(1)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("calendar.book.\(summary.id)")
        } else {
            Text(title)
                .font(.geist(.subheadline, weight: .medium))
                .foregroundStyle(Theme.foreground)
                .lineLimit(1)
                .frame(minHeight: 28)
        }
    }
}

/// A day as a 24-hour strip, a line per Book with a block for each Session
/// at the time of day it ran. It's drawn for sighted readers; the list under
/// it carries every Session's times.
private struct DayTimeline: View {
    let lanes: [CalendarLayout.DayLane]
    let colors: [String: String]

    private let hours = [0, 3, 6, 9, 12, 15, 18, 21, 24]
    @ScaledMetric(relativeTo: .caption2) private var labelHeight: CGFloat = 14

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            GeometryReader { geometry in
                ForEach(hours.filter { $0 % 6 == 0 }, id: \.self) { hour in
                    Text(CalendarText.hour(hour))
                        .font(.geist(size: 10, relativeTo: .caption2))
                        .foregroundStyle(Theme.mutedForeground)
                        .fixedSize()
                        .frame(width: 60, alignment: hour == 0 ? .leading : hour == 24 ? .trailing : .center)
                        .offset(x: geometry.size.width * CGFloat(hour) / 24 - (hour == 0 ? 0 : hour == 24 ? 60 : 30))
                }
            }
            .frame(height: labelHeight)
            VStack(spacing: 6) {
                ForEach(lanes) { lane in
                    GeometryReader { geometry in
                        let width = geometry.size.width
                        ZStack(alignment: .leading) {
                            RoundedRectangle(cornerRadius: 2).fill(Theme.muted.opacity(0.4))
                            ForEach(lane.sessions) { placed in
                                RoundedRectangle(cornerRadius: 2)
                                    .fill(CalendarLayout.color(colors[lane.book.key] ?? CalendarLayout.bookColor(lane.book.key)))
                                    .frame(width: max(3, width * CGFloat(placed.to - placed.from) / CalendarLayout.minutesInDay))
                                    .offset(x: width * CGFloat(placed.from) / CalendarLayout.minutesInDay)
                            }
                        }
                    }
                    .frame(height: 12)
                }
            }
            .padding(.vertical, 4)
            .background {
                GeometryReader { geometry in
                    ForEach(hours, id: \.self) { hour in
                        Rectangle()
                            .fill(Theme.border)
                            .frame(width: 1, height: geometry.size.height)
                            .offset(x: min(geometry.size.width - 1, geometry.size.width * CGFloat(hour) / 24))
                    }
                }
            }
        }
        .accessibilityHidden(true)
    }
}
