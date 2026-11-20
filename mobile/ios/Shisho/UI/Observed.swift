import GRDB
import SwiftUI

extension View {
    /// Keeps value up to date with fetch from the current account's catalog:
    /// it runs once, then again whenever a sync changes what it read, so
    /// screens never reload by hand.
    func observing<Value: Sendable & Equatable>(
        _ store: CatalogStore?,
        id: some Equatable,
        into value: Binding<Value?>,
        fetch: @escaping @Sendable (Database) throws -> Value
    ) -> some View {
        task(id: id) {
            guard let store else { return }
            do {
                for try await fetched in ValueObservation.tracking(fetch).removeDuplicates().values(in: store.pool) {
                    value.wrappedValue = fetched
                }
            } catch {
                // A read that fails leaves what was shown.
            }
        }
    }
}
