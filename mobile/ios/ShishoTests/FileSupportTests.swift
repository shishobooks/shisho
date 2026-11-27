import XCTest
@testable import Shisho

/// A File of a type this version doesn't know shows as not supported, with
/// Download disabled, and doesn't count toward its Book's Download (ADR
/// 0021).
final class FileSupportTests: XCTestCase {
    func testAKnownFileTypeIsSupported() {
        for type in ["epub", "kepub", "azw3", "mobi", "m4b", "cbz", "pdf"] {
            let support = FileSupport(fileType: type)
            XCTAssertTrue(support.isSupported, type)
            XCTAssertNil(support.notice, "\(type) shows its facts")
        }
    }

    func testAnUnknownFileTypeSaysItsNotSupported() {
        let support = FileSupport(fileType: "cbr")
        XCTAssertFalse(support.isSupported)
        XCTAssertEqual(support.notice, "Not supported in this version of the app")
        XCTAssertEqual(support.downloadTitle, "Not supported")
        XCTAssertEqual(support.downloadLabel("Dune"), "Download Dune, not supported in this version of the app")
    }

    func testDownloadIsDisabledForAnUnknownFileTypeEvenOnline() {
        let support = FileSupport(fileType: "cbr")
        XCTAssertFalse(support.canDownload(online: true))
        XCTAssertFalse(support.canDownload(online: false))
        XCTAssertFalse(support.needsConnection(online: false), "it says it's not supported, not that it needs a connection")
    }

    func testDownloadOfAKnownFileTypeNeedsAConnection() {
        let support = FileSupport(fileType: "epub")
        XCTAssertEqual(support.downloadTitle, "Download")
        XCTAssertEqual(support.downloadLabel("Dune"), "Download Dune")
        XCTAssertTrue(support.canDownload(online: true))
        XCTAssertFalse(support.canDownload(online: false))
        XCTAssertFalse(support.needsConnection(online: true))
        XCTAssertTrue(support.needsConnection(online: false))
    }

    func testABookOffersDownloadOnlyWithAFileTypeItKnows() {
        XCTAssertTrue(FileSupport.canDownloadBook(fileTypes: ["cbr", "epub"]))
        XCTAssertFalse(FileSupport.canDownloadBook(fileTypes: ["cbr", "djvu"]), "every File is of a type it doesn't know")
        XCTAssertFalse(FileSupport.canDownloadBook(fileTypes: []))
    }
}
