import GRDB
import XCTest
@testable import Shisho

/// What reading an ebook leaves in the journal, and when it asks for a
/// sync (#919).
@MainActor
final class ReadingTrackerTests: XCTestCase {
    private var store: CatalogStore!
    private var clock = Date(timeIntervalSince1970: 1_800_000_000)
    private var syncs = 0

    override func setUp() async throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
    }

    private func tracker() -> ReadingTracker {
        ReadingTracker(fileID: 8, store: store, now: { [unowned self] in self.clock }) { [unowned self] in self.syncs += 1 }
    }

    private func page(_ n: Int, percentage: Double = 0.1, atEnd: Bool = false) -> PageLocation {
        PageLocation(cfi: "epubcfi(/6/\(n * 2)!/4/2)", fraction: percentage, index: n, page: 1, pages: 3, atEnd: atEnd, screen: "\(n):1")
    }

    private func journal() throws -> [String] {
        try store.pool.read { try String.fetchAll($0, sql: "SELECT type FROM journal ORDER BY seq") }
    }

    private func wait(_ seconds: TimeInterval) {
        clock = clock.addingTimeInterval(seconds)
    }

    /// Pages turned too quickly to count still move the place, which goes
    /// out when the reader closes.
    func testClosingAfterASkimSendsThePlace() throws {
        let tracker = tracker()
        tracker.opened(page(1))
        for n in 2...4 {
            wait(1)
            tracker.moved(page(n, percentage: Double(n) / 10))
        }
        XCTAssertEqual(syncs, 0)
        tracker.close()
        XCTAssertEqual(syncs, 1)
        XCTAssertEqual(try journal(), ["reading_position"])
    }

    /// Where the reader opened isn't a move, and the same place twice is
    /// saved once.
    func testOnlyMovesAreSaved() throws {
        let tracker = tracker()
        tracker.opened(page(1))
        tracker.moved(page(1))
        XCTAssertEqual(try journal(), [])
        wait(6)
        tracker.moved(page(2))
        tracker.moved(page(2))
        XCTAssertEqual(try journal(), ["reading_position", "reading_session"])
    }

    /// Going to the background records the session there; closing while
    /// away doesn't record it again.
    func testBackgroundThenCloseRecordsTheSessionOnce() throws {
        let tracker = tracker()
        tracker.opened(page(1))
        wait(20)
        tracker.background()
        XCTAssertEqual(try journal(), ["reading_session"])
        XCTAssertEqual(syncs, 1)
        wait(10)
        tracker.close()
        XCTAssertEqual(try journal(), ["reading_session"])
    }

    /// Reaching the end is sent at once, and asked for again on closing
    /// while it still waits, as after a sync that failed.
    func testReachingTheEndSyncs() throws {
        let tracker = tracker()
        tracker.opened(page(1))
        wait(6)
        tracker.moved(page(9, percentage: 1, atEnd: true))
        XCTAssertEqual(syncs, 1)
        tracker.close()
        XCTAssertEqual(syncs, 3)
    }

    /// Opening again, as Try Again does after a page failed, records the
    /// session before it instead of dropping it.
    func testOpeningAgainRecordsTheSessionBefore() throws {
        let tracker = tracker()
        tracker.opened(page(1))
        wait(20)
        tracker.opened(page(1))
        XCTAssertEqual(try journal(), ["reading_session"])
    }

    /// A book that finishes opening while the app is away counts nothing
    /// until it's back.
    func testOpeningWhileAwayCountsFromTheReturn() throws {
        let tracker = tracker()
        tracker.background()
        tracker.opened(page(1))
        wait(25)
        tracker.foreground()
        wait(8)
        tracker.moved(page(2))
        let duration = try store.pool.read { db in
            try Data.fetchOne(db, sql: "SELECT payload FROM journal WHERE type = 'reading_session'")
        }.flatMap { try JSONSerialization.jsonObject(with: $0) as? [String: Any] }?["duration"] as? Int
        XCTAssertEqual(duration, 8)
    }
}
