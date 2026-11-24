import XCTest
@testable import Shisho

/// What the banner offering another device's place says (#912).
final class PlaceOfferTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func place(source: String, device: String? = nil) -> ServerPlace {
        ServerPlace(seconds: 754, occurredAt: now, deviceID: device == nil ? nil : "device-1", source: source,
                    cfi: nil, percentage: 0.54, deviceName: device)
    }

    /// The offer names the Device that's ahead.
    func testItNamesTheDevice() {
        let kobo = place(source: "kobo", device: "Robin's Kobo")
        XCTAssertEqual(PlaceOfferBanner.message(for: kobo, kind: .reading), "You read up to 54% on Robin's Kobo.")
        XCTAssertEqual(PlaceOfferBanner.message(for: kobo, kind: .listening), "You listened up to 12:34 on Robin's Kobo.")
    }

    /// The web reader is no Device.
    func testTheWebIsTheWeb() {
        let web = place(source: "web_reader")
        XCTAssertEqual(PlaceOfferBanner.message(for: web, kind: .reading), "You read up to 54% on the web.")
        XCTAssertEqual(PlaceOfferBanner.message(for: web, kind: .listening), "You listened up to 12:34 on the web.")
    }

    /// A renamed or deleted Device sends its places again: still the same
    /// move, so one dismissed isn't offered again.
    func testARenamedDevicesPlaceIsTheSameMove() {
        let kobo = place(source: "kobo", device: "Robin's Kobo")
        var renamed = kobo
        renamed.deviceName = "Study Kobo"
        XCTAssertTrue(renamed.isSameMove(as: kobo))
        let deleted = ServerPlace(seconds: 754, occurredAt: now, deviceID: nil, source: "kobo", cfi: nil, percentage: 0.54)
        XCTAssertTrue(deleted.isSameMove(as: kobo))
        var moved = kobo
        moved.percentage = 0.6
        XCTAssertFalse(moved.isSameMove(as: kobo))
        XCTAssertFalse(kobo.isSameMove(as: nil))
    }

    /// A Device deleted since, or a name that's only spaces, is another
    /// device.
    func testAPlaceWithNoNameIsAnotherDevice() {
        for name in [nil, "", "  "] {
            let place = place(source: "koreader", device: name)
            XCTAssertEqual(PlaceOfferBanner.message(for: place, kind: .reading), "You read up to 54% on another device.")
            XCTAssertEqual(PlaceOfferBanner.message(for: place, kind: .listening), "You listened up to 12:34 on another device.")
        }
    }
}
