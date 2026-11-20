import Foundation
import ShishoAPI

/// Brings an account's copy of the catalog up to date from the change feed
/// (ADR 0023): every page after the stored cursor, each applied in one
/// transaction.
final class SyncEngine: Sendable {
    private let client: APIClient
    private let store: CatalogStore

    init(client: APIClient, store: CatalogStore) {
        self.client = client
        self.store = store
    }

    /// Syncs until the feed has nothing more. A "start over" answer starts
    /// a fresh download from the start, keeping the current copy on screen
    /// until it's done.
    func run() async throws {
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
