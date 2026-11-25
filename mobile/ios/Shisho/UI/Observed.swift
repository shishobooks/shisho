import GRDB
import SwiftUI

extension View {
    /// Keeps value up to date with fetch from the current account's catalog:
    /// it runs once, then again whenever a sync changes what it read, so
    /// screens never reload by hand.
    ///
    /// While the reader covers the screen, a change is held and shown once
    /// it closes: every turn saves the place, and redrawing the screens
    /// under the reader for each would stutter a scroll through the pages.
    func observing<Value: Sendable & Equatable, ID: Equatable>(
        _ store: CatalogStore?,
        id: ID,
        into value: Binding<Value?>,
        fetch: @escaping @Sendable (Database) throws -> Value
    ) -> some View {
        modifier(Observing(store: store, id: id, value: value, fetch: fetch))
    }
}

extension EnvironmentValues {
    /// The reader is open over this screen, so nothing on it can be seen.
    @Entry var coveredByReader = false
}

private struct Observing<Value: Sendable & Equatable, ID: Equatable>: ViewModifier {
    let store: CatalogStore?
    let id: ID
    @Binding var value: Value?
    let fetch: @Sendable (Database) throws -> Value
    @Environment(\.coveredByReader) private var covered
    @State private var held = Held<Value>()

    func body(content: Content) -> some View {
        content
            .task(id: id) {
                // A change held for the last id isn't this one's.
                held.value = nil
                guard let store else { return }
                do {
                    for try await fetched in ValueObservation.tracking(fetch).removeDuplicates().values(in: store.pool) {
                        // The first value always shows, so a screen never
                        // waits on the reader to have anything at all.
                        if held.covered, value != nil {
                            held.value = fetched
                        } else {
                            held.value = nil
                            value = fetched
                        }
                    }
                } catch {
                    // A read that fails leaves what was shown.
                }
            }
            .onChange(of: covered, initial: true) { _, covered in
                held.covered = covered
                guard !covered, let latest = held.value else { return }
                held.value = nil
                value = latest
            }
    }
}

/// What the loop reads as it goes: whether the reader covers the screen
/// now, and the change it held meanwhile.
@MainActor
private final class Held<Value> {
    var covered = false
    var value: Value?
}
