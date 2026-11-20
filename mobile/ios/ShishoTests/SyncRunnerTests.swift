import XCTest
@testable import Shisho

/// Holds each caller until opened, so a test can act while a sync runs.
@MainActor
private final class Gate {
    private var waiters: [CheckedContinuation<Void, Never>] = []
    private(set) var arrivals = 0

    func wait() async {
        arrivals += 1
        await withCheckedContinuation { waiters.append($0) }
    }

    func open() {
        let waiting = waiters
        waiters = []
        waiting.forEach { $0.resume() }
    }
}

@MainActor
private func settle(until condition: () -> Bool) async {
    for _ in 0..<1000 where !condition() {
        await Task.yield()
    }
}

@MainActor
final class SyncRunnerTests: XCTestCase {
    /// A sync asked for while one runs isn't dropped: the runner syncs once
    /// more after it, and both callers wait for that.
    func testARequestDuringASyncRunsOnceMoreAfterIt() async {
        let runner = SyncRunner()
        let gate = Gate()
        var runs = 0
        let first = Task { await runner.run { _ in runs += 1; await gate.wait() } }
        await settle { gate.arrivals == 1 }
        var secondDone = false
        let second = Task { await runner.run { _ in XCTFail("joins the sync in flight") }; secondDone = true }
        await settle { false }

        gate.open()
        await settle { gate.arrivals == 2 }
        XCTAssertEqual(runs, 2)
        XCTAssertFalse(secondDone, "waits for the sync after the one in flight")
        gate.open()
        await first.value
        await second.value
        XCTAssertFalse(runner.isRunning)
    }

    /// Stopping (signing out, losing the network) drops the sync in flight:
    /// its result is stale, and the next sync starts at once, without
    /// waiting for it to unwind.
    func testStoppingDropsTheSyncInFlight() async {
        let runner = SyncRunner()
        let gate = Gate()
        var stale: Bool?
        let first = Task { await runner.run { isCurrent in await gate.wait(); stale = !isCurrent() } }
        await settle { gate.arrivals == 1 }

        runner.stop()
        XCTAssertFalse(runner.isRunning)
        var ran = false
        await runner.run { isCurrent in ran = isCurrent() }
        XCTAssertTrue(ran)

        gate.open()
        await first.value
        XCTAssertEqual(stale, true)
        XCTAssertFalse(runner.isRunning)
    }
}
