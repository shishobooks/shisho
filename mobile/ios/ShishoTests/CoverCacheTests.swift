import UIKit
import XCTest
@testable import Shisho

final class CoverCacheTests: XCTestCase {
    /// Offline, a cover kept at another size stands in for the one asked
    /// for, so a Book seen in the grid still has its cover on its own page.
    /// Another version of the cover never does: it was replaced.
    func testFallsBackToAnotherSizeOfTheSameCoverOffline() async throws {
        let account = UUID().uuidString
        let unreachable = APIClient(baseURL: URL(string: "http://127.0.0.1:9")!, credential: nil)
        let cache = CoverCache(client: unreachable, accountID: account, renderKey: "1")
        let folder = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Covers/\(account)/7", isDirectory: true)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: folder.deletingLastPathComponent()) }
        let png = UIGraphicsImageRenderer(size: CGSize(width: 2, height: 3)).pngData { context in
            UIColor.red.setFill()
            context.fill(CGRect(x: 0, y: 0, width: 2, height: 3))
        }
        try png.write(to: folder.appendingPathComponent("7-v1-512-r1.jpg"))

        let stand = await cache.image(bookID: 7, version: "v1", size: 1024)
        XCTAssertNotNil(stand)
        let replaced = await cache.image(bookID: 7, version: "v2", size: 1024)
        XCTAssertNil(replaced)
    }
}
