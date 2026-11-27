import Foundation

/// What the app offers for a File, by whether this version knows its File
/// Type (ADR 0021). One it doesn't know is still listed on its Book's page,
/// saying it's not supported in place of its facts, with Download there but
/// disabled; the queries for Downloads leave it out
/// (`CatalogStore.downloadableTypes`).
struct FileSupport: Equatable, Sendable {
    let isSupported: Bool

    init(fileType: String) {
        isSupported = FileTypes.isKnown(fileType)
    }

    /// What the File's row on its Book's page says in place of its facts,
    /// or nil for a File Type this version knows.
    var notice: String? {
        isSupported ? nil : String(localized: "Not supported in this version of the app")
    }

    /// Download's title on the File's page.
    var downloadTitle: String {
        isSupported ? String(localized: "Download") : String(localized: "Not supported")
    }

    /// What VoiceOver reads for Download of the File named name.
    func downloadLabel(_ name: String) -> String {
        isSupported ? String(localized: "Download \(name)")
            : String(localized: "Download \(name), not supported in this version of the app")
    }

    /// Whether Download can be tapped: never for a File Type this version
    /// doesn't know, and otherwise while the server can be reached.
    func canDownload(online: Bool) -> Bool {
        isSupported && online
    }

    /// Whether Download is held back only by the connection, so it says it
    /// needs one. One not supported says that instead.
    func needsConnection(online: Bool) -> Bool {
        isSupported && !online
    }

    /// Whether a Book with nothing downloaded or on its way offers Download:
    /// only when one of its Files is of a type this version knows.
    static func canDownloadBook(fileTypes: some Sequence<String>) -> Bool {
        fileTypes.contains(where: FileTypes.isKnown)
    }
}
