import Foundation

/// The page reader's settings kept on the device, for every account on it,
/// as the web keeps them per browser: whether right-to-left Files turn the
/// other way. The Page flow is the Book's, kept on the server with the
/// user's record of it (CatalogStore.pageFlow).
struct PageReaderSettings: Equatable, Sendable {
    private static let followKey = "reader.followReadingDirection"

    /// Right-to-left Files, as manga, turn right to left. Off, every File
    /// turns left to right.
    var followReadingDirection = true

    /// Whether a File with readingDirection turns right to left.
    func rightToLeft(_ file: ReadingFile) -> Bool {
        followReadingDirection && file.rightToLeft
    }

    static func load(_ defaults: UserDefaults = .standard) -> PageReaderSettings {
        var settings = PageReaderSettings()
        if defaults.object(forKey: followKey) != nil {
            settings.followReadingDirection = defaults.bool(forKey: followKey)
        }
        return settings
    }

    func save(_ defaults: UserDefaults = .standard) {
        defaults.set(followReadingDirection, forKey: Self.followKey)
    }
}

extension PageFlow {
    var label: LocalizedStringResource {
        switch self {
        case .paginated: "Paginated"
        case .scrolled: "Scrolled"
        }
    }
}
