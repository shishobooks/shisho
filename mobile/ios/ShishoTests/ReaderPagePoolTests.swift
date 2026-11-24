import XCTest
@testable import Shisho

/// Reader pages booted while a Book's page shows are the ones the reader
/// opens on (#919).
@MainActor
final class ReaderPagePoolTests: XCTestCase {
    override func tearDown() async throws {
        ReaderPagePool.shared.drain()
    }

    func testTheReaderTakesThePagesBootedAhead() {
        let pool = ReaderPagePool.shared
        pool.prewarm()
        XCTAssertTrue(pool.isWarm)
        _ = pool.take()
        XCTAssertFalse(pool.isWarm)
    }

    /// A Book's page showing again doesn't boot another set.
    func testPrewarmingTwiceBootsOneSet() {
        let pool = ReaderPagePool.shared
        pool.prewarm()
        let first = pool.take()
        pool.prewarm()
        pool.prewarm()
        let second = pool.take()
        XCTAssertFalse(first.current === second.current)
        XCTAssertFalse(pool.isWarm)
    }

    /// The pool lets go on the main actor after the notification, which a
    /// busy CI simulator can hold up for seconds.
    private func waitUntilCold(_ pool: ReaderPagePool) async throws {
        let deadline = Date().addingTimeInterval(30)
        while pool.isWarm, Date() < deadline {
            try await Task.sleep(for: .milliseconds(50))
        }
        XCTAssertFalse(pool.isWarm)
    }

    func testMemoryPressureLetsThePagesGo() async throws {
        let pool = ReaderPagePool.shared
        pool.prewarm()
        NotificationCenter.default.post(name: UIApplication.didReceiveMemoryWarningNotification, object: nil)
        try await waitUntilCold(pool)
    }

    func testGoingToTheBackgroundLetsThePagesGo() async throws {
        let pool = ReaderPagePool.shared
        pool.prewarm()
        NotificationCenter.default.post(name: UIApplication.didEnterBackgroundNotification, object: nil)
        try await waitUntilCold(pool)
    }
}
