import Foundation
import os
import ShishoAPI

/// Syncs an account both ways: brings the copy of the catalog up to date
/// from the change feed (ADR 0023), every page after the stored cursor,
/// each applied in one transaction, and sends the journal of changes made
/// on the device (ADR 0024).
final class SyncEngine: Sendable {
    /// The most events one batch carries.
    static let batchSize = 200

    private let client: APIClient
    private let store: CatalogStore
    /// The capabilities the server listed last, which decide when events it
    /// didn't understand are sent again.
    private let capabilities: @Sendable () -> [String]
    /// This installation's Device, which tells its own places from others'.
    private let deviceID: String
    /// Years of Reading Stats a screen asked for that have no copy yet,
    /// fetched with the next sync's.
    private let wantedYears = OSAllocatedUnfairLock<Set<Int>>(initialState: [])

    init(client: APIClient, store: CatalogStore, deviceID: String = "", capabilities: @escaping @Sendable () -> [String] = { [] }) {
        self.client = client
        self.store = store
        self.deviceID = deviceID
        self.capabilities = capabilities
    }

    /// Syncs until the feed has nothing more, sends the journal, then syncs
    /// again for what the journal changed. The feed comes first so another
    /// device's place is seen, and kept as an offer, before this device's
    /// moves replace it on the server. A "start over" answer starts a fresh
    /// download from the start, keeping the current copy on screen until
    /// it's done.
    func run() async throws {
        // A feed that fails doesn't keep the journal from being sent.
        var feedError: Error?
        do {
            try await pullFeedAndConfirm()
        } catch let error as APIError where error == .noAccess || error == .signedOut || error == .serverReplaced {
            // The server refused the account itself; the journal would be
            // refused too, and a failure sending it mustn't hide why.
            throw error
        } catch {
            feedError = error
        }
        let pending = try await store.pool.read(CatalogStore.pendingCount)
        try await sendJournal()
        if let feedError {
            throw feedError
        }
        if pending > 0 {
            try await pullFeedAndConfirm()
        }
        // Stats that fail to come keep the copy, which still counts this
        // device's reading on top; the rest of the sync stands.
        do {
            try await fetchStats()
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            await Diagnostics.shared.log(.sync, "reading stats not fetched: \(Diagnostics.describe(error))")
        }
    }

    /// Asks the next sync to fetch Reading Stats for year too, which a
    /// screen shows but the device has no copy of. It's fetched in the sync
    /// rather than beside it, so it never runs while the journal is sent
    /// and counts a session the copy then counts again.
    func want(year: Int) {
        _ = wantedYears.withLock { $0.insert(year) }
    }

    /// Pulls the feed, then stops laying the changes the server took in
    /// before it began over the synced copy, which now holds them.
    private func pullFeedAndConfirm() async throws {
        let began = Date()
        // A pull another sync overtook leaves confirming to that one.
        guard try await pullFeed() else { return }
        try store.feedCaughtUp(since: began)
    }

    /// Fetches Reading Stats again for every range kept, and all time and
    /// this year always, so they count what the journal just sent (ADR
    /// 0024). A server that doesn't list them keeps the copy as it was.
    func fetchStats() async throws {
        guard capabilities().contains(Shishoapp.Capability.readingStats.rawValue) else { return }
        var ranges = Set(try store.statsRanges())
        ranges.insert(nil)
        ranges.insert(Calendar.current.component(.year, from: Date()))
        for year in wantedYears.withLock({ $0 }) { ranges.insert(year) }
        for year in ranges.sorted(by: { ($0 ?? 0) < ($1 ?? 0) }) {
            let began = Date()
            let stats = try await client.stats(year: year)
            try Task.checkCancellation()
            try store.saveStats(stats, year: year, fetchedAt: began)
            if let year { _ = wantedYears.withLock { $0.remove(year) } }
        }
        let calendarShown = try await refreshCalendar()
        try store.statsCaughtUp(calendarHolds: calendarShown)
    }

    /// Fetches every copy of Reading Stats' Calendar again once this device
    /// sent changes that can alter it (markCalendarStale), one request a
    /// copy, so each counts what the server took in before statsCaughtUp
    /// stops laying that over it. A copy the server refuses for good goes;
    /// one that can't come now stays as it was, and holds back what it
    /// doesn't count, until a later sync fetches it. Returns whether the
    /// server lists the Calendar: one that doesn't hides it, so its copies
    /// hold nothing back meanwhile, and the changes stay noted for when it
    /// does again.
    private func refreshCalendar() async throws -> Bool {
        guard capabilities().contains(Shishoapp.Capability.readingCalendar.rawValue) else { return false }
        let began = Date()
        var current = true
        for key in try store.calendarCopiesToRefresh() {
            let fetchedAt = Date()
            do {
                switch key {
                case .heatmap(let year):
                    let response = try await client.statsHeatmap(year: year)
                    try Task.checkCancellation()
                    try store.saveCalendar(response, key: key, fetchedAt: fetchedAt, viewed: false)
                case .month(let year, let month):
                    let response = try await client.statsCalendar(year: year, month: month)
                    try Task.checkCancellation()
                    try store.saveCalendar(response, key: key, fetchedAt: fetchedAt, viewed: false)
                case .day(let day):
                    let response = try await client.statsDay(day: day)
                    try Task.checkCancellation()
                    try store.saveCalendar(response, key: key, fetchedAt: fetchedAt, zone: response.timeZone.isEmpty ? nil : response.timeZone, viewed: false)
                }
            } catch {
                if error is CancellationError || Task.isCancelled { throw CancellationError() }
                if case APIError.server(let status, _) = error, (400..<500).contains(status), status != 429 {
                    await Diagnostics.shared.log(.sync, "reading calendar copy \(key.raw) dropped: \(Diagnostics.describe(error))")
                    try store.deleteCalendar(key)
                } else {
                    await Diagnostics.shared.log(.sync, "reading calendar copy \(key.raw) kept, not fetched again: \(Diagnostics.describe(error))")
                    current = false
                }
            }
        }
        if current { try store.calendarRefreshed(before: began) }
        return true
    }

    /// Sends the journal in batches until nothing sendable is left. Any
    /// failure keeps the whole batch, to send again.
    func sendJournal() async throws {
        // Each round takes at least one event off the sendable list, so this
        // ends; the cap only guards against a server that answers oddly.
        for _ in 0..<50 {
            let listed = capabilities().sorted().joined(separator: ",")
            let rows = try store.pendingJournal(capabilities: listed, limit: SyncEngine.batchSize)
            guard !rows.isEmpty else { return }
            // Noted first, so a sync that stops once the server has them
            // still leaves the calendar's copies to fetch again.
            try store.markCalendarStale(rows)
            let response: Shishoapp.JournalResponse
            do {
                response = try await client.journal(rows)
            } catch {
                await Diagnostics.shared.log(.sync, "journal batch of \(rows.count) not sent: \(Diagnostics.describe(error))")
                throw error
            }
            // The answer's own capabilities are the ones it was judged by.
            let judgedBy = capabilities().sorted().joined(separator: ",")
            try store.applyJournalResults(response.results, sent: rows, capabilities: judgedBy)
            await Diagnostics.shared.logJournal(response, sent: rows)
        }
    }

    /// Pulls the feed to its end, reporting false when another sync moved
    /// the copy on first and this one stopped.
    @discardableResult
    private func pullFeed() async throws -> Bool {
        try store.startOverIfNeeded()
        var restarts = 0
        while true {
            let state = try store.syncState()
            let fetched = try await client.changes(cursor: state.cursor, expiredThrough: state.expiredThrough)
            let page = fetched.response
            if page.startOver {
                restarts += 1
                // A server that keeps answering "start over" can't be synced
                // with; say so rather than look up to date.
                guard restarts <= 2 else {
                    throw APIError.server(status: 0, message: String(localized: "The server kept asking to download the library again. Try syncing later."))
                }
                try store.startOverIfNeeded(serverSaidSo: true, at: state.cursor)
                continue
            }
            // Another sync moved the copy on while this one waited, so this
            // one's answer is stale.
            try Task.checkCancellation()
            guard try store.apply(page, after: state.cursor, ownDeviceID: deviceID, access: fetched.access) else {
                return false
            }
            // Every page's access counts: a Library lost and gained again
            // between two pages had its records skipped meanwhile.
            if let access = fetched.access {
                try store.reconcileAccess(access, fromFeed: true)
            }
            if !page.hasMore {
                if fetched.access != nil {
                    try await fetchGained()
                }
                return true
            }
        }
    }

    /// Fetches each Library and List the user gained whole, since their
    /// records are behind the cursor (ADR 0023). A fetch cut short goes on
    /// from where it got to next time.
    ///
    /// Each answer says what the user can access too, so a change seen
    /// meanwhile, such as a Library lost and gained again while a List was
    /// fetched, queues what it affects for another round.
    private func fetchGained() async throws {
        for _ in 0..<3 {
            let pending = try store.pendingAccess()
            guard !pending.libraries.isEmpty || !pending.lists.isEmpty else { return }
            for library in pending.libraries {
                var cursor = library.cursor
                while true {
                    let fetched: APIClient.FeedPage
                    do {
                        fetched = try await client.scopedChanges(libraryID: library.id, cursor: cursor)
                    } catch APIError.server(let status, _) where status == 404 {
                        // Lost again since; the next page's access drops it.
                        break
                    }
                    // A sync stopped meanwhile, as by switching accounts,
                    // writes nothing.
                    try Task.checkCancellation()
                    try store.applyScoped(fetched.response, libraryID: library.id, ownDeviceID: deviceID)
                    if let access = fetched.access {
                        try store.reconcileAccess(access)
                    }
                    await Diagnostics.shared.log(.sync, "fetched \(fetched.response.entries.count) records of a Library gained")
                    guard fetched.response.hasMore else { break }
                    cursor = fetched.response.cursor
                }
            }
            for listID in pending.lists {
                let fetched: APIClient.FeedPage
                do {
                    fetched = try await client.scopedChanges(listID: listID, cursor: 0)
                } catch APIError.server(let status, _) where status == 404 {
                    continue
                }
                try Task.checkCancellation()
                try store.applyScoped(fetched.response, listID: listID, ownDeviceID: deviceID)
                if let access = fetched.access {
                    try store.reconcileAccess(access)
                }
            }
        }
    }
}
