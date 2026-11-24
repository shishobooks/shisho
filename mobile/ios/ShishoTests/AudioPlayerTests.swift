import ShishoAPI
import XCTest
@testable import Shisho

/// The player against a server that isn't there, so every stream fails. It's
/// a missing folder rather than a closed port, which fails at once without
/// depending on the network, where a firewall can leave a connection hanging.
@MainActor
final class AudioPlayerTests: XCTestCase {
    private var session: AccountSession!
    private var player: AudioPlayer!

    override func setUp() async throws {
        let missing = FileManager.default.temporaryDirectory.appendingPathComponent("no-server-\(UUID().uuidString)", isDirectory: true)
        let account = Account(id: "test-\(UUID().uuidString)", serverURL: missing, username: "u", userID: 1,
                              deviceID: "me", serverVersion: "dev", thumbnailRenderKey: "", capabilities: [], signedOut: false)
        session = try AccountSession(account: account, credential: "credential") { _ in }
        try session.store.startOverIfNeeded()
        try session.store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Audio","cover_aspect_ratio":"audiobook"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Harbor Lights","sort_title":"Harbor Lights",
          "created_at":"2026-01-01T00:00:00Z","cover_version":"1-1","files":[{"id":5,"file_type":"m4b","file_role":"main","size_bytes":10,
          "audiobook_duration_seconds":600,"chapters":[{"id":10,"title":"The Harbor","start_timestamp_ms":0},
          {"id":11,"title":"The Lights","start_timestamp_ms":120000},{"id":12,"title":"The Keeper","start_timestamp_ms":300000}]},{"id":6,"file_type":"m4b","file_role":"main","size_bytes":10,
          "audiobook_duration_seconds":300,"chapters":[]}]}}],
         "cursor":2}
        """))
        player = AudioPlayer(session: session) {}
    }

    override func tearDown() async throws {
        player.stop()
        try? FileManager.default.removeItem(at: session.account.folder)
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func wait(_ what: String, timeout: TimeInterval = 15, until done: () -> Bool) async throws {
        let deadline = Date().addingTimeInterval(timeout)
        while !done() {
            guard Date() < deadline else { return XCTFail("timed out waiting for \(what)") }
            try await Task.sleep(for: .milliseconds(100))
        }
    }

    /// Trying a failed stream again keeps following the File, so a File
    /// that leaves the catalog still closes the player.
    func testRetryingAFailedStreamKeepsFollowingTheFile() async throws {
        player.open(fileID: 5, autoplay: true)
        try await wait("the stream to fail") { player.error != nil }
        player.play()
        XCTAssertNil(player.error)

        try session.store.apply(page(#"{"entries":[{"kind":"book","id":1,"deleted":true}],"cursor":3}"#))
        try await wait("the player to close") { player.file == nil }
    }

    /// Swiping the paused mini player away closes the File, and the next
    /// launch doesn't open it again; a playing one stays.
    func testDismissClosesAPausedFileForGood() throws {
        player.open(fileID: 5, autoplay: false)
        XCTAssertEqual(player.file?.fileID, 5)
        player.dismiss()
        XCTAssertNil(player.file)

        let next = AudioPlayer(session: session) {}
        next.restore()
        XCTAssertNil(next.file, "not opened again at the next launch")
        next.stop()
    }

    /// Opening a File that's gone leaves the one loaded alone, so a chapter
    /// tap on a File a sync just removed can't stop or move another.
    func testOpeningAMissingFileLeavesTheLoadedOne() throws {
        player.open(fileID: 5, autoplay: false)
        player.open(fileID: 999, autoplay: false)
        XCTAssertEqual(player.file?.fileID, 5)

        let next = AudioPlayer(session: session) {}
        next.restore()
        XCTAssertEqual(next.file?.fileID, 5, "still the one opened at the next launch")
        next.stop()
    }

    /// Speed is remembered per File on the device, since it suits the
    /// narrator, and a File never played starts at 1x.
    func testSpeedIsRememberedPerFile() {
        player.open(fileID: 5, autoplay: false)
        XCTAssertEqual(player.rate, 1)
        player.setRate(1.4)
        player.open(fileID: 6, autoplay: false)
        XCTAssertEqual(player.rate, 1)
        player.setRate(1.1)
        player.open(fileID: 5, autoplay: false)
        XCTAssertEqual(player.rate, 1.4, accuracy: 0.001)
        XCTAssertEqual(AudioPlayer.storedRate(accountID: session.account.id, fileID: 6), 1.1, accuracy: 0.001)
        XCTAssertEqual(AudioPlayer.storedRate(accountID: "another-account", fileID: 5), 1)
    }

    /// A Bookmark is made where playback is, through the player, so it's kept
    /// with no screen open.
    func testABookmarkIsMadeWherePlaybackIs() throws {
        player.open(fileID: 5, autoplay: false)
        player.seek(to: 83)
        player.addBookmark(note: "here")
        let bookmarks = try session.store.pool.read { try CatalogStore.bookmarks($0, fileID: 5) }
        XCTAssertEqual(bookmarks.map(\.seconds), [83])
        XCTAssertEqual(bookmarks.map(\.note), ["here"])
    }

    /// The chapter buttons move by the File's chapters from where playback
    /// is: Previous goes to the chapter's start, and to the one before just
    /// after a start; Next stops at the last.
    func testChapterButtonsMoveByTheFilesChapters() {
        player.open(fileID: 5, autoplay: false)
        player.seek(to: 150)
        player.previousChapter()
        XCTAssertEqual(player.position, 120)
        XCTAssertEqual(player.chapterTitle, "The Lights")
        player.previousChapter()
        XCTAssertEqual(player.position, 0)
        XCTAssertEqual(player.chapterTitle, "The Harbor")
        player.nextChapter()
        player.nextChapter()
        XCTAssertEqual(player.position, 300)
        XCTAssertEqual(player.chapterTitle, "The Keeper")
        player.nextChapter()
        XCTAssertEqual(player.position, 300, "Next in the last chapter stays")
    }
}
