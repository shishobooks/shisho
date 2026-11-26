import ShishoAPI
import XCTest
@testable import Shisho

/// What CarPlay lists (#917), read from the account's database so it works
/// with no phone screen and no connection: its tabs Recent (the audiobooks
/// partway through), Downloads, and Library.
final class CarPlayCatalogTests: XCTestCase {
    private var store: CatalogStore!
    /// Every audiobook in the fixtures is this long.
    private let duration = 1000.0

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page(#"""
            {"entries":[\#(library(1, "Fiction")),\#(library(2, "Comics")),
             \#(book(1, "Dune", files: [(10, "m4b", "main"), (11, "epub", "main")])),
             \#(book(2, "Hyperion", files: [(20, "m4b", "main"), (21, "m4b", "main")], cover: "f21")),
             \#(book(3, "Foundation", files: [(30, "m4b", "main")])),
             \#(book(4, "Neuromancer", files: [(40, "epub", "main"), (41, "m4b", "supplement")])),
             \#(book(5, "Saga", library: 2, files: [(50, "cbz", "main")]))],"cursor":5}
            """#))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func library(_ id: Int, _ name: String) -> String {
        #"{"kind":"library","id":\#(id),"library_id":\#(id),"library":{"id":\#(id),"name":"\#(name)","cover_aspect_ratio":"book"}}"#
    }

    private func book(_ id: Int, _ title: String, library: Int = 1, files: [(Int, String, String)], cover: String = "") -> String {
        let fileJSON: String = files.map { (file: (Int, String, String)) -> String in
            let version = file.0 == 21 ? cover : ""
            let length = file.1 == "m4b" ? #","audiobook_duration_seconds":\#(duration)"# : ""
            return #"{"id":\#(file.0),"file_type":"\#(file.1)","file_role":"\#(file.2)","size_bytes":100,"display_name":"File \#(file.0)","cover_version":"\#(version)"\#(length)}"#
        }.joined(separator: ",")
        return """
        {"kind":"book","id":\(id),"library_id":\(library),"book":{"id":\(id),"library_id":\(library),"title":"\(title)","sort_title":"\(title)",
         "created_at":"2026-01-0\(id)T00:00:00Z","authors":[{"person_id":1,"name":"Author \(id)","sort_name":"Author \(id)"}],
         "files":[\(fileJSON)],"cover_version":"\(id)-1"}}
        """
    }

    /// Another device's place, from the feed.
    private func serverPlace(fileID: Int, percentage: Double, at: String) throws {
        try store.apply(page(#"""
            {"entries":[{"kind":"reading_position","id":\#(fileID),"reading_position":{"file_id":\#(fileID),"seconds":\#(percentage * duration),"percentage":\#(percentage),"occurred_at":"\#(at)","source":"web_reader"}}],"cursor":6}
            """#))
    }

    private func readThrough(_ id: Int, book: Int, outcome: String, paused: Bool = false) throws {
        try store.apply(page(#"""
            {"entries":[{"kind":"read_through","id":\#(id),"read_through":{"id":\#(id),"book_id":\#(book),"outcome":"\#(outcome)","paused":\#(paused)}}],"cursor":7}
            """#))
    }

    /// This device's place, waiting to be sent.
    private func listen(fileID: Int, percentage: Double, at: Date) throws {
        try store.recordPosition(fileID: fileID, seconds: percentage * duration, percentage: percentage, at: at,
                                 atEnd: percentage >= 1, sitting: nil)
    }

    private func continueListening(limit: Int = 50) throws -> [CarPlayAudiobook] {
        try store.pool.read { try CatalogStore.continueListening($0, limit: limit, ownDeviceID: "phone") }
    }

    func testContinueListeningIsTheAudiobooksStartedAndNotFinishedNewestFirst() throws {
        XCTAssertEqual(try continueListening(), [], "nothing listened to yet")

        try serverPlace(fileID: 30, percentage: 0.5, at: "2026-01-01T00:00:00Z")
        try listen(fileID: 10, percentage: 0.25, at: Date(timeIntervalSince1970: 1_800_000_000))
        // An ebook's place and a supplement's aren't listening.
        try listen(fileID: 11, percentage: 0.5, at: Date(timeIntervalSince1970: 1_800_000_100))
        try listen(fileID: 41, percentage: 0.5, at: Date(timeIntervalSince1970: 1_800_000_100))

        let listed = try continueListening()
        XCTAssertEqual(listed.map(\.fileID), [10, 30], "this device's newer place first")
        XCTAssertEqual(listed.first, CarPlayAudiobook(fileID: 10, bookID: 1, title: "Dune", authors: "Author 1", label: nil,
                                                      coverVersion: "", progress: 0.25),
                       "no cover of its own: never its Book's, which can be an ebook's")
        XCTAssertEqual(listed.last?.progress, 0.5, "the server's place, with none on this device")
        XCTAssertEqual(try continueListening(limit: 1).map(\.fileID), [10])
        XCTAssertEqual(try continueListening(limit: 0), [])

        try listen(fileID: 10, percentage: 0.999, at: Date(timeIntervalSince1970: 1_800_000_200))
        XCTAssertEqual(try continueListening().map(\.fileID), [10, 30], "a second from the end plays on")
        try listen(fileID: 10, percentage: 0.9995, at: Date(timeIntervalSince1970: 1_800_000_300))
        XCTAssertEqual(try continueListening().map(\.fileID), [30], "where playing starts over is the end")
    }

    /// A Book with several audiobooks lists each, told apart by its label,
    /// with its own cover.
    func testContinueListeningListsEachOfABooksAudiobooks() throws {
        try listen(fileID: 20, percentage: 0.1, at: Date(timeIntervalSince1970: 1_800_000_000))
        try listen(fileID: 21, percentage: 0.4, at: Date(timeIntervalSince1970: 1_800_000_100))
        let listed = try continueListening()
        XCTAssertEqual(listed.map(\.fileID), [21, 20])
        XCTAssertEqual(listed.map(\.label), ["File 21", "File 20"])
        XCTAssertEqual(listed.map(\.coverVersion), ["f21", ""])
    }

    /// Each audiobook is where the player would open it: this device's own
    /// place while it waits to be sent, even with a newer one from another
    /// device.
    func testContinueListeningIsWhereThePlayerWouldOpen() throws {
        try listen(fileID: 30, percentage: 0.2, at: Date(timeIntervalSince1970: 1_700_000_000))
        try serverPlace(fileID: 30, percentage: 1, at: "2026-06-01T00:00:00Z")
        XCTAssertEqual(try continueListening().map(\.progress), [0.2], "this device's place, not yet sent")

        // Sent: the other device's place, at the end, is newer.
        try store.pool.write { try $0.execute(sql: "DELETE FROM journal") }
        XCTAssertEqual(try continueListening(), [])
    }

    func testContinueListeningLeavesOutBooksNotBeingListenedTo() throws {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        try listen(fileID: 10, percentage: 0.3, at: now)
        try listen(fileID: 20, percentage: 0.3, at: now)
        try listen(fileID: 30, percentage: 0.3, at: now)
        try readThrough(1, book: 1, outcome: "finished")
        try readThrough(3, book: 3, outcome: "underway", paused: true)
        try readThrough(2, book: 2, outcome: "underway")
        XCTAssertEqual(try continueListening().map(\.bookID), [2], "not a Book read, nor one paused")
        try readThrough(2, book: 2, outcome: "abandoned")
        XCTAssertEqual(try continueListening(), [], "nor one given up on")
    }

    func testDownloadsAreTheAudiobooksOnThePhoneNewestFirst() throws {
        XCTAssertEqual(try store.queueDownloads([10, 11], at: Date(timeIntervalSince1970: 1_800_000_000)), [10, 11])
        XCTAssertEqual(try store.queueDownloads([30], at: Date(timeIntervalSince1970: 1_800_000_100)), [30])
        XCTAssertEqual(try store.pool.read(CatalogStore.downloadedAudiobooks), [], "none has arrived yet")

        for fileID in [10, 11, 30] {
            XCTAssertNotNil(try store.markDownloaded(fileID: fileID, sizeBytes: 100))
        }
        try listen(fileID: 30, percentage: 0.3, at: Date())
        var listed = try store.pool.read(CatalogStore.downloadedAudiobooks)
        XCTAssertEqual(listed.map(\.fileID), [30, 10], "only audiobooks, the newest Download first")
        XCTAssertEqual(listed.first?.progress, 0.3)
        XCTAssertNil(listed.last?.progress, "never started")

        // A downloaded supplement is audio too, named so it isn't taken for
        // its Book's audiobook.
        XCTAssertEqual(try store.queueDownloads([41], at: Date(timeIntervalSince1970: 1_800_000_200)), [41])
        XCTAssertNotNil(try store.markDownloaded(fileID: 41, sizeBytes: 100))
        listed = try store.pool.read(CatalogStore.downloadedAudiobooks)
        XCTAssertEqual(listed.map(\.fileID), [41, 30, 10])
        XCTAssertEqual(listed.first?.label, "File 41")

        // One being replaced plays the copy it has meanwhile.
        try store.pool.write { try $0.execute(sql: "UPDATE downloads SET state = 'queued', replacing = 1 WHERE file_id = 30") }
        listed = try store.pool.read(CatalogStore.downloadedAudiobooks)
        XCTAssertEqual(listed.map(\.fileID), [41, 30, 10])
    }

    /// The Library tab lists the Libraries with audiobooks, or the only one's
    /// audiobooks.
    func testTheLibraryTabListsLibrariesOrTheOnlyOnesAudiobooks() throws {
        XCTAssertEqual(try store.pool.read(CatalogStore.audiobookLibraries), [LibrarySummary(id: 1, name: "Fiction")])
        XCTAssertEqual(try store.pool.read { try CatalogStore.carPlayLibraryTab($0) { _ in nil } }.compactMap {
            if case .audiobook(let book) = $0 { book.fileID } else { nil }
        }, [30, 20, 21, 10], "the only Library's audiobooks, in its order")
        XCTAssertEqual(try store.pool.read { try CatalogStore.carPlayLibraryTab($0) { _ in "title:asc" } }.compactMap {
            if case .audiobook(let book) = $0 { book.fileID } else { nil }
        }, [10, 30, 20, 21], "in the order picked on the phone")

        try store.apply(page(#"{"entries":[\#(library(3, "Audio")),\#(book(6, "Anathem", library: 3, files: [(60, "m4b", "main")]))],"cursor":8}"#))
        XCTAssertEqual(try store.pool.read { try CatalogStore.carPlayLibraryTab($0) { _ in nil } },
                       [.library(LibrarySummary(id: 3, name: "Audio")), .library(LibrarySummary(id: 1, name: "Fiction"))])
    }

    /// A Library lists every audiobook of its Books, in its order, and a
    /// Book without one not at all.
    func testALibraryListsEachAudiobookInItsOrder() throws {
        try listen(fileID: 21, percentage: 0.4, at: Date(timeIntervalSince1970: 1_800_000_100))
        let listed = try store.pool.read { try CatalogStore.audiobooks($0, libraryID: 1) }
        // Newest first, as the Library is sorted with no saved sort; a
        // supplement alone doesn't make a Book an audiobook.
        XCTAssertEqual(listed.map(\.title), ["Foundation", "Hyperion", "Hyperion", "Dune"])
        XCTAssertEqual(listed.map(\.fileID), [30, 20, 21, 10])
        XCTAssertEqual(listed.map(\.label), [nil, "File 20", "File 21", nil])
        XCTAssertEqual(listed[2].progress, 0.4)
        XCTAssertNil(listed[1].progress)
        XCTAssertEqual(try store.pool.read { try CatalogStore.audiobookFileID($0, bookID: 2) }, 21, "Listen plays the one listened to last")

        XCTAssertEqual(try store.pool.read { try CatalogStore.audiobooks($0, libraryID: 1, sort: "title:asc") }.map(\.title),
                       ["Dune", "Foundation", "Hyperion", "Hyperion"], "the order picked on the phone")
        XCTAssertEqual(try store.pool.read { try CatalogStore.audiobooks($0, libraryID: 2) }, [])
    }
}
