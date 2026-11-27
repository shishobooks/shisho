import GRDB
import ShishoAPI
import SwiftUI

/// What the Book page's reading controls show, read from the device.
struct ReadingControlsState: Equatable, Sendable {
    var status: String?
    var rating: Int?
    var underway: (id: Int, pending: Bool)?
    var readBefore = false
    var toReadPlace: Int?
    /// A change to the Book's Read-throughs the server hasn't confirmed.
    var pending = false

    static func == (a: Self, b: Self) -> Bool {
        a.status == b.status && a.rating == b.rating && a.underway?.id == b.underway?.id && a.underway?.pending == b.underway?.pending
            && a.readBefore == b.readBefore && a.toReadPlace == b.toReadPlace && a.pending == b.pending
    }

    static func fetch(_ db: Database, bookID: Int) throws -> ReadingControlsState {
        let standing = try CatalogStore.standing(db, bookID: bookID)
        return ReadingControlsState(status: standing.status, rating: standing.rating,
                                    underway: try CatalogStore.underwayReadThrough(db, bookID: bookID),
                                    readBefore: try CatalogStore.readBefore(db, bookID: bookID),
                                    toReadPlace: try CatalogStore.toReadPlace(db, bookID: bookID),
                                    pending: try pendingStatus(db, bookID: bookID))
    }

    /// The Book's status has a change the server hasn't confirmed: a
    /// Read-through not confirmed, or a change to its status not sent or not
    /// back in the feed yet. The second covers Mark as Reference and Mark as
    /// not started, which delete the Read-through the first would find.
    static func pendingStatus(_ db: Database, bookID: Int) throws -> Bool {
        let types = statusTypes.map { "'\($0.rawValue)'" }.joined(separator: ",")
        return try Bool.fetchOne(db, sql: """
            SELECT EXISTS (SELECT 1 FROM read_throughs WHERE book_id = ?1 AND pending)
                OR EXISTS (SELECT 1 FROM journal WHERE book_id = ?1 AND type IN (\(types)))
                OR EXISTS (SELECT 1 FROM journal_applied WHERE book_id = ?1 AND type IN (\(types)))
            """, arguments: [bookID]) ?? false
    }

    /// The changes that set a Book's Reading Status, To Read's among them.
    private static let statusTypes: [Shishoapp.JournalEventType] = [
        .readThroughStart, .readThroughFinish, .readThroughAbandon, .readThroughPause, .readThroughResume,
        .readThroughDelete, .referenceMark, .referenceRemove, .toReadAdd, .toReadRemove,
    ]
}

/// The transitions a Reading Status offers, as the web's status menu does
/// (`readingTransitions`): starting is "Read again" once the Book was read.
enum ReadingTransitions {
    enum Transition: String, CaseIterable {
        case start, pause, resume, finish, abandon
    }

    static func available(_ status: String?) -> [Transition] {
        switch status {
        case "currently_reading": [.pause, .finish, .abandon]
        case "paused": [.resume, .finish, .abandon]
        default: [.start, .finish]
        }
    }

    /// A Book being read can't go on To Read: starting takes it off.
    static func canQueue(_ status: String?) -> Bool {
        status != "currently_reading" && status != "paused"
    }

    static func label(_ transition: Transition, status: String?, readBefore: Bool) -> String {
        switch transition {
        case .start:
            status == "read" || status == "did_not_finish" || (status == "reference" && readBefore)
                ? String(localized: "Read again") : String(localized: "Start reading")
        case .pause: String(localized: "Pause")
        case .resume: String(localized: "Resume")
        case .finish: String(localized: "Mark as read")
        case .abandon: String(localized: "Mark as Did Not Finish")
        }
    }

    static func change(_ transition: Transition, book: Int) -> ReadingChange {
        switch transition {
        case .start: .start(book: book)
        case .pause: .pause(book: book)
        case .resume: .resume(book: book)
        case .finish: .finish(book: book)
        case .abandon: .abandon(book: book)
        }
    }
}

/// The user's Reading Status, Rating, and place on To Read for a Book,
/// changed here as on the web's Book page, online or not. Each control
/// shows only for a server that takes its changes (ADR 0024).
struct BookReadingControls: View {
    @Environment(AppModel.self) private var model
    let bookID: Int
    let title: String

    @State private var state: ReadingControlsState?
    @State private var confirmingReference = false
    @State private var confirmingNotStarted = false
    @State private var addingToList = false

    private var status: String? { state?.status }
    private var canChangeStatus: Bool { model.offers(.readingStatus) }
    private var canQueue: Bool { model.offers(.toRead) && ReadingTransitions.canQueue(status) }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Side by side when they fit, the Rating under the status when
            // they don't, as at the largest text sizes.
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 6) {
                    statusControl
                    Spacer(minLength: 0)
                    ratingControl
                }
                VStack(alignment: .leading, spacing: 6) {
                    statusControl
                    ratingControl
                }
            }
            HStack(spacing: 14) {
                if let place = state?.toReadPlace {
                    Button { model.push(.toRead) } label: {
                        Text("#\(place) on To Read")
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            .frame(minHeight: 44)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("book.detail.toReadPlace")
                }
                if model.offers(.lists) {
                    Button { addingToList = true } label: {
                        Label("Add to List", icon: .listPlus, size: 14, relativeTo: .footnote)
                            .font(.geist(.footnote, weight: .medium))
                            .foregroundStyle(Theme.foreground)
                            .frame(minHeight: 44)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("book.detail.addToList")
                }
            }
        }
        .observing(model.session?.store, id: bookID, into: $state) { [bookID] db in
            try ReadingControlsState.fetch(db, bookID: bookID)
        }
        .alert(String(localized: "Mark as Reference?"), isPresented: $confirmingReference) {
            Button(String(localized: "Mark as Reference"), role: .destructive) { model.change(.markReference(book: bookID)) }
                .accessibilityIdentifier("reference.confirm")
            Button(String(localized: "Cancel"), role: .cancel) {}
                .accessibilityIdentifier("reference.cancel")
        } message: {
            Text("Your current Read-through of this book is deleted from your reading history, and this can't be undone. The reading time recorded during it still counts toward your Reading Stats, and every file keeps its place. While the book is Reference, opening it never starts or finishes a Read-through.")
        }
        .alert(String(localized: "Mark as not started?"), isPresented: $confirmingNotStarted) {
            Button(String(localized: "Mark as not started"), role: .destructive) { model.change(.notStarted(book: bookID)) }
                .accessibilityIdentifier("notStarted.confirm")
            Button(String(localized: "Cancel"), role: .cancel) {}
                .accessibilityIdentifier("notStarted.cancel")
        } message: {
            Text("Your current Read-through is deleted along with the reading time and places recorded during it, and every main file opens at the start again. If starting it took the book off To Read, it goes back to its place there.")
        }
        .sheet(isPresented: $addingToList) {
            AddToListSheet(bookID: bookID, title: title)
        }
    }

    private var ratingControl: some View {
        RatingControl(units: state?.rating, editable: model.offers(.ratings), title: title) { units in
            model.change(.rating(book: bookID, units: units))
        }
    }

    @ViewBuilder
    private var statusControl: some View {
        if canChangeStatus || canQueue {
            Menu {
                if canQueue {
                    if status == "to_read" {
                        Button(String(localized: "Remove from To Read")) { model.change(.removeFromToRead(book: bookID)) }
                            .accessibilityIdentifier("status.toRead.remove")
                    } else {
                        Button(String(localized: "Add to To Read")) { model.change(.addToRead(book: bookID)) }
                            .accessibilityIdentifier("status.toRead.add")
                    }
                    Divider()
                }
                if canChangeStatus {
                    ForEach(ReadingTransitions.available(status), id: \.self) { transition in
                        Button(ReadingTransitions.label(transition, status: status, readBefore: state?.readBefore ?? false)) {
                            model.change(ReadingTransitions.change(transition, book: bookID))
                        }
                        .accessibilityIdentifier("status.\(transition.rawValue)")
                    }
                    Divider()
                    if status == "reference" {
                        Button(String(localized: "Remove from Reference")) { model.change(.removeReference(book: bookID)) }
                            .accessibilityIdentifier("status.reference.remove")
                    } else {
                        Button(String(localized: "Mark as Reference")) {
                            if state?.underway != nil {
                                confirmingReference = true
                            } else {
                                model.change(.markReference(book: bookID))
                            }
                        }
                        .accessibilityIdentifier("status.reference.mark")
                    }
                    if state?.underway != nil {
                        Button(String(localized: "Mark as not started"), role: .destructive) { confirmingNotStarted = true }
                            .accessibilityIdentifier("status.notStarted")
                    }
                }
            } label: {
                statusLabel(chevron: true)
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityLabel(String(localized: "Reading status: \(status.map(ReadingStatusLabel.text) ?? String(localized: "Not started"))"))
            .accessibilityValue(state?.pending == true ? String(localized: "Not synced yet") : "")
            .accessibilityIdentifier("book.detail.status")
        } else if status != nil {
            statusLabel(chevron: false)
                .font(.geist(.footnote, weight: .medium))
                .padding(.horizontal, 12)
                .frame(minHeight: 34)
                .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(String(localized: "Reading status: \(status.map(ReadingStatusLabel.text) ?? "")"))
                .accessibilityValue(state?.pending == true ? String(localized: "Not synced yet") : "")
                .accessibilityIdentifier("book.detail.status")
        }
    }

    /// The status as the Library's picker shows its Library: in a small
    /// outlined box (SmallOutlineStyle).
    private func statusLabel(chevron: Bool) -> some View {
        HStack(spacing: 6) {
            Icon(StatusStyle.icon(status), size: 14, relativeTo: .footnote)
                .foregroundStyle(status.map(StatusStyle.color) ?? Theme.mutedForeground)
            Text(status.map(ReadingStatusLabel.text) ?? String(localized: "Not started"))
                .foregroundStyle(Theme.foreground)
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
            if chevron {
                Icon(.chevronDown, size: 12, relativeTo: .footnote)
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityHidden(true)
            }
            if state?.pending == true {
                PendingMark()
            }
        }
    }
}

/// The small mark on something the server hasn't confirmed yet, as Reading
/// History marks reading still on the device. It's drawn only: what it sits
/// in says "Not synced yet" to VoiceOver, since the icon is hidden from it.
struct PendingMark: View {
    var body: some View {
        Icon(.cloudUpload, size: 13, relativeTo: .caption)
            .foregroundStyle(Theme.mutedForeground)
    }
}

/// A Rating in stars, in quarter steps as on the web. The stars open a
/// sheet to rate, change, or clear it in, with room for quarter stars that
/// a row this size can't give a finger (RatingSheet). Read only for a
/// server that doesn't take Ratings from the app.
struct RatingControl: View {
    /// Quarter stars, 1 to 20, or nil when unrated.
    let units: Int?
    let editable: Bool
    /// The Book's title, for the sheet.
    let title: String
    let set: (Int?) -> Void

    @State private var rating = false
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .body) private var starSize: CGFloat = 22

    var body: some View {
        if !editable {
            if let units { RatingStars(units: units) }
        } else {
            Button { rating = true } label: {
                PreciseStars(units: units ?? 0, size: starSize)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(String(localized: "Rating"))
            .accessibilityValue(units.map { String(localized: "\(Ratings.text($0)) of 5 stars") } ?? String(localized: "Not rated"))
            .accessibilityHint(String(localized: "Opens a picker that rates in quarter stars."))
            .accessibilityIdentifier("book.detail.rating")
            .sheet(isPresented: $rating) {
                RatingSheet(title: title, initial: units, set: set)
                    // Room for the stars and the steps, and all the screen
                    // at the largest text sizes.
                    .presentationDetents(dynamicTypeSize.isAccessibilitySize ? [.large] : [.height(330), .large])
                    .presentationDragIndicator(.visible)
            }
        }
    }
}

/// Five stars filled to a Rating's exact quarter, as the web's StarRating
/// draws them, rather than rounded to halves.
struct PreciseStars: View {
    /// Quarter stars, 0 to 20.
    let units: Int
    let size: CGFloat
    var spacing: CGFloat = 2

    var body: some View {
        HStack(spacing: spacing) {
            ForEach(0..<5, id: \.self) { index in
                let fill = min(1, max(0, Double(units) / 4 - Double(index)))
                ZStack(alignment: .leading) {
                    Icon(.star, size: size, relativeTo: nil)
                        .foregroundStyle(Theme.rating.opacity(0.55))
                    Icon(.starFill, size: size, relativeTo: nil)
                        .foregroundStyle(Theme.rating)
                        .mask(alignment: .leading) {
                            Rectangle().frame(width: size * fill)
                        }
                }
                .frame(width: size, height: size)
            }
        }
        .accessibilityHidden(true)
    }
}

/// The rules of the Rating sheet's stars, in quarter-star units, 1 to 20,
/// and 0 for none.
enum RatingRules {
    /// The quarter star at fraction of the way across the stars: never
    /// none, so a drag back past the first star doesn't unrate by accident.
    static func units(at fraction: Double) -> Int {
        max(1, min(20, Int((min(1, max(0, fraction)) * 20).rounded(.up))))
    }

    /// value moved by a quarter star at a time: below a quarter is none,
    /// and five stars is the most.
    static func step(_ value: Int, by: Int) -> Int {
        let next = value + by
        return next < 1 ? 0 : min(20, next)
    }
}

/// Where a Rating is set in quarter stars: big stars to tap or drag across,
/// a tick under the finger at each quarter, the number as it changes, and
/// steps of a quarter either way. A sheet, since nothing scrolls under a
/// drag here and the stars can be large enough for a finger. Done saves it;
/// Cancel, or swiping the sheet away, leaves the Rating as it was.
struct RatingSheet: View {
    let title: String
    let initial: Int?
    let set: (Int?) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var value: Int
    @ScaledMetric(relativeTo: .largeTitle) private var starSize: CGFloat = 42
    /// The width the stars have, which caps how big each can be.
    @State private var width: CGFloat = 0
    private let spacing: CGFloat = 6

    init(title: String, initial: Int?, set: @escaping (Int?) -> Void) {
        self.title = title
        self.initial = initial
        self.set = set
        _value = State(initialValue: initial ?? 0)
    }

    /// As big as the text size asks, but never wider than the sheet.
    private var size: CGFloat {
        guard width > 0 else { return starSize }
        return min(starSize, (width - spacing * 4) / 5)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 18) {
                    Text(value == 0 ? String(localized: "Not rated") : Ratings.text(value))
                        .font(.geist(size: value == 0 ? 22 : 40, relativeTo: .largeTitle, weight: .bold))
                        .monospacedDigit()
                        .foregroundStyle(value == 0 ? Theme.mutedForeground : Theme.foreground)
                        .contentTransition(.numericText(value: Double(value)))
                        .frame(minHeight: 48)
                        .accessibilityHidden(true)
                    PreciseStars(units: value, size: size, spacing: spacing)
                        .contentShape(Rectangle())
                        .gesture(DragGesture(minimumDistance: 0).onChanged { drag in
                            let units = RatingRules.units(at: drag.location.x / (size * 5 + spacing * 4))
                            if units != value { value = units }
                        })
                        .accessibilityElement()
                        .accessibilityLabel(String(localized: "Rating"))
                        .accessibilityValue(value == 0 ? String(localized: "Not rated")
                            : String(localized: "\(Ratings.text(value)) of 5 stars"))
                        .accessibilityAdjustableAction { direction in
                            switch direction {
                            case .increment: value = RatingRules.step(value, by: 1)
                            case .decrement: value = RatingRules.step(value, by: -1)
                            @unknown default: break
                            }
                        }
                        .accessibilityIdentifier("rating.stars")
                    HStack(spacing: 28) {
                        stepButton(-1, icon: .minus, label: String(localized: "A quarter star less"), identifier: "rating.less")
                        Text("Tap or drag the stars")
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            .multilineTextAlignment(.center)
                            .accessibilityHidden(true)
                        stepButton(1, icon: .plus, label: String(localized: "A quarter star more"), identifier: "rating.more")
                    }
                }
                .frame(maxWidth: .infinity)
                .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { width = $0 }
                .padding(.horizontal, 20)
                .padding(.top, 8)
            }
            .scrollBounceBehavior(.basedOnSize)
            .sensoryFeedback(.selection, trigger: value)
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "Cancel")) { dismiss() }
                        .accessibilityIdentifier("rating.cancel")
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Done")) {
                        let chosen = value == 0 ? nil : value
                        if chosen != initial { set(chosen) }
                        dismiss()
                    }
                    .fontWeight(.semibold)
                    .accessibilityIdentifier("rating.done")
                }
                ToolbarItem(placement: .bottomBar) {
                    if value != 0 {
                        Button(String(localized: "Clear Rating"), role: .destructive) { value = 0 }
                            .accessibilityIdentifier("rating.clearInSheet")
                    }
                }
            }
        }
    }

    private func stepButton(_ by: Int, icon: Lucide, label: String, identifier: String) -> some View {
        Button { value = RatingRules.step(value, by: by) } label: {
            Icon(icon, size: 18, relativeTo: .body)
                .foregroundStyle(Theme.foreground)
                .frame(width: 44, height: 44)
                .background(Theme.secondary, in: Circle())
        }
        .buttonStyle(.plain)
        .disabled(by > 0 ? value >= 20 : value <= 0)
        .accessibilityLabel(label)
        .accessibilityIdentifier(identifier)
    }
}

/// The Lists a Book can go in, each with a check when it's there, and a new
/// List made right here.
struct AddToListSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let bookID: Int
    let title: String

    @State private var choices: [ListChoice]?
    @State private var making = false

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Button { making = true } label: {
                        Label("New List", icon: .plus, size: 16, relativeTo: .body)
                            .font(.geist(.body, weight: .medium))
                            .foregroundStyle(Theme.primary)
                    }
                    .accessibilityIdentifier("addToList.new")
                }
                Section {
                    ForEach(choices ?? []) { choice in
                        Button {
                            model.change(choice.contains ? .removeFromList(id: choice.id, book: bookID) : .addToList(id: choice.id, book: bookID))
                        } label: {
                            HStack {
                                Text(choice.name)
                                    .font(.geist(.body))
                                    .foregroundStyle(Theme.foreground)
                                Spacer()
                                if choice.contains {
                                    Icon(.check, size: 16, relativeTo: .body)
                                        .foregroundStyle(Theme.primary)
                                }
                            }
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .accessibilityAddTraits(choice.contains ? .isSelected : [])
                        .accessibilityIdentifier("addToList.row")
                    }
                } footer: {
                    if choices?.isEmpty == true {
                        Text("Lists you can add Books to appear here.")
                    }
                }
            }
            .navigationTitle(String(localized: "Add to List"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Done")) { dismiss() }
                        .accessibilityIdentifier("addToList.done")
                }
            }
            .sheet(isPresented: $making) {
                ListFormSheet(title: String(localized: "New List")) { name, ordered in
                    if let id = model.change(.createList(name: name, description: nil, ordered: ordered)) {
                        model.change(.addToList(id: id, book: bookID))
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .observing(model.session?.store, id: bookID, into: $choices) { [bookID] db in
            try CatalogStore.listChoices(db, bookID: bookID)
        }
    }
}

/// Names a List, new or renamed. A new List also picks whether its Books
/// keep an order of their own.
struct ListFormSheet: View {
    @Environment(\.dismiss) private var dismiss
    let title: String
    var name = ""
    /// Whether to offer the ordered choice, which only a new List makes.
    var asksOrdered = true
    let save: (String, Bool) -> Void

    @State private var entered = ""
    @State private var ordered = false
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                TextField(String(localized: "Name"), text: $entered)
                    .font(.geist(.body))
                    .focused($focused)
                    .submitLabel(.done)
                    .onSubmit(submit)
                    .accessibilityIdentifier("listForm.name")
                if asksOrdered {
                    Toggle(String(localized: "Keep Books in my own order"), isOn: $ordered)
                        .font(.geist(.body))
                        .tint(Theme.primary)
                        .accessibilityIdentifier("listForm.ordered")
                }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "Cancel")) { dismiss() }
                        .accessibilityIdentifier("listForm.cancel")
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Save"), action: submit)
                        .disabled(trimmed.isEmpty)
                        .accessibilityIdentifier("listForm.save")
                }
            }
        }
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .onAppear {
            entered = name
            focused = true
        }
    }

    private var trimmed: String { entered.trimmingCharacters(in: .whitespacesAndNewlines) }

    private func submit() {
        guard !trimmed.isEmpty else { return }
        save(String(trimmed.prefix(200)), ordered)
        dismiss()
    }
}

/// Clears the user's saved place in a File, after asking, as the web's
/// file menu does: its readers open at the start, and the status and
/// Reading History stay as they are.
struct ClearPositionConfirmation: ViewModifier {
    @Environment(AppModel.self) private var model
    @Binding var file: BookDetail.File?

    func body(content: Content) -> some View {
        content.alert(String(localized: "Clear saved position?"), isPresented: Binding(get: { file != nil }, set: { if !$0 { file = nil } }),
                      presenting: file) { file in
            Button(String(localized: "Clear"), role: .destructive) { model.change(.clearPosition(file: file.id)) }
                .accessibilityIdentifier("clearPosition.confirm")
            Button(String(localized: "Cancel"), role: .cancel) {}
                .accessibilityIdentifier("clearPosition.cancel")
        } message: { file in
            Text("\(file.label) will open at the start. Your reading history and status stay as they are.")
        }
    }
}

extension View {
    /// Asks to clear file's saved place while it's set.
    func clearPositionConfirmation(_ file: Binding<BookDetail.File?>) -> some View {
        modifier(ClearPositionConfirmation(file: file))
    }
}
