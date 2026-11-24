import Foundation

/// Ebooks opened without a Download (ADR 0022): each is fetched once into
/// the caches folder, which iOS may clear when space runs low, so it stays
/// readable for a while without counting as a Download.
final class EbookCache: Sendable {
    private let client: APIClient
    private let folder: URL

    init(client: APIClient, accountID: String) {
        self.client = client
        folder = EbookCache.folder(accountID: accountID)
    }

    /// The folder an account's ebooks are kept in.
    static func folder(accountID: String) -> URL {
        FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Ebooks", isDirectory: true)
            .appendingPathComponent(accountID, isDirectory: true)
    }

    /// Where the File's bytes are on the device, fetching them first when
    /// they aren't. A File whose size changed on the server is fetched
    /// again. Downloads (#915) will be used instead when there is one.
    func file(_ file: ReadingFile) async throws -> URL {
        let name = "\(file.fileID)-\(file.sizeBytes).\(file.fileType)"
        let kept = folder.appendingPathComponent(name)
        let fileManager = FileManager.default
        if fileManager.fileExists(atPath: kept.path) {
            // Kept longest when read most recently.
            try? fileManager.setAttributes([.modificationDate: Date()], ofItemAtPath: kept.path)
            return kept
        }
        let downloaded = try await client.download(fileID: file.fileID)
        try fileManager.createDirectory(at: folder, withIntermediateDirectories: true)
        // An older copy of the same File is stale now.
        for old in (try? fileManager.contentsOfDirectory(atPath: folder.path)) ?? [] where old.hasPrefix("\(file.fileID)-") {
            try? fileManager.removeItem(at: folder.appendingPathComponent(old))
        }
        try? fileManager.removeItem(at: kept)
        try fileManager.moveItem(at: downloaded, to: kept)
        return kept
    }

    /// Forgets the kept copy of a File, as when it wouldn't open.
    func discard(_ file: ReadingFile) {
        try? FileManager.default.removeItem(at: folder.appendingPathComponent("\(file.fileID)-\(file.sizeBytes).\(file.fileType)"))
    }
}
