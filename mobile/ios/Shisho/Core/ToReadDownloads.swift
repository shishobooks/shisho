import Foundation

/// Downloading the top of To Read: the first few Books on it download, so
/// what's next is on the device before it's needed. A Book downloads each
/// time it comes into the top: one whose Download the user removed isn't
/// downloaded again while it stays there, and nothing is removed when a
/// Book moves down or the count goes down.
enum ToReadDownloads {
    struct Plan: Equatable {
        /// The Books to download now, in their order on To Read.
        let download: [Int]
        /// The Books in the top now, downloaded since they came into it, to
        /// remember for next time.
        let handled: Set<Int>
    }

    /// What to download for To Read, in its order, with the top count Books
    /// wanted, given the Books handled before. A count of 0 is off.
    static func plan(toRead: [Int], count: Int, handled: Set<Int>) -> Plan {
        guard count > 0 else { return Plan(download: [], handled: []) }
        let top = toRead.prefix(count)
        // A Book that left the top is forgotten, so it downloads again when
        // it comes back.
        return Plan(download: top.filter { !handled.contains($0) }, handled: Set(top))
    }
}
