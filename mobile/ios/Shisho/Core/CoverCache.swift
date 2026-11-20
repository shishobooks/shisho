import Foundation
import UIKit

/// Covers, kept on disk as they're first seen so they show offline. Each is
/// keyed by its Book's cover version, so an edited cover is a new file.
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

    func cachedImage(bookID: Int, version: String, size: Int) -> UIImage? {
        let key = cacheKey(bookID: bookID, version: version, size: size)
        if let image = memory[key] {
            return image
        }
        guard let data = try? Data(contentsOf: fileURL(key)), let image = UIImage(data: data) else {
            return nil
        }
        memory[key] = image
        return image
    }

    /// The cover, from memory, disk, or the server, or nil when there's
    /// none or the server can't be reached.
    func image(bookID: Int, version: String, size: Int) async -> UIImage? {
        guard !version.isEmpty else { return nil }
        if let cached = cachedImage(bookID: bookID, version: version, size: size) {
            return cached
        }
        guard let data = try? await client.cover(bookID: bookID, version: version, size: size, renderKey: renderKey),
              let image = UIImage(data: data)
        else { return nil }
        let key = cacheKey(bookID: bookID, version: version, size: size)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        removeOlderVersions(bookID: bookID, keeping: version)
        try? data.write(to: fileURL(key), options: .atomic)
        memory[key] = image
        return image
    }

    private func cacheKey(bookID: Int, version: String, size: Int) -> String {
        "\(bookID)-\(version)-\(size)-r\(renderKey)"
    }

    private func fileURL(_ key: String) -> URL {
        folder.appendingPathComponent(key + ".jpg")
    }

    private func removeOlderVersions(bookID: Int, keeping version: String) {
        let prefix = "\(bookID)-"
        let current = "\(bookID)-\(version)-"
        let names = (try? FileManager.default.contentsOfDirectory(atPath: folder.path)) ?? []
        for name in names where name.hasPrefix(prefix) && !name.hasPrefix(current) {
            try? FileManager.default.removeItem(at: folder.appendingPathComponent(name))
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
