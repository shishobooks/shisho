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
                                    pending: try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM read_throughs WHERE book_id = ? AND pending)",
                                                               arguments: [bookID]) ?? false)
    }
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
            HStack(spacing: 10) {
                statusControl
                Spacer(minLength: 8)
                RatingControl(units: state?.rating, editable: model.offers(.ratings)) { units in
                    model.change(.rating(book: bookID, units: units))
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
            .accessibilityLabel(String(localized: "Reading status: \(status.map(ReadingStatusLabel.text) ?? String(localized: "Not started"))"))
            .accessibilityValue(state?.pending == true ? String(localized: "Not synced yet") : "")
            .accessibilityIdentifier("book.detail.status")
        } else if status != nil {
            statusLabel(chevron: false)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(String(localized: "Reading status: \(status.map(ReadingStatusLabel.text) ?? "")"))
                .accessibilityValue(state?.pending == true ? String(localized: "Not synced yet") : "")
                .accessibilityIdentifier("book.detail.status")
        }
    }

    private func statusLabel(chevron: Bool) -> some View {
        HStack(spacing: 6) {
            Icon(StatusStyle.icon(status), size: 14, relativeTo: .footnote)
                .foregroundStyle(status.map(StatusStyle.color) ?? Theme.mutedForeground)
            Text(status.map(ReadingStatusLabel.text) ?? String(localized: "Not started"))
                .font(.geist(.footnote, weight: .medium))
                .foregroundStyle(Theme.foreground)
            if chevron {
                Icon(.chevronDown, size: 12, relativeTo: .footnote)
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityHidden(true)
            }
            if state?.pending == true {
                PendingMark()
            }
        }
        .padding(.horizontal, 12)
        .frame(minHeight: 44)
        .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
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

/// A Rating in stars, set in quarter steps as on the web: a tap sets whole
/// stars, a drag across sets quarters, and VoiceOver adjusts it a quarter at
/// a time. Read only for a server that doesn't take Ratings from the app.
struct RatingControl: View {
    /// Quarter stars, 1 to 20, or nil when unrated.
    let units: Int?
    let editable: Bool
    let set: (Int?) -> Void

    @State private var dragging: Int?
    @ScaledMetric(relativeTo: .body) private var starSize: CGFloat = 22

    private var shown: Int { dragging ?? units ?? 0 }

    var body: some View {
        if !editable {
            if let units { RatingStars(units: units) }
        } else {
            HStack(spacing: 4) {
                stars
                if units != nil {
                    Button { set(nil) } label: {
                        Icon(.x, size: 14, relativeTo: nil)
                            .foregroundStyle(Theme.mutedForeground)
                            .frame(width: 44, height: 44)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(String(localized: "Clear rating"))
                    .accessibilityIdentifier("rating.clear")
                }
            }
        }
    }

    private var stars: some View {
        let spacing: CGFloat = 2
        return HStack(spacing: spacing) {
            ForEach(0..<5, id: \.self) { index in
                let fill = min(1, max(0, Double(shown) / 4 - Double(index)))
                ZStack {
                    Icon(fill >= 0.75 ? .starFill : .star, size: starSize, relativeTo: nil)
                    if fill >= 0.25, fill < 0.75 {
                        Icon(.starHalfFill, size: starSize, relativeTo: nil)
                    }
                }
                .foregroundStyle(Theme.rating)
                .frame(minHeight: 44)
            }
        }
        .contentShape(Rectangle())
        .gesture(DragGesture(minimumDistance: 0)
            .onChanged { value in dragging = units(at: value.location.x, spacing: spacing) }
            .onEnded { value in
                let picked = units(at: value.location.x, spacing: spacing)
                dragging = nil
                // A tap lands on whole stars; a drag keeps its quarters.
                let tapped = abs(value.translation.width) < 4
                set(tapped ? max(4, Int((Double(picked) / 4).rounded(.up)) * 4) : picked)
            })
        .accessibilityElement()
        .accessibilityLabel(String(localized: "Rating"))
        .accessibilityValue(units.map { String(localized: "\((Double($0) / 4).formatted(.number.precision(.fractionLength(0...2)))) of 5 stars") }
            ?? String(localized: "Not rated"))
        .accessibilityAdjustableAction { direction in
            switch direction {
            case .increment: set(min(20, (units ?? 0) + 1))
            case .decrement: set((units ?? 0) <= 1 ? nil : (units ?? 0) - 1)
            @unknown default: break
            }
        }
        .accessibilityIdentifier("book.detail.rating")
    }

    /// The quarter stars at x across the stars, 1 to 20.
    private func units(at x: CGFloat, spacing: CGFloat) -> Int {
        let width = starSize * 5 + spacing * 4
        let fraction = min(1, max(0, x / width))
        return max(1, min(20, Int((fraction * 20).rounded(.up))))
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
