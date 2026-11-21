import Foundation
import ShishoAPI
import UIKit

/// A rolling log on the device of what the app can't show the user: sync
/// batches and their results, player errors, and the versions on both
/// sides. There's no crash reporting service; the user exports this and
/// reads it before sending it anywhere.
///
/// Lines name Books and Files by id, never by title, and the export removes
/// server addresses and credentials, so a log is safe to share. Titles go
/// into an export only when the user includes them.
actor Diagnostics {
    static let shared = Diagnostics()

    enum Category: String, Sendable {
        case app, sync, journal, player
    }

    /// The log is trimmed to its newer half once it passes this size.
    static let maxBytes = 512 * 1024

    private let url: URL

    init(folder: URL = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        .appendingPathComponent("Diagnostics", isDirectory: true)) {
        url = folder.appendingPathComponent("log.txt")
    }

    func log(_ category: Category, _ message: String, at date: Date = Date()) {
        let line = "\(date.formatted(.iso8601)) [\(category.rawValue)] \(message)\n"
        let fileManager = FileManager.default
        try? fileManager.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        if let handle = try? FileHandle(forWritingTo: url) {
            defer { try? handle.close() }
            _ = try? handle.seekToEnd()
            try? handle.write(contentsOf: Data(line.utf8))
        } else {
            try? Data(line.utf8).write(to: url, options: .atomic)
        }
        trimIfNeeded()
    }

    /// Notes what the server did with a journal batch: a count of each
    /// result, and each event it didn't take in, with why.
    func logJournal(_ response: Shishoapp.JournalResponse, sent: [JournalRow]) {
        var counts: [String: Int] = [:]
        for result in response.results {
            counts[result.result.rawValue, default: 0] += 1
        }
        let summary = counts.sorted { $0.key < $1.key }.map { "\($0.value) \($0.key)" }.joined(separator: ", ")
        log(.journal, "batch of \(sent.count): \(summary); clock offset \(Int(response.clockOffsetSeconds)) s")
        let types = Dictionary(sent.map { ($0.eventID, $0.type) }, uniquingKeysWith: { first, _ in first })
        for result in response.results where result.result != .accepted && result.result != .duplicate {
            log(.journal, "\(types[result.id] ?? "event") \(result.id) \(result.result.rawValue): \(result.reason)")
        }
    }

    /// The whole log as it is.
    func contents() -> String {
        (try? String(contentsOf: url, encoding: .utf8)) ?? ""
    }

    func clear() {
        try? FileManager.default.removeItem(at: url)
    }

    private func trimIfNeeded() {
        guard let size = (try? FileManager.default.attributesOfItem(atPath: url.path))?[.size] as? Int,
              size > Diagnostics.maxBytes,
              let data = try? Data(contentsOf: url)
        else { return }
        var kept = data.suffix(Diagnostics.maxBytes / 2)
        if let newline = kept.firstIndex(of: UInt8(ascii: "\n")) {
            kept = kept[kept.index(after: newline)...]
        }
        try? Data(kept).write(to: url, options: .atomic)
    }

    // MARK: - Export

    /// What the export says about one account, without its address or user.
    struct AccountSummary: Sendable {
        let serverVersion: String
        let capabilities: [String]
        let pendingEvents: Int
    }

    /// The export as text: versions, then the log, with server addresses
    /// and credentials removed. titles, when given, names the Books and
    /// Files the log mentions by id.
    func export(accounts: [AccountSummary], servers: [URL], titles: [String: String]?, device: String, system: String) -> String {
        var text = """
        Shisho app diagnostics
        Exported: \(Date().formatted(.iso8601))
        App: \(Installation.appVersion), protocol \(Installation.appProtocol)
        Device: \(device), \(system)

        """
        for (index, account) in accounts.enumerated() {
            let capabilities = account.capabilities.isEmpty ? "none" : account.capabilities.joined(separator: ", ")
            text += "Account \(index + 1): server \(account.serverVersion), capabilities: \(capabilities), changes waiting to sync: \(account.pendingEvents)\n"
        }
        text += "\nLog\n\n" + contents()
        if let titles, !titles.isEmpty {
            text += "\nTitles\n\n" + titles.sorted { $0.key < $1.key }.map { "\($0.key): \($0.value)" }.joined(separator: "\n") + "\n"
        }
        return Diagnostics.redact(text, servers: servers)
    }

    /// The Books and Files the log names by id, as "book 3" and "file 12".
    func mentionedIDs() -> (books: Set<Int>, files: Set<Int>) {
        let text = contents()
        func ids(_ word: String) -> Set<Int> {
            let pattern = try? NSRegularExpression(pattern: "\\b\(word) (\\d+)\\b")
            let range = NSRange(text.startIndex..., in: text)
            return Set((pattern?.matches(in: text, range: range) ?? []).compactMap { match in
                Range(match.range(at: 1), in: text).flatMap { Int(text[$0]) }
            })
        }
        return (ids("book"), ids("file"))
    }

    /// text with every server address, any other URL or IP address, and any
    /// credential replaced.
    static func redact(_ text: String, servers: [URL]) -> String {
        var text = text
        for server in servers {
            for known in [server.absoluteString, server.host()].compactMap({ $0 }) where !known.isEmpty {
                text = text.replacingOccurrences(of: known, with: "<server>")
            }
        }
        let patterns = [
            ("(?i)bearer\\s+[A-Za-z0-9._~+/=-]+", "Bearer <credential>"),
            ("[A-Za-z][A-Za-z0-9+.-]*://[^\\s\"'<>)]+", "<address>"),
            ("\\b\\d{1,3}(\\.\\d{1,3}){3}(:\\d+)?\\b", "<address>"),
            ("(?i)\\b[0-9a-f]{1,4}(:[0-9a-f]{0,4}){4,7}\\b", "<address>"),
            // Compressed IPv6, such as fe80::1; a clock time never has "::".
            ("(?i)(?<![\\w:])(?:[0-9a-f]{1,4}:){1,6}:(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4})*)?(?![\\w:])", "<address>"),
        ]
        for (pattern, replacement) in patterns {
            text = text.replacingOccurrences(of: pattern, with: replacement, options: .regularExpression)
        }
        return text
    }

    /// An error as the log names it.
    nonisolated static func describe(_ error: Error) -> String {
        if let error = error as? APIError {
            return String(describing: error)
        }
        let ns = error as NSError
        return "\(ns.domain) \(ns.code): \(ns.localizedDescription)"
    }
}
