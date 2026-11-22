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
          "audiobook_duration_seconds":600,"chapters":[]}]}}],
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
}
