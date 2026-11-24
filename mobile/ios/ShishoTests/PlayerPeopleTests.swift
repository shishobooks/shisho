import XCTest
@testable import Shisho

/// How many authors and narrators the player names before "+N more".
final class PlayerPeopleTests: XCTestCase {
    func testTwoOfEachThenHowManyMore() {
        XCTAssertEqual(PlayerPeople.cut(authors: 1, narrators: 6, all: false),
                       PlayerPeople.Cut(limit: 2, hidden: 4, collapsible: false))
        XCTAssertEqual(PlayerPeople.cut(authors: 3, narrators: 3, all: false).hidden, 2, "the authors' and narrators' together")
        XCTAssertEqual(PlayerPeople.cut(authors: 2, narrators: 2, all: false).hidden, 0)
    }

    func testEveryoneOnceShownCanBeCutAgain() {
        XCTAssertEqual(PlayerPeople.cut(authors: 1, narrators: 6, all: true),
                       PlayerPeople.Cut(limit: .max, hidden: 0, collapsible: true))
        XCTAssertFalse(PlayerPeople.cut(authors: 2, narrators: 2, all: true).collapsible, "nothing to cut")
    }
}
