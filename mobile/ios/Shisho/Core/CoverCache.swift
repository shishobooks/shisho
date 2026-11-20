import Foundation
import UIKit

/// Covers, kept on disk as they're first seen so they show offline. Each is
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
        folder = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Covers", isDirectory: true)
            .appendingPathComponent(accountID, isDirectory: true)
    }

    /// The cover thumbnail size, in pixels, for a view this many points
    /// wide, from the sizes the server makes.
    static func pixelSize(forWidth points: CGFloat, scale: CGFloat) -> Int {
        let pixels = Int(points * scale)
        return [128, 256, 512, 1024].first { $0 >= pixels } ?? 1024
    }

    /// The cover if it's in memory: what a view can show at once, without
    /// waiting for the disk.
    func memoryImage(bookID: Int, version: String, size: Int) -> UIImage? {
        memory[fileName(version: version, size: size, bookID: bookID)]
    }

    /// The cover, from memory, disk, or the server, or nil when there's
    /// none or the server can't be reached. It runs off the main actor.
    func image(bookID: Int, version: String, size: Int) async -> UIImage? {
        guard !version.isEmpty else { return nil }
        let name = fileName(version: version, size: size, bookID: bookID)
        if let image = memory[name] {
            return image
        }
        let file = bookFolder(bookID).appendingPathComponent(name)
        if let data = try? Data(contentsOf: file), let image = UIImage(data: data) {
            memory[name] = image
            return image
        }
        guard let data = try? await client.cover(bookID: bookID, version: version, size: size, renderKey: renderKey),
              let image = UIImage(data: data)
        else { return nil }
        removeOlderVersions(bookID: bookID, keeping: version)
        try? FileManager.default.createDirectory(at: bookFolder(bookID), withIntermediateDirectories: true)
        try? data.write(to: file, options: .atomic)
        memory[name] = image
        return image
    }

    private func bookFolder(_ bookID: Int) -> URL {
        folder.appendingPathComponent(String(bookID), isDirectory: true)
    }

    /// Unique across Books too, since the memory cache holds every Book's.
    private func fileName(version: String, size: Int, bookID: Int) -> String {
        "\(bookID)-\(version)-\(size)-r\(renderKey).jpg"
    }

    /// Deletes the Book's covers of other versions: an edited cover
    /// replaces them.
    private func removeOlderVersions(bookID: Int, keeping version: String) {
        let current = "\(bookID)-\(version)-"
        let names = (try? FileManager.default.contentsOfDirectory(atPath: bookFolder(bookID).path)) ?? []
        for name in names where !name.hasPrefix(current) {
            try? FileManager.default.removeItem(at: bookFolder(bookID).appendingPathComponent(name))
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
        return cache
    }()

    subscript(key: String) -> UIImage? {
        get { cache.object(forKey: key as NSString) }
        set {
            if let newValue {
                cache.setObject(newValue, forKey: key as NSString)
            } else {
                cache.removeObject(forKey: key as NSString)
            }
        }
    }
}
