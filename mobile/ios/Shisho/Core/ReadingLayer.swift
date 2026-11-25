import Foundation

/// The user's reading record for some Books and Lists: as the feed last
/// sent it, or with the changes the server hasn't confirmed laid over it.
/// Screens read the second, so a change made offline shows at once and a
/// change the server refuses falls away (ADR 0024).
struct ReadingRecord: Equatable, Sendable {
    struct UserBook: Equatable, Sendable {
        /// Quarter stars, 1 to 20, or nil when unrated.
        var rating: Int?
        var reference = false
        var pageFlow: String?
    }

    struct ReadThrough: Equatable, Sendable {
        let id: Int
        var bookID: Int?
        var beginDay: String?
        var endDay: String?
        var outcome: String
        var paused = false
        var lastReadAt: Double?
        var bookTitle: String?
        var bookAuthors: String?
        /// A change the server hasn't confirmed made or changed it.
        var pending = false
        /// A change the server hasn't confirmed ended it, so Reading Stats
        /// counts it before the next sync does.
        var endedHere = false
    }

    struct ListBook: Equatable, Sendable {
        let bookID: Int
        var sortOrder: Int?
        var addedAt: Double
    }

    struct List: Equatable, Sendable {
        let id: Int
        /// The id this installation made it under, for one it made.
        var localID: String?
        var name: String
        var description: String?
        var isOrdered: Bool
        var defaultSort: String
        var ownerID: Int
        var ownerName: String
        var permission: String
        var shares: String
        /// In the List's order.
        var books: [ListBook]
        var pending = false
    }

    var userBooks: [Int: UserBook] = [:]
    /// The Books on To Read, in order.
    var toRead: [Int] = []
    var readThroughs: [Int: ReadThrough] = [:]
    var lists: [Int: List] = [:]
}

/// What a File is, for working out what a place in it starts or finishes.
struct LayerFile: Equatable, Sendable {
    let bookID: Int
    let supplement: Bool
    var pageCount: Int?
    var duration: Double?
}

/// One change not yet confirmed, in the order the user made it.
struct LayerEvent: Equatable, Sendable {
    enum Change: Equatable, Sendable {
        case start(book: Int, day: String)
        case finish(book: Int, day: String)
        case abandon(book: Int, day: String)
        case pause(book: Int)
        case resume(book: Int)
        /// A Read-through by id, or the one underway in book.
        case deleteReadThrough(id: Int?, book: Int?)
        case referenceMark(book: Int)
        case referenceRemove(book: Int)
        case rating(book: Int, units: Int?)
        case toReadAdd(book: Int)
        case toReadRemove(book: Int)
        case toReadMove(book: Int, after: Int?)
        case listCreate(localID: String, name: String, description: String?, ordered: Bool)
        case listRename(list: ListRef, name: String)
        case listDelete(list: ListRef)
        case listAdd(list: ListRef, book: Int)
        case listRemove(list: ListRef, book: Int)
        case listMove(list: ListRef, book: Int, after: Int?)
        /// A place in a File, which may start, resume, or finish its Book as
        /// the server's position update does.
        case position(file: Int, seconds: Double?, page: Int?, percentage: Double, atEnd: Bool, listened: Double,
                      day: String, sinceDay: String?)
        /// Reading time in a File, which marks the Read-through underway as
        /// holding reading not yet sent.
        case session(file: Int)
    }

    /// The journal's order, which also names what an event makes, such as
    /// a Read-through started offline.
    let seq: Int
    let at: Double
    let change: Change
}

/// Names a List by the server's id or this installation's local id.
enum ListRef: Equatable, Hashable, Sendable {
    case id(Int)
    case local(String)
}

/// Lays changes not yet confirmed over the reading record, by the rules the
/// server applies them with: the web's status menu, Rating, To Read, and
/// Lists, and the position update's automatic starts and finishes
/// ("Starting and Finishing Automatically" in the docs, ADR 0028).
enum ReadingLayer {
    /// Seconds of listening in one sitting that start a Book.
    static let audioStartSeconds = 5.0
    /// How close to an audiobook's end counts as its end.
    static let audioEndSeconds = 60.0

    /// The id a Read-through or List made by the event seq gets until the
    /// feed brings the server's: negative, so it never meets one.
    static func localID(_ seq: Int) -> Int { -seq }

    /// The id a Read-through started by reading gets until the feed brings
    /// the server's. A place's events replace each other while reading, so
    /// it's the Book's rather than an event's, and stays put save to save.
    static func readingStartID(book: Int) -> Int { -1_000_000_000 - book }

    static func apply(_ events: [LayerEvent], to record: inout ReadingRecord, files: [Int: LayerFile]) {
        for event in events {
            apply(event, to: &record, files: files)
        }
    }

    static func apply(_ event: LayerEvent, to record: inout ReadingRecord, files: [Int: LayerFile]) {
        switch event.change {
        case .start(let book, let day):
            guard underway(book, in: record) == nil else { return }
            start(book, day: day, id: localID(event.seq), in: &record)
            record.userBooks[book, default: .init()].reference = false
        case .finish(let book, let day):
            end(book, outcome: "finished", day: day, seq: event.seq, in: &record)
        case .abandon(let book, let day):
            end(book, outcome: "abandoned", day: day, seq: event.seq, in: &record)
        case .pause(let book):
            setPaused(book, true, in: &record)
        case .resume(let book):
            setPaused(book, false, in: &record)
        case .deleteReadThrough(let id, let book):
            if let id {
                record.readThroughs[id] = nil
            } else if let book, let rt = underway(book, in: record) {
                record.readThroughs[rt.id] = nil
            }
        case .referenceMark(let book):
            if let rt = underway(book, in: record) {
                record.readThroughs[rt.id] = nil
            }
            record.toRead.removeAll { $0 == book }
            record.userBooks[book, default: .init()].reference = true
        case .referenceRemove(let book):
            record.userBooks[book, default: .init()].reference = false
        case .rating(let book, let units):
            record.userBooks[book, default: .init()].rating = units
        case .toReadAdd(let book):
            guard underway(book, in: record) == nil, !record.toRead.contains(book) else { return }
            record.toRead.append(book)
            record.userBooks[book, default: .init()].reference = false
        case .toReadRemove(let book):
            record.toRead.removeAll { $0 == book }
        case .toReadMove(let book, let after):
            move(book, after: after, in: &record.toRead)
        case .listCreate(let localID, let name, let description, let ordered):
            guard !record.lists.values.contains(where: { $0.localID == localID }) else { return }
            let id = Self.localID(event.seq)
            record.lists[id] = .init(id: id, localID: localID, name: name, description: description, isOrdered: ordered,
                                     defaultSort: ordered ? "manual" : "added_at_desc", ownerID: 0, ownerName: "",
                                     permission: "owner", shares: "[]", books: [], pending: true)
        case .listRename(let ref, let name):
            guard let id = listID(ref, in: record) else { return }
            record.lists[id]?.name = name
            record.lists[id]?.pending = true
        case .listDelete(let ref):
            guard let id = listID(ref, in: record) else { return }
            record.lists[id] = nil
        case .listAdd(let ref, let book):
            guard let id = listID(ref, in: record), var list = record.lists[id],
                  !list.books.contains(where: { $0.bookID == book }) else { return }
            let last = list.books.compactMap(\.sortOrder).max() ?? 0
            list.books.append(.init(bookID: book, sortOrder: list.isOrdered ? last + 1 : nil, addedAt: event.at))
            if !list.isOrdered {
                // Unordered Lists show the newest first.
                list.books.sort { $0.addedAt > $1.addedAt }
            }
            list.pending = true
            record.lists[id] = list
        case .listRemove(let ref, let book):
            guard let id = listID(ref, in: record) else { return }
            record.lists[id]?.books.removeAll { $0.bookID == book }
            record.lists[id]?.pending = true
        case .listMove(let ref, let book, let after):
            guard let id = listID(ref, in: record), var list = record.lists[id], list.isOrdered else { return }
            var order = list.books.map(\.bookID)
            move(book, after: after, in: &order)
            let byBook = Dictionary(list.books.map { ($0.bookID, $0) }, uniquingKeysWith: { first, _ in first })
            list.books = order.enumerated().compactMap { index, id in
                byBook[id].map { ReadingRecord.ListBook(bookID: id, sortOrder: index + 1, addedAt: $0.addedAt) }
            }
            list.pending = true
            record.lists[id] = list
        case .position(let fileID, let seconds, let page, let percentage, let atEnd, let listened, let day, let sinceDay):
            guard let file = files[fileID], !file.supplement else { return }
            position(file: file, seconds: seconds, page: page, percentage: percentage, reportedEnd: atEnd, listened: listened,
                     day: day, sinceDay: sinceDay, seq: event.seq, in: &record)
        case .session(let fileID):
            guard let file = files[fileID], let rt = underway(file.bookID, in: record) else { return }
            record.readThroughs[rt.id]?.pending = true
        }
    }

    // MARK: - Read-throughs

    static func underway(_ book: Int, in record: ReadingRecord) -> ReadingRecord.ReadThrough? {
        record.readThroughs.values.first { $0.bookID == book && $0.outcome == "underway" }
    }

    private static func start(_ book: Int, day: String, id: Int, in record: inout ReadingRecord) {
        record.readThroughs[id] = .init(id: id, bookID: book, beginDay: day, outcome: "underway", pending: true)
        record.toRead.removeAll { $0 == book }
    }

    /// Ends the Read-through underway, or for a finish with nothing
    /// underway, records one with no begin day, as Mark as read does.
    private static func end(_ book: Int, outcome: String, day: String, seq: Int, in record: inout ReadingRecord) {
        if let rt = underway(book, in: record) {
            var ended = rt
            ended.outcome = outcome
            ended.endDay = max(day, rt.beginDay ?? day)
            ended.paused = false
            ended.pending = true
            ended.endedHere = true
            record.readThroughs[rt.id] = ended
        } else if outcome == "finished" {
            // Taken in, and the feed already brought it, while its page
            // still lays the change over: once is enough.
            if record.readThroughs.values.contains(where: { $0.id > 0 && $0.bookID == book && $0.outcome == outcome && $0.endDay == day }) {
                return
            }
            let id = localID(seq)
            record.readThroughs[id] = .init(id: id, bookID: book, endDay: day, outcome: outcome, pending: true, endedHere: true)
        } else {
            return
        }
        record.toRead.removeAll { $0 == book }
        record.userBooks[book, default: .init()].reference = false
    }

    private static func setPaused(_ book: Int, _ paused: Bool, in record: inout ReadingRecord) {
        guard let rt = underway(book, in: record), rt.paused != paused else { return }
        record.readThroughs[rt.id]?.paused = paused
        record.readThroughs[rt.id]?.pending = true
    }

    /// What a place in a File does to its Book, as the server's position
    /// update does: real progress starts a Book never read or on To Read,
    /// and resumes a paused one; reaching the end finishes it. A Reference
    /// Book never starts or finishes.
    private static func position(file: LayerFile, seconds: Double?, page: Int?, percentage: Double, reportedEnd: Bool,
                                 listened: Double, day: String, sinceDay: String?, seq: Int, in record: inout ReadingRecord) {
        let book = file.bookID
        if record.userBooks[book]?.reference == true { return }
        var current = underway(book, in: record)
        let atEnd = reportedEnd || reachesEnd(file, seconds: seconds, page: page)
        let realProgress: Bool
        if seconds != nil {
            realProgress = (current.map { !$0.paused } ?? false) || listened >= audioStartSeconds
        } else if let page {
            realProgress = page > 0
        } else {
            realProgress = percentage > 0
        }
        let progress = atEnd || realProgress
        guard progress else { return }
        if current == nil {
            let readBefore = record.readThroughs.values.contains { $0.bookID == book }
            guard !readBefore || record.toRead.contains(book) else { return }
            start(book, day: sinceDay ?? day, id: readingStartID(book: book), in: &record)
            current = underway(book, in: record)
        } else if let rt = current, rt.paused, !atEnd {
            record.readThroughs[rt.id]?.paused = false
            record.readThroughs[rt.id]?.pending = true
        }
        if atEnd, let rt = current {
            // A place from before the read underway began ends nothing.
            if let begin = rt.beginDay, day < begin { return }
            end(book, outcome: "finished", day: day, seq: seq, in: &record)
        }
    }

    /// Whether a place is at the end of the File by its own length: the last
    /// page, or an audiobook's last minute.
    static func reachesEnd(_ file: LayerFile, seconds: Double?, page: Int?) -> Bool {
        if let page, let count = file.pageCount, count > 0 {
            return page >= count - 1
        }
        if let seconds, let duration = file.duration, duration > audioEndSeconds {
            return seconds >= duration - audioEndSeconds
        }
        return false
    }

    // MARK: - Order

    /// Moves id to just after after, or to the top with nil. An id whose
    /// neighbor is gone stays where it is, as the server leaves it.
    static func move(_ id: Int, after: Int?, in order: inout [Int]) {
        guard let from = order.firstIndex(of: id) else { return }
        var rest = order
        rest.remove(at: from)
        var to = 0
        if let after {
            guard let index = rest.firstIndex(of: after) else { return }
            to = index + 1
        }
        rest.insert(id, at: to)
        order = rest
    }

    /// A drag of the Book at from to to in order, as the journal records
    /// it: the Book, and the one it now follows, or nil for the top. Nil
    /// when nothing moved.
    static func move(in order: [Int], from: Int, to: Int) -> (book: Int, after: Int?)? {
        guard order.indices.contains(from), from != to else { return nil }
        var order = order
        let id = order.remove(at: from)
        let target = min(max(0, to), order.count)
        order.insert(id, at: target)
        return (id, target == 0 ? nil : order[target - 1])
    }

    static func listID(_ ref: ListRef, in record: ReadingRecord) -> Int? {
        switch ref {
        case .id(let id):
            return record.lists[id] == nil ? nil : id
        case .local(let localID):
            return record.lists.values.first { $0.localID == localID }?.id
        }
    }
}
