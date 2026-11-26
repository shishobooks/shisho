import Foundation
import GRDB

/// A Book's Downloads taken together, for its page's Download button, its
/// cell's mark, and its press-and-hold menu. A whole Book downloads every
/// File of a type this version knows, its supplements too, since they're
/// usually small and go with it, as a Series' Download All does.
struct BookDownloads: Equatable, Sendable {
    struct File: Equatable, Sendable {
        let fileID: Int
        /// The File's size in the catalog, or the bytes on the device once
        /// it's downloaded.
        let sizeBytes: Int
        /// Nil without a Download.
        let state: DownloadState?
        let supplement: Bool
        /// Queued again because the server replaced it, with the old copy
        /// still on the device.
        var replacing = false
    }

    /// Where the Book's Downloads stand, as its Download button shows it.
    enum Phase: Equatable, Sendable {
        /// Nothing it could download.
        case unavailable
        /// Nothing downloaded or on its way: Download, with how many Files
        /// and how many bytes.
        case notDownloaded(files: Int, bytes: Int)
        /// Something on its way, with how far all of it has got from 0 to
        /// 1, or nil before anything is known.
        case downloading(Double?)
        /// Some downloaded and the rest not: Download the rest.
        case partlyDownloaded(files: Int, bytes: Int)
        /// Everything downloaded, with how many Files and bytes.
        case downloaded(files: Int, bytes: Int)
    }

    let bookID: Int
    var files: [File]

    static func empty(_ bookID: Int) -> BookDownloads { BookDownloads(bookID: bookID, files: []) }

    /// Something is on its way.
    var isDownloading: Bool { files.contains { $0.state == .queued } }

    /// A File is on the device, as `downloadedBookIDs` counts one: done, or
    /// queued again with its old copy still here.
    var hasDownloaded: Bool { files.contains { $0.state == .done || $0.replacing } }

    /// Files with no Download, or one that failed, supplements included:
    /// what Download starts.
    var missing: [File] { files.filter { $0.state == nil || $0.state == .failed } }

    /// What Cancel Download stops: everything on its way except a replaced
    /// File's new copy, whose old one stays usable until it arrives.
    var cancellable: [Int] { files.filter { $0.state == .queued && !$0.replacing }.map(\.fileID) }

    /// Every File with a Download of any kind, for removing them all.
    var removable: [Int] { files.filter { $0.state != nil }.map(\.fileID) }

    func phase(progress: [Int: Double]) -> Phase {
        if isDownloading { return .downloading(fraction(progress: progress)) }
        let missing = missing
        if missing.isEmpty {
            let onDevice = files.filter { $0.state == .done }
            return onDevice.isEmpty ? .unavailable : .downloaded(files: onDevice.count, bytes: onDevice.reduce(0) { $0 + $1.sizeBytes })
        }
        let bytes = missing.reduce(0) { $0 + $1.sizeBytes }
        return hasDownloaded ? .partlyDownloaded(files: missing.count, bytes: bytes) : .notDownloaded(files: missing.count, bytes: bytes)
    }

    /// How far the Book's Downloads have got, from 0 to 1, by bytes: the
    /// Files downloaded count whole, and each on its way by how far it has
    /// got, so a Book of several Files fills one ring as they arrive. Nil
    /// while nothing is known yet, as before the first transfer reports.
    func fraction(progress: [Int: Double]) -> Double? {
        let counted = files.filter { $0.state == .queued || $0.state == .done }
        guard !counted.isEmpty else { return nil }
        // Sizes can be missing from the catalog, so each File counts the
        // same when none is known.
        let bySize = counted.contains { $0.sizeBytes > 0 }
        var known = false
        var got = 0.0
        var total = 0.0
        for file in counted {
            let weight = bySize ? Double(max(0, file.sizeBytes)) : 1
            total += weight
            if file.state == .done {
                got += weight
                known = true
            } else if let fraction = progress[file.fileID] {
                got += weight * min(1, max(0, fraction))
                known = true
            }
        }
        guard known, total > 0 else { return nil }
        return min(1, got / total)
    }

    /// What a Book's cell says to VoiceOver about its Downloads, or nil
    /// with none.
    func words(progress: [Int: Double]) -> String? {
        if isDownloading {
            guard let fraction = fraction(progress: progress) else { return String(localized: "Downloading") }
            return String(localized: "Downloading, \(fraction.formatted(.percent.precision(.fractionLength(0))))")
        }
        return hasDownloaded ? String(localized: "Downloaded") : nil
    }
}

extension CatalogStore {
    /// The Files a Book's Downloads are about: its main Files a Download can
    /// be made of, and any other File of it with a Download.
    private static let bookDownloadsSQL = """
        SELECT f.book_id, f.id, COALESCE(d.size_bytes, f.size_bytes) AS size_bytes, f.file_role, d.state, d.replacing
        FROM book_files f LEFT JOIN downloads d ON d.file_id = f.id
        """

    private static var bookDownloadsFilter: String {
        "(d.file_id IS NOT NULL OR f.file_type IN (\(downloadableTypes)))"
    }

    static func bookDownloads(_ db: Database, bookID: Int) throws -> BookDownloads {
        let rows = try Row.fetchAll(db, sql: """
            \(bookDownloadsSQL) WHERE f.book_id = ? AND \(bookDownloadsFilter) ORDER BY f.position
            """, arguments: [bookID])
        return BookDownloads(bookID: bookID, files: rows.map(bookDownloadFile))
    }

    /// Every Book with a Download of any of its Files, by Book, for a grid's
    /// marks and menus: one read for the whole grid, rather than one per
    /// cell. A Book missing from it has nothing downloaded or on its way.
    static func booksDownloads(_ db: Database) throws -> [Int: BookDownloads] {
        let rows = try Row.fetchAll(db, sql: """
            \(bookDownloadsSQL)
            WHERE f.book_id IN (SELECT b.book_id FROM downloads x JOIN book_files b ON b.id = x.file_id) AND \(bookDownloadsFilter)
            ORDER BY f.book_id, f.position
            """)
        var books: [Int: BookDownloads] = [:]
        for row in rows {
            let bookID: Int = row["book_id"]
            books[bookID, default: .empty(bookID)].files.append(bookDownloadFile(row))
        }
        return books
    }

    private static func bookDownloadFile(_ row: Row) -> BookDownloads.File {
        BookDownloads.File(fileID: row["id"], sizeBytes: row["size_bytes"],
                           state: (row["state"] as String?).map { DownloadState(rawValue: $0) ?? .failed },
                           supplement: (row["file_role"] as String) == "supplement",
                           replacing: (row["replacing"] as Bool?) ?? false)
    }
}
