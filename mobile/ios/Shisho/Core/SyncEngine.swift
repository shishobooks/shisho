import Foundation
import ShishoAPI

/// Syncs an account both ways: sends the journal of changes made on the
/// device (ADR 0024), then brings the copy of the catalog up to date from
/// the change feed (ADR 0023), every page after the stored cursor, each
/// applied in one transaction.
final class SyncEngine: Sendable {
    /// The most events one batch carries.
    static let batchSize = 200

    private let client: APIClient
    private let store: CatalogStore
    /// The capabilities the server listed last, which decide when events it
    /// didn't understand are sent again.
    private let capabilities: @Sendable () -> [String]

    init(client: APIClient, store: CatalogStore, capabilities: @escaping @Sendable () -> [String] = { [] }) {
        self.client = client
        self.store = store
        self.capabilities = capabilities
    }

    /// Sends the journal, then syncs until the feed has nothing more, so the
    /// feed brings back what the journal changed. A "start over" answer
    /// starts a fresh download from the start, keeping the current copy on
    /// screen until it's done.
    func run() async throws {
        try await sendJournal()
        try await pullFeed()
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

    private func pullFeed() async throws {
        try store.startOverIfNeeded()
        var restarts = 0
        while true {
            let state = try store.syncState()
            let page = try await client.changes(cursor: state.cursor, expiredThrough: state.expiredThrough)
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
            guard try store.apply(page, after: state.cursor) else {
                return
            }
            if !page.hasMore {
                return
            }
        }
    }
}
