import Foundation
import UIKit

/// Covers, kept on disk as they're first seen so they show offline, in the
/// account's folder in Application Support, which iOS never clears, unlike
/// Caches, so ones downloaded for a trip stay. Each is
/// keyed by its Book's cover version, so an edited cover is a new file, and
/// each Book's covers have a folder of their own, so replacing one never
/// lists every cover.
final class CoverCache: Sendable {
    private let client: APIClient
    private let folder: URL
    private let renderKey: String
    private let memory = MemoryCache()

    init(client: APIClient, accountID: String, renderKey: String) {
        self.client = client
        self.renderKey = renderKey
        folder = CoverCache.folder(accountID: accountID)
        CoverCache.prepare(folder)
    }

    /// The folder an account's covers are kept in.
    static func folder(accountID: String) -> URL {
        AccountData(accountID: accountID).folder.appendingPathComponent("Covers", isDirectory: true)
    }

    /// Makes the covers folder, kept out of iCloud backup as Downloads are:
    /// every cover can be downloaded again.
    private static func prepare(_ folder: URL) {
        guard !FileManager.default.fileExists(atPath: folder.path) else { return }
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        var folder = folder
        try? folder.setResourceValues(values)
    }

    /// The cover thumbnail size, in pixels, for a view this many points
    /// wide, from the sizes the server makes.
    static func pixelSize(forWidth points: CGFloat, scale: CGFloat) -> Int {
        let pixels = Int(points * scale)
        return [128, 256, 512, 1024].first { $0 >= pixels } ?? 1024
    }

    /// Whose cover: a Book's, as the Library shows it, or one File's own.
    enum Owner: Hashable, Sendable {
        case book(Int)
        case file(Int)

        var isFile: Bool {
            if case .file = self { return true }
            return false
        }

        /// The folder that holds the owner's covers, and starts their names.
        fileprivate var name: String {
            switch self {
            case .book(let id): String(id)
            case .file(let id): "file-\(id)"
            }
        }
    }

    /// The cover if it's in memory: what a view can show at once, without
    /// waiting for the disk.
    func memoryImage(_ owner: Owner, version: String, size: Int) -> UIImage? {
        memory[fileName(owner, version: version, size: size)]
    }

    /// The cover, from memory, disk, or the server, or nil when there's
    /// none or the server can't be reached. It runs off the main actor.
    func image(bookID: Int, version: String, size: Int) async -> UIImage? {
        await image(.book(bookID), version: version, size: size)
    }

    func image(_ owner: Owner, version: String, size: Int) async -> UIImage? {
        guard !version.isEmpty else { return nil }
        let name = fileName(owner, version: version, size: size)
        if let image = memory[name] {
            return image
        }
        let file = folder(owner).appendingPathComponent(name)
        if let data = try? Data(contentsOf: file), let image = await Self.decoded(data) {
            memory[name] = image
            return image
        }
        let fetched: Data?
        switch owner {
        case .book(let id):
            fetched = try? await client.cover(bookID: id, version: version, size: size, renderKey: renderKey)
        case .file(let id):
            fetched = try? await client.fileCover(fileID: id, version: version, size: size, renderKey: renderKey)
        }
        guard let data = fetched, let image = await Self.decoded(data) else {
            return otherSize(owner, version: version)
        }
        // A view that moved on to a newer version cancelled this download;
        // its answer must not prune the newer version from disk.
        guard !Task.isCancelled else { return image }
        removeOlderVersions(owner, keeping: version)
        try? FileManager.default.createDirectory(at: folder(owner), withIntermediateDirectories: true)
        try? data.write(to: file, options: .atomic)
        memory[name] = image
        return image
    }

    /// The largest size kept of this version of the cover, for when the size
    /// asked for can't be fetched: a Book seen in the grid offline still has
    /// its cover on its own page.
    private func otherSize(_ owner: Owner, version: String) -> UIImage? {
        let prefix = "\(owner.name)-\(version)-"
        let names = (try? FileManager.default.contentsOfDirectory(atPath: folder(owner).path)) ?? []
        let largest = names
            .filter { $0.hasPrefix(prefix) && $0.hasSuffix("-r\(renderKey).jpg") }
            .max { size(of: $0, after: prefix) < size(of: $1, after: prefix) }
        guard let largest, let data = try? Data(contentsOf: folder(owner).appendingPathComponent(largest)) else {
            return nil
        }
        return UIImage(data: data)
    }

    /// The cover decoded here, off the main thread, rather than when it's
    /// first drawn, which would decode it on the main thread as a grid
    /// scrolls.
    private static func decoded(_ data: Data) async -> UIImage? {
        guard let image = UIImage(data: data) else { return nil }
        return await image.byPreparingForDisplay() ?? image
    }

    private func size(of name: String, after prefix: String) -> Int {
        Int(name.dropFirst(prefix.count).prefix { $0.isNumber }) ?? 0
    }

    private func folder(_ owner: Owner) -> URL {
        self.folder.appendingPathComponent(owner.name, isDirectory: true)
    }

    /// Unique across owners too, since the memory cache holds every one's.
    private func fileName(_ owner: Owner, version: String, size: Int) -> String {
        "\(owner.name)-\(version)-\(size)-r\(renderKey).jpg"
    }

    /// Deletes the owner's covers of other versions: an edited cover
    /// replaces them.
    private func removeOlderVersions(_ owner: Owner, keeping version: String) {
        let current = "\(owner.name)-\(version)-"
        let names = (try? FileManager.default.contentsOfDirectory(atPath: folder(owner).path)) ?? []
        for name in names where !name.hasPrefix(current) {
            try? FileManager.default.removeItem(at: folder(owner).appendingPathComponent(name))
        }
    }
}

/// Recently shown covers, dropped first when memory runs low.
// concurrency-exception: NSCache is documented as safe to use from any
// thread; it just isn't marked Sendable. Only it is stored here.
private final class MemoryCache: @unchecked Sendable {
    private let cache: NSCache<NSString, UIImage> = {
        let cache = NSCache<NSString, UIImage>()
        cache.countLimit = 300
        // Decoded, a cover takes 4 bytes a pixel whatever its file size.
        cache.totalCostLimit = 64 * 1024 * 1024
        return cache
    }()

    subscript(key: String) -> UIImage? {
        get { cache.object(forKey: key as NSString) }
        set {
            if let newValue {
                let pixels = newValue.size.width * newValue.scale * newValue.size.height * newValue.scale
                cache.setObject(newValue, forKey: key as NSString, cost: Int(pixels) * 4)
            } else {
                cache.removeObject(forKey: key as NSString)
            }
        }
    }
}
