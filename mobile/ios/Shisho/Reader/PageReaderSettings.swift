import Foundation

/// The page reader's settings, kept on the device: whether right-to-left
/// Files turn the other way, for every account as the web keeps it per
/// browser, and the Page flow chosen for each of an account's Books, which
/// otherwise follows the pages' shapes (PageLayout.detectedFlow). Books are
/// numbered per server, so each account keeps its own choices.
struct PageReaderSettings: Equatable, Sendable {
    private static let followKey = "reader.followReadingDirection"
    private static func flowsKey(_ accountID: String) -> String { "reader.pageFlows.\(accountID)" }

    let accountID: String

    /// Right-to-left Files, as manga, turn right to left. Off, every File
    /// turns left to right.
    var followReadingDirection = true
    /// The flow chosen for each Book, by its id.
    private(set) var flows: [Int: PageFlow] = [:]

    func flow(forBook bookID: Int) -> PageFlow? { flows[bookID] }

    mutating func choose(_ flow: PageFlow, forBook bookID: Int) {
        flows[bookID] = flow
    }

    /// Whether a File with readingDirection turns right to left.
    func rightToLeft(_ file: ReadingFile) -> Bool {
        followReadingDirection && file.rightToLeft
    }

    init(accountID: String) {
        self.accountID = accountID
    }

    static func load(accountID: String, _ defaults: UserDefaults = .standard) -> PageReaderSettings {
        var settings = PageReaderSettings(accountID: accountID)
        if defaults.object(forKey: followKey) != nil {
            settings.followReadingDirection = defaults.bool(forKey: followKey)
        }
        // Keys are Book ids as text, as a property list's keys must be.
        let stored = defaults.dictionary(forKey: flowsKey(accountID)) as? [String: String] ?? [:]
        for (book, flow) in stored {
            if let id = Int(book), let flow = PageFlow(rawValue: flow) { settings.flows[id] = flow }
        }
        return settings
    }

    func save(_ defaults: UserDefaults = .standard) {
        defaults.set(followReadingDirection, forKey: Self.followKey)
        defaults.set(Dictionary(uniqueKeysWithValues: flows.map { (String($0.key), $0.value.rawValue) }),
                     forKey: Self.flowsKey(accountID))
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
