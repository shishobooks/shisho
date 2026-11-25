import ShishoAPI
import XCTest
@testable import Shisho

/// The reader's model as the page drives it, with no page: the pages left
/// the bar reads out, and the display settings kept between books.
@MainActor
final class ReaderModelTests: XCTestCase {
    private var session: AccountSession!
    private var keptSettings: Data?

    override func setUp() async throws {
        let missing = FileManager.default.temporaryDirectory.appendingPathComponent("no-server-\(UUID().uuidString)", isDirectory: true)
        let account = Account(id: "test-\(UUID().uuidString)", serverURL: missing, username: "u", userID: 1,
                              deviceID: "me", serverVersion: "dev", thumbnailRenderKey: "", capabilities: [], signedOut: false)
        session = try AccountSession(account: account, credential: "credential") { _ in }
        try session.store.startOverIfNeeded()
        // The model keeps its settings in the app's own defaults, which this
        // test puts back as it found them.
        keptSettings = UserDefaults.standard.data(forKey: "reader.settings")
    }

    override func tearDown() async throws {
        UserDefaults.standard.set(keptSettings, forKey: "reader.settings")
        try? FileManager.default.removeItem(at: session.account.folder)
    }

    private func model() -> ReaderModel {
        let file = ReadingFile(fileID: 8, book: BookSummary(id: 1, libraryID: 1, title: "Book", authors: "", coverVersion: "",
                                                            coverAspectRatio: "book", fileTypes: ["epub"]),
                               fileType: "epub", displayName: "", sizeBytes: 10, chapters: [])
        return ReaderModel(file: file, session: session, requestSync: {}, onGone: {})
    }

    /// Page 2 of 10 in a section that's the first half of the book.
    private func page(nextChapter: PageLocation.NextChapter?) -> PageLocation {
        PageLocation(cfi: "epubcfi(/6/2!/4/2)", fraction: 0.1, start: 0.05,
                     section: .init(from: 0, to: 0.5, page: 2, pages: 10), nextChapter: nextChapter,
                     index: 0, page: 2, pages: 10, screen: "0:2")
    }

    private func text(_ label: AttributedString?) -> String? {
        label.map { String($0.characters) }
    }

    /// The readout counts the pages left in the chapter, or in the book.
    func testPagesLeftReadInTheChapterOrTheBook() {
        let model = model()
        XCTAssertNil(model.pagesLeftLabel(inBook: false), "none before a page shows")

        model.foliateShowed(page(nextChapter: .init(page: 6, atTop: true)))
        XCTAssertEqual(text(model.pagesLeftLabel(inBook: false)), "3 pages left in chapter")
        XCTAssertEqual(text(model.pagesLeftLabel(inBook: true)), "18 pages left in book")

        model.foliateMoved(page(nextChapter: .init(page: 3, atTop: true)))
        XCTAssertEqual(text(model.pagesLeftLabel(inBook: false)), "Last page in chapter")

        // Until the contents place the chapter's end, the book's count
        // stands in.
        model.foliateMoved(page(nextChapter: nil))
        XCTAssertEqual(text(model.pagesLeftLabel(inBook: false)), "18 pages left in book")
    }

    /// A setting changed in one book is kept, and the next book opens in it.
    func testAChangedSettingIsKeptForTheNextBook() {
        // Kept settings other than the ones the test sets, so it can't pass
        // on what an earlier run left.
        ReaderSettings().save()
        let first = model()
        XCTAssertNotEqual(first.settings.size, 21)
        XCTAssertNotEqual(first.settings.layout, .scrolled)
        first.settings.size = 21
        first.settings.layout = .scrolled
        let next = model()
        XCTAssertEqual(next.settings.size, 21)
        XCTAssertEqual(next.settings.layout, .scrolled)
    }

    /// The book (File 8) downloaded, as a finished transfer leaves it.
    private func downloadedBook() throws -> URL {
        try session.store.apply(makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Fiction","cover_aspect_ratio":"book"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Book","sort_title":"Book","created_at":"2026-01-01T00:00:00Z",
          "cover_version":"1-1","files":[{"id":8,"file_type":"epub","file_role":"main","size_bytes":10}]}}],"cursor":2}
        """.utf8)))
        try session.store.queueDownloads([8])
        let transferred = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try Data("abc".utf8).write(to: transferred)
        XCTAssertNotNil(try session.downloads.copies.publish(fileID: 8, from: transferred))
        return session.downloads.location(fileID: 8, fileType: "epub")
    }

    /// A Download that won't open stays: a page failing under memory
    /// pressure, say on a plane, mustn't cost the only copy. Only one this
    /// open just downloaded goes, to be downloaded again on Try Again.
    func testABookThatWontOpenKeepsItsDownload() throws {
        let book = try downloadedBook()
        model().foliateFailed()
        XCTAssertTrue(FileManager.default.fileExists(atPath: book.path))
        XCTAssertNotNil(session.downloads.localFile(fileID: 8))
    }

    /// Going back from the end undoes an automatic finish, so Remove when
    /// finished keeps the book.
    func testPeekingAtTheEndKeepsTheDownload() throws {
        let removeWhenFinished = DownloadSettings.removeWhenFinished
        defer { DownloadSettings.removeWhenFinished = removeWhenFinished }
        let book = try downloadedBook()
        session.downloads.removeWhenFinished = true
        let model = model()
        var end = page(nextChapter: nil)
        end.atEnd = true
        model.foliateMoved(end)
        model.foliateMoved(page(nextChapter: nil))
        model.close()
        XCTAssertTrue(FileManager.default.fileExists(atPath: book.path))

        // Reaching the end and staying there does remove it.
        let again = self.model()
        again.foliateMoved(end)
        again.close()
        XCTAssertFalse(FileManager.default.fileExists(atPath: book.path))
    }
}
