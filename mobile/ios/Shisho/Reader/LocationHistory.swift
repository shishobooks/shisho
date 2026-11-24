import Foundation

/// The places a reader can go back and forward to in an open book, by the
/// web reader's rules (app/libraries/locationHistory.ts, after KOReader's
/// "Go back to previous location"); a change there changes these too:
///
/// - A jump (a link, a footnote's note, the contents, the scrubber) records
///   where it left from.
/// - Turning pages records nothing, so going back after paging around where
///   a jump landed returns to where the jump left from.
/// - Jumps in one group, as the results of one search, record only the first.
/// - Going back keeps the place it left for going forward again; a new jump
///   forgets the way forward, as a browser does.
/// - Going back or forward skips the place the reader is already at.
///
/// Places are CFIs of what the screen shows. The history lasts while the
/// book is open.
struct LocationHistory: Equatable {
    private var backStack: [String] = []
    private var forwardStack: [String] = []
    /// The group of the last jump recorded, until another jump or a move.
    private var group: Int?

    var canGoBack: Bool { !backStack.isEmpty }
    var canGoForward: Bool { !forwardStack.isEmpty }

    mutating func recordJump(from origin: String, group: Int? = nil) {
        if let group, group == self.group { return }
        backStack.append(origin)
        forwardStack = []
        self.group = group
    }

    /// The place to go back to from current, keeping current for going
    /// forward again; nil for none.
    mutating func back(from current: String) -> String? {
        Self.move(from: &backStack, to: &forwardStack, current: current, group: &group)
    }

    mutating func forward(from current: String) -> String? {
        Self.move(from: &forwardStack, to: &backStack, current: current, group: &group)
    }

    private static func move(from: inout [String], to: inout [String], current: String, group: inout Int?) -> String? {
        var target = from.popLast()
        while let place = target, place == current { target = from.popLast() }
        guard let target else { return nil }
        to.append(current)
        group = nil
        return target
    }
}
