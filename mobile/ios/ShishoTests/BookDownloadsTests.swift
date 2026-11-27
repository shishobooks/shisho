import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// A whole Book's Downloads: which Files it covers, where they stand
/// together for its Download button, and how far they've got for its
/// cell's ring.
final class BookDownloadsTests: XCTestCase {
    private typealias File = BookDownloads.File

    private func book(_ files: [File]) -> BookDownloads {
        BookDownloads(bookID: 1, files: files)
    }

    // MARK: - Phase

    func testNothingDownloadedOffersEveryFileSupplementsToo() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: nil, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: nil, supplement: false),
                              File(fileID: 12, sizeBytes: 50, state: nil, supplement: true)])
        XCTAssertEqual(downloads.phase(progress: [:]), .notDownloaded(files: 3, bytes: 450), "a supplement comes with the Book")
        XCTAssertEqual(downloads.missing.map(\.fileID), [10, 11, 12])
        XCTAssertFalse(downloads.isDownloading)
        XCTAssertFalse(downloads.hasDownloaded)
    }

    func testAFailedDownloadCountsAsNotDownloaded() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: .failed, supplement: false)])
        XCTAssertEqual(downloads.phase(progress: [:]), .notDownloaded(files: 1, bytes: 300))
    }

    func testSomeDownloadedOffersTheRest() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: .done, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: nil, supplement: false)])
        XCTAssertEqual(downloads.phase(progress: [:]), .partlyDownloaded(files: 1, bytes: 100))
    }

    func testEverythingDownloaded() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: .done, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: .done, supplement: false),
                              File(fileID: 12, sizeBytes: 50, state: .done, supplement: true)])
        XCTAssertEqual(downloads.phase(progress: [:]), .downloaded(files: 3, bytes: 450))
        XCTAssertEqual(downloads.removable, [10, 11, 12], "removing takes the supplement downloaded on its own too")
    }

    func testAnythingOnItsWayIsDownloading() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: .done, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: .queued, supplement: false),
                              File(fileID: 12, sizeBytes: 100, state: nil, supplement: false)])
        XCTAssertEqual(downloads.phase(progress: [11: 0.5]), .downloading(350.0 / 400))
        XCTAssertEqual(downloads.cancellable, [11])
    }

    func testABookWithNothingItCanDownload() {
        XCTAssertEqual(BookDownloads.empty(1).phase(progress: [:]), .unavailable)
    }

    func testCancellingKeepsAReplacedFilesOldCopy() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: .queued, supplement: false, replacing: true),
                              File(fileID: 11, sizeBytes: 100, state: .queued, supplement: false)])
        XCTAssertEqual(downloads.cancellable, [11])
        XCTAssertTrue(downloads.hasDownloaded, "the old copy is still on the device")
    }

    // MARK: - Fraction

    func testTheFractionWeighsEachFileBySize() {
        let downloads = book([File(fileID: 10, sizeBytes: 300, state: .queued, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: .queued, supplement: false)])
        XCTAssertEqual(downloads.fraction(progress: [10: 0.5]) ?? -1, 150.0 / 400, accuracy: 0.0001)
        XCTAssertEqual(downloads.fraction(progress: [10: 1, 11: 0.5]) ?? -1, 350.0 / 400, accuracy: 0.0001)
    }

    func testAFileThatArrivesKeepsCountingSoTheRingDoesntGoBack() {
        let downloads = book([File(fileID: 10, sizeBytes: 100, state: .done, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: .queued, supplement: false)])
        XCTAssertEqual(downloads.fraction(progress: [:]) ?? -1, 0.5, accuracy: 0.0001)
    }

    func testNothingKnownYetIsIndeterminate() {
        let downloads = book([File(fileID: 10, sizeBytes: 100, state: .queued, supplement: false),
                              File(fileID: 11, sizeBytes: 100, state: nil, supplement: false)])
        XCTAssertNil(downloads.fraction(progress: [:]))
        XCTAssertEqual(downloads.fraction(progress: [10: 0.25]) ?? -1, 0.25, accuracy: 0.0001,
                       "a File not asked for doesn't hold the ring back")
    }

    func testFilesWithoutSizesCountTheSame() {
        let downloads = book([File(fileID: 10, sizeBytes: 0, state: .done, supplement: false),
                              File(fileID: 11, sizeBytes: 0, state: .queued, supplement: false)])
        XCTAssertEqual(downloads.fraction(progress: [11: 0.5]) ?? -1, 0.75, accuracy: 0.0001)
    }

    func testWordsForVoiceOver() {
        let downloading = book([File(fileID: 10, sizeBytes: 100, state: .queued, supplement: false)])
        XCTAssertEqual(downloading.words(progress: [10: 0.4]), "Downloading, 40%")
        XCTAssertEqual(downloading.words(progress: [:]), "Downloading")
        XCTAssertEqual(book([File(fileID: 10, sizeBytes: 100, state: .done, supplement: false)]).words(progress: [:]), "Downloaded")
        XCTAssertNil(book([File(fileID: 10, sizeBytes: 100, state: nil, supplement: false)]).words(progress: [:]))
    }

    @MainActor
    func testTheButtonsTitle() {
        let size = FileFacts.size(400)
        XCTAssertEqual(BookDownloadButton.downloadTitle(files: 2, bytes: 400, partly: false), "Download all · \(size)")
        XCTAssertEqual(BookDownloadButton.downloadTitle(files: 1, bytes: 400, partly: false), "Download · \(size)")
        XCTAssertEqual(BookDownloadButton.downloadTitle(files: 1, bytes: 400, partly: true), "Download the rest · \(size)")
    }
}

/// The Book Downloads read from the account's database.
final class BookDownloadsStoreTests: XCTestCase {
    private var store: CatalogStore!

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, files: [(10, "m4b", "main"), (11, "epub", "main"), (12, "pdf", "supplement")])),\#(book(2, files: [(20, "epub", "main"), (21, "cbr", "main")])),\#(book(3, files: [(30, "epub", "main")]))],"cursor":3}"#))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private let library = #"{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Fiction","cover_aspect_ratio":"book"}}"#

    private func book(_ id: Int, files: [(Int, String, String)]) -> String {
        let fileJSON = files.map {
            #"{"id":\#($0.0),"file_type":"\#($0.1)","file_role":"\#($0.2)","size_bytes":\#($0.0 * 100),"display_name":"File \#($0.0)"}"#
        }.joined(separator: ",")
        return """
        {"kind":"book","id":\(id),"library_id":1,"book":{"id":\(id),"library_id":1,"title":"Book \(id)","sort_title":"Book \(id)",
         "created_at":"2026-01-01T00:00:00Z","series":[],"files":[\(fileJSON)],"cover_version":"\(id)-1"}}
        """
    }

    func testABooksDownloadsAreTheFilesItKnows() throws {
        let book = try store.pool.read { try CatalogStore.bookDownloads($0, bookID: 1) }
        XCTAssertEqual(book.files.map(\.fileID), [10, 11, 12], "the supplement comes with it")
        XCTAssertEqual(book.phase(progress: [:]), .notDownloaded(files: 3, bytes: 3300))

        let unknown = try store.pool.read { try CatalogStore.bookDownloads($0, bookID: 2) }
        XCTAssertEqual(unknown.files.map(\.fileID), [20], "a File Type this version doesn't know isn't")
    }

    func testADownloadedSupplementIsCounted() throws {
        try store.queueDownloads([12])
        try store.markDownloaded(fileID: 12, sizeBytes: 1150)
        let book = try store.pool.read { try CatalogStore.bookDownloads($0, bookID: 1) }
        XCTAssertEqual(book.files.map(\.fileID), [10, 11, 12])
        XCTAssertEqual(book.files.last?.sizeBytes, 1150, "the bytes on the device once it's here")
        XCTAssertEqual(book.phase(progress: [:]), .partlyDownloaded(files: 2, bytes: 2100), "the main Files are left")
    }

    func testTheGridReadsOnlyBooksWithDownloads() throws {
        XCTAssertEqual(try store.pool.read(CatalogStore.booksDownloads), [:])
        try store.queueDownloads([10, 30])
        try store.markDownloaded(fileID: 30, sizeBytes: 3000)
        let books = try store.pool.read(CatalogStore.booksDownloads)
        XCTAssertEqual(Set(books.keys), [1, 3])
        XCTAssertEqual(books[1]?.files.map(\.state), [.queued, nil, nil], "its supplement too")
        XCTAssertTrue(books[1]?.isDownloading == true)
        XCTAssertEqual(books[3]?.phase(progress: [:]), .downloaded(files: 1, bytes: 3000))
    }

    func testTheGridLeavesOutFileTypesItDoesntKnow() throws {
        try store.queueDownloads([20])
        let books = try store.pool.read(CatalogStore.booksDownloads)
        XCTAssertEqual(books[2]?.files.map(\.fileID), [20], "the File of a type this version doesn't know isn't counted")
        XCTAssertEqual(books[2]?.phase(progress: [:]), .downloading(nil))
    }

    func testABookOfOnlyFileTypesItDoesntKnowHasNothingToDownload() throws {
        try store.apply(page(#"{"entries":[\#(book(4, files: [(40, "cbr", "main"), (41, "djvu", "supplement")]))],"cursor":4}"#))
        let book = try store.pool.read { try CatalogStore.bookDownloads($0, bookID: 4) }
        XCTAssertEqual(book.files, [])
        XCTAssertEqual(book.phase(progress: [:]), .unavailable, "its Download button shows nothing")
        XCTAssertEqual(try store.queueDownloads([40, 41]), [])
    }
}
