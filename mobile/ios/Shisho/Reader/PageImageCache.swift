import UIKit

/// Decoded pages for the image pager, held up to a memory cap (ADR 0025).
///
/// Pages decode off the main thread at the size they're shown. The ones on
/// screen and their neighbors stay while they fit under the cap, least
/// recently used going first; a full-resolution page for zooming isn't
/// kept here at all, only by the page showing it. Decodes of pages the
/// reader has moved away from are cancelled.
@MainActor
final class PageImageCache {
    /// How soon a page is needed.
    enum Need {
        /// On screen now.
        case now
        /// A neighbor, which a turn shows next.
        case soon
        /// A thumbnail, or a page further off.
        case later

        var priority: TaskPriority {
            switch self {
            case .now: .userInitiated
            case .soon: .medium
            case .later: .utility
            }
        }
    }

    private struct Key: Hashable {
        let page: Int
        /// The size asked for, rounded up, so near sizes share an image.
        let bucket: Int
    }

    private struct Entry {
        let image: UIImage
        let cost: Int
        var used: Int
    }

    let source: PageImageSource
    /// The most bytes of decoded pages kept.
    let limit: Int
    private var images: [Key: Entry] = [:]
    private var inFlight: [Key: (task: Task<CGImage?, Never>, need: Need)] = [:]
    private(set) var cost = 0
    private var clock = 0

    /// A phone's screen-size page is around 14 MB, so this keeps the pages
    /// on screen and two either side.
    static let defaultLimit = 120 * 1024 * 1024

    init(source: PageImageSource, limit: Int = PageImageCache.defaultLimit) {
        self.source = source
        self.limit = limit
    }

    /// The size a request for maxPixel is decoded at.
    static func bucket(_ maxPixel: Int) -> Int {
        max(64, (min(maxPixel, PageImages.largestPixel) + 63) / 64 * 64)
    }

    /// The page at least maxPixel on its longer side, when it's kept.
    func cached(page: Int, maxPixel: Int) -> UIImage? {
        let bucket = Self.bucket(maxPixel)
        let key = images.keys.filter { $0.page == page && $0.bucket >= bucket }.min { $0.bucket < $1.bucket }
        guard let key, var entry = images[key] else { return nil }
        clock += 1
        entry.used = clock
        images[key] = entry
        return entry.image
    }

    /// The page at maxPixel, decoding it when it isn't kept. keep is false
    /// for a full-resolution page, which its view holds only while zoomed.
    func image(page: Int, maxPixel: Int, need: Need, keep: Bool = true) async -> UIImage? {
        if let image = cached(page: page, maxPixel: maxPixel) { return image }
        let key = Key(page: page, bucket: Self.bucket(maxPixel))
        let task: Task<CGImage?, Never>
        if let running = inFlight[key] {
            task = running.task
            if need.priority > running.need.priority { inFlight[key] = (task, need) }
        } else {
            let source = source
            task = Task.detached(priority: need.priority) { await source.image(page: page, maxPixel: key.bucket) }
            inFlight[key] = (task, need)
        }
        let decoded = await task.value
        if inFlight[key]?.task == task { inFlight[key] = nil }
        guard let decoded else { return nil }
        if let kept = images[key]?.image { return kept }
        let image = UIImage(cgImage: decoded)
        if keep { store(image, cost: decoded.bytesPerRow * decoded.height, for: key) }
        return image
    }

    /// Starts decoding a page that's about to be needed, without waiting.
    func prefetch(page: Int, maxPixel: Int, need: Need) {
        guard cached(page: page, maxPixel: maxPixel) == nil else { return }
        Task { _ = await self.image(page: page, maxPixel: maxPixel, need: need) }
    }

    /// Cancels prefetches of pages other than these, which the reader moved
    /// away from. A page on screen, and thumbnails, are left to finish.
    func cancel(except pages: Set<Int>) {
        for (key, running) in inFlight where !pages.contains(key.page) && running.need == .soon {
            running.task.cancel()
            inFlight[key] = nil
        }
    }

    /// Lets every kept page go, as on a memory warning. The pages showing
    /// keep theirs in their views.
    func purge() {
        images = [:]
        cost = 0
    }

    private func store(_ image: UIImage, cost: Int, for key: Key) {
        clock += 1
        images[key] = Entry(image: image, cost: cost, used: clock)
        self.cost += cost
        while self.cost > limit, images.count > 1, let oldest = images.min(by: { $0.value.used < $1.value.used }) {
            images[oldest.key] = nil
            self.cost -= oldest.value.cost
        }
    }
}
