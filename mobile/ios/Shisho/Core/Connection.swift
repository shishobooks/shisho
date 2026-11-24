import Foundation
import os

/// Whether the app can reach an account's server, as every screen shows it.
enum Connection: Equatable, Sendable {
    case online
    /// The device has no network at all, as in airplane mode.
    case offline
    /// There's a network, but the server didn't answer: a home server away
    /// from home, a server that's down, or a VPN that keeps the phone looking
    /// connected in airplane mode.
    case unreachable
}

/// What's known of whether one account's server can be reached, shared by
/// every request on it, from any thread. The device's network comes from
/// network changes; whether the server answers comes from the requests
/// themselves and from a quick check (`APIClient.checkServer`). While either
/// says no, requests fail at once rather than each waiting out a timeout.
final class ServerReachability: Sendable {
    private struct State: Sendable {
        var network = true
        var answers = true
    }

    private let state = OSAllocatedUnfairLock(initialState: State())
    private let onChange: OSAllocatedUnfairLock<(@Sendable (Connection) -> Void)?> = OSAllocatedUnfairLock(initialState: nil)

    var connection: Connection {
        state.withLock { ServerReachability.connection($0) }
    }

    /// Requests may go to the server.
    var allowsRequests: Bool { connection == .online }

    /// Called with the new connection whenever it changes, from any thread.
    func observe(_ handler: @escaping @Sendable (Connection) -> Void) {
        onChange.withLock { $0 = handler }
    }

    func networkChanged(available: Bool) {
        update { $0.network = available }
    }

    /// The server answered, with anything, even an error.
    func answered() {
        update { $0.answers = true }
    }

    /// A request got no answer: when it says the server can't be reached,
    /// later ones don't wait until a check finds it again.
    func failed(_ error: Error) {
        guard ConnectionRules.meansUnreachable(error) else { return }
        update { $0.answers = false }
    }

    private func update(_ change: @Sendable (inout State) -> Void) {
        let (before, after) = state.withLock { state -> (Connection, Connection) in
            let before = ServerReachability.connection(state)
            change(&state)
            return (before, ServerReachability.connection(state))
        }
        guard before != after else { return }
        onChange.withLock { $0 }?(after)
    }

    private static func connection(_ state: State) -> Connection {
        !state.network ? .offline : state.answers ? .online : .unreachable
    }
}

enum ConnectionRules {
    /// Whether a request's failure says the server can't be reached now,
    /// rather than, say, that the request was cancelled.
    static func meansUnreachable(_ error: Error) -> Bool {
        guard let error = error as? URLError else { return false }
        switch error.code {
        case .notConnectedToInternet, .networkConnectionLost, .timedOut, .cannotConnectToHost, .cannotFindHost,
             .dnsLookupFailed, .internationalRoamingOff, .dataNotAllowed, .secureConnectionFailed:
            return true
        default:
            return false
        }
    }

    /// Whether a File can be listened to or read now: the server can be
    /// reached, or it's on the device, or it's already playing.
    static func canOpen(connection: Connection, downloaded: Bool, playing: Bool) -> Bool {
        connection == .online || downloaded || playing
    }

    /// Whether the Library dims a Book: without the server, one with
    /// nothing downloaded, and every one until the downloaded are known.
    static func dimmed(bookID: Int, connection: Connection, downloaded: Set<Int>?) -> Bool {
        connection != .online && downloaded?.contains(bookID) != true
    }

    /// What the player says about an audiobook it can't stream.
    static func audiobook(_ connection: Connection) -> String {
        connection == .offline
            ? String(localized: "You're offline, and this audiobook isn't downloaded. Download it to listen without a connection.")
            : String(localized: "Your server can't be reached, and this audiobook isn't downloaded. Download it to listen without a connection.")
    }

    /// What the reader says about a book it can't fetch.
    static func book(_ connection: Connection) -> String {
        connection == .offline
            ? String(localized: "You're offline, and this book isn't downloaded. Download it to read without a connection.")
            : String(localized: "Your server can't be reached, and this book isn't downloaded. Download it to read without a connection.")
    }
}
