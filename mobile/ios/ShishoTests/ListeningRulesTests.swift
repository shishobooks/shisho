import XCTest
@testable import Shisho

final class ListeningRulesTests: XCTestCase {
    private let start = Date(timeIntervalSince1970: 1_800_000_000)

    private func at(_ seconds: TimeInterval) -> Date {
        start.addingTimeInterval(seconds)
    }

    /// A pause of up to five minutes keeps the session going without
    /// counting the pause.
    func testAShortPauseKeepsTheSessionAndIsNotCounted() throws {
        var (sitting, ended) = ListeningRules.play(nil, fileID: 5, at: at(0), percentage: 0.1)
        XCTAssertNil(ended)
        sitting = ListeningRules.pause(sitting, at: at(60), percentage: 0.2)
        (sitting, ended) = ListeningRules.play(sitting, fileID: 5, at: at(60 + 240), percentage: 0.2)
        XCTAssertNil(ended)
        sitting = ListeningRules.pause(sitting, at: at(400), percentage: 0.3)

        let session = try XCTUnwrap(ListeningRules.finish(sitting))
        XCTAssertEqual(session.sittingID, sitting.id)
        XCTAssertEqual(session.startedAt, at(0))
        XCTAssertEqual(session.endedAt, at(400))
        XCTAssertEqual(session.duration, 160)
        XCTAssertEqual(session.startPercentage, 0.1)
        XCTAssertEqual(session.endPercentage, 0.3)
    }

    /// A longer pause splits it: the first session ends where the pause
    /// began.
    func testALongPauseSplitsTheSession() throws {
        var (sitting, _) = ListeningRules.play(nil, fileID: 5, at: at(0), percentage: 0)
        sitting = ListeningRules.pause(sitting, at: at(90), percentage: 0.1)
        XCTAssertFalse(ListeningRules.isOver(sitting, at: at(90 + 300)))
        XCTAssertTrue(ListeningRules.isOver(sitting, at: at(90 + 301)))

        let (next, ended) = ListeningRules.play(sitting, fileID: 5, at: at(1000), percentage: 0.1)
        let first = try XCTUnwrap(ended)
        XCTAssertEqual(first.endedAt, at(90))
        XCTAssertEqual(first.duration, 90)
        XCTAssertEqual(next.startedAt, at(1000))
        XCTAssertNotEqual(next.id, sitting.id)
        XCTAssertEqual(next.played(at: at(1010)), 10)
    }

    /// Playing another File ends the sitting.
    func testAnotherFileEndsTheSitting() throws {
        var (sitting, _) = ListeningRules.play(nil, fileID: 5, at: at(0), percentage: 0)
        sitting = ListeningRules.pause(sitting, at: at(30), percentage: 0.1)
        let (next, ended) = ListeningRules.play(sitting, fileID: 6, at: at(40), percentage: 0)
        XCTAssertEqual(ended?.fileID, 5)
        XCTAssertEqual(next.fileID, 6)
    }

    /// A sitting the app was killed during ends at its last save.
    func testASittingFoundAfterARelaunchEndsAtItsLastSave() throws {
        var (sitting, _) = ListeningRules.play(nil, fileID: 5, at: at(0), percentage: 0)
        sitting = ListeningRules.tick(sitting, at: at(45), percentage: 0.2)
        let data = try JSONEncoder().encode(sitting)
        let restored = try JSONDecoder().decode(ListeningSitting.self, from: data)

        let session = try XCTUnwrap(ListeningRules.finish(restored))
        XCTAssertEqual(session.endedAt, at(45))
        XCTAssertEqual(session.duration, 45)
    }

    func testNothingPlayedMakesNoSession() {
        let (sitting, _) = ListeningRules.play(nil, fileID: 5, at: at(0), percentage: 0)
        XCTAssertNil(ListeningRules.finish(ListeningRules.pause(sitting, at: at(0.5), percentage: 0)))
    }
}

final class PlaceRulesTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func server(_ seconds: Double, at offset: TimeInterval, device: String? = "other") -> ServerPlace {
        ServerPlace(seconds: seconds, occurredAt: now.addingTimeInterval(offset), deviceID: device, source: "web_reader")
    }

    private func local(_ seconds: Double, at offset: TimeInterval, pending: Bool, base: TimeInterval? = nil) -> LocalPlace {
        LocalPlace(seconds: seconds, updatedAt: now.addingTimeInterval(offset), pending: pending,
                   base: base.map { now.addingTimeInterval($0) })
    }

    /// Unmoved since its last sync, the device takes the newest synced place.
    func testAnUnmovedDeviceJumpsToTheNewestPlace() {
        let choice = PlaceRules.open(local: local(100, at: -600, pending: false), server: server(900, at: -60), ownDeviceID: "me")
        XCTAssertEqual(choice, PlaceRules.Choice(start: 900, offer: nil))
    }

    /// A feed that hasn't brought back the device's own last move yet
    /// doesn't send it back to the place it knew before moving.
    func testAnOlderServerPlaceDoesntRewind() {
        let choice = PlaceRules.open(local: local(300, at: -10, pending: false, base: -600), server: server(100, at: -600), ownDeviceID: "me")
        XCTAssertEqual(choice.start, 300)
        let own = PlaceRules.open(local: local(300, at: -10, pending: false), server: server(290, at: -5, device: "me"), ownDeviceID: "me")
        XCTAssertEqual(own.start, 300)
    }

    /// The phone's clock never decides: a phone running an hour fast still
    /// takes a newer place from the web.
    func testAFastPhoneClockStillTakesTheNewerPlace() {
        let choice = PlaceRules.open(local: local(100, at: 3600, pending: false, base: -7200), server: server(900, at: -60), ownDeviceID: "me")
        XCTAssertEqual(choice.start, 900)
    }

    /// Both moved: the device stays and offers the other place.
    func testBothMovedOffersWithoutJumping() {
        let other = server(900, at: -60)
        let choice = PlaceRules.open(local: local(300, at: -30, pending: true, base: -3600), server: other, ownDeviceID: "me")
        XCTAssertEqual(choice, PlaceRules.Choice(start: 300, offer: other))
    }

    /// Only this device moved: nothing to offer.
    func testOnlyThisDeviceMoved() {
        let mine = server(100, at: -3600, device: "me")
        XCTAssertNil(PlaceRules.open(local: local(300, at: -30, pending: true, base: -3600), server: mine, ownDeviceID: "me").offer)
        let unchanged = server(100, at: -3600)
        XCTAssertNil(PlaceRules.open(local: local(300, at: -30, pending: true, base: -3600), server: unchanged, ownDeviceID: "me").offer)
    }

    func testANearbyPlaceIsntOffered() {
        let choice = PlaceRules.open(local: local(300, at: -30, pending: true), server: server(303, at: -10), ownDeviceID: "me")
        XCTAssertNil(choice.offer)
    }

    func testAPlaceArrivingWhileListeningIsOnlyOffered() {
        let known = now.addingTimeInterval(-3600)
        XCTAssertNotNil(PlaceRules.offerDuringListening(server: server(900, at: -10), at: 200, known: known, ownDeviceID: "me"))
        XCTAssertNil(PlaceRules.offerDuringListening(server: server(900, at: -3600), at: 200, known: known, ownDeviceID: "me"))
        XCTAssertNil(PlaceRules.offerDuringListening(server: server(900, at: -10, device: "me"), at: 200, known: known, ownDeviceID: "me"))
    }

    /// A place moved on the web before listening began, which the feed
    /// brings only after, is still offered: it's newer than the place the
    /// player knew.
    func testAPlaceFromBeforeTheSittingThatArrivesLateIsOffered() {
        let webMove = server(900, at: -120)
        XCTAssertEqual(PlaceRules.offerDuringListening(server: webMove, at: 30, known: now.addingTimeInterval(-86400), ownDeviceID: "me"), webMove)
        XCTAssertNotNil(PlaceRules.offerDuringListening(server: webMove, at: 30, known: nil, ownDeviceID: "me"))
    }
}

final class AudioChaptersTests: XCTestCase {
    private let chapters = [AudioChapter(title: "One", start: 0), AudioChapter(title: "Two", start: 60), AudioChapter(title: "Three", start: 120)]

    func testChapterNavigation() {
        XCTAssertEqual(AudioChapters.index(at: 59.9, in: chapters), 1)
        XCTAssertEqual(AudioChapters.index(at: 30, in: chapters), 0)
        XCTAssertNil(AudioChapters.index(at: 30, in: []))
        XCTAssertEqual(AudioChapters.nextTarget(from: 30, in: chapters), 60)
        XCTAssertNil(AudioChapters.nextTarget(from: 130, in: chapters))
        XCTAssertEqual(AudioChapters.previousTarget(from: 70, in: chapters), 60)
        XCTAssertEqual(AudioChapters.previousTarget(from: 61, in: chapters), 0)
        XCTAssertEqual(AudioChapters.end(of: 2, in: chapters, duration: 200), 200)
    }

    func testSpeedSteps() {
        XCTAssertEqual(PlaybackSpeed.clamp(1.0 + 0.1 + 0.1 + 0.1), 1.3)
        XCTAssertEqual(PlaybackSpeed.clamp(9), 4)
        XCTAssertEqual(PlaybackSpeed.clamp(0.1), 0.5)
        XCTAssertEqual(PlaybackSpeed.label(1.4), "1.4x")
        XCTAssertEqual(TimeLabels.clock(3725), "1:02:05")
        XCTAssertEqual(TimeLabels.clock(65), "1:05")
        XCTAssertEqual(TimeLabels.clock(21.9), "0:21")
    }

    /// The time left is at the speed playing, and never below nothing.
    func testTimeLeftIsAtTheSpeed() {
        XCTAssertEqual(PlaybackSpeed.timeLeft(from: 30, to: 100, at: 1.4), 50, accuracy: 0.001)
        XCTAssertEqual(PlaybackSpeed.timeLeft(from: 30, to: 100, at: 1), 70)
        XCTAssertEqual(PlaybackSpeed.timeLeft(from: 30, to: 100, at: 0.5), 140)
        XCTAssertEqual(PlaybackSpeed.timeLeft(from: 120, to: 100, at: 1.4), 0, "past the end")
    }
}
