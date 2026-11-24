import XCTest
@testable import Shisho

/// Each tab's pages, and its search closing whenever its stack changes: on
/// iOS 26 a stack with a page over an active search ignores being emptied.
final class TabNavigationTests: XCTestCase {
    func testPushingAPageClosesThatTabsSearch() {
        var navigation = TabNavigation()
        navigation.push(.search, on: .library)
        navigation.searchActive = [.library, .lists]

        navigation.push(.browse(.person), on: .library)

        XCTAssertEqual(navigation.path(.library), [.search, .browse(.person)])
        XCTAssertEqual(navigation.searchActive, [.lists])
    }

    func testGoingBackToTheFirstScreenClosesTheSearchAndEmptiesThePath() {
        var navigation = TabNavigation()
        navigation.push(.search, on: .library)
        navigation.push(.browse(.person), on: .library)
        navigation.push(.search, on: .lists)
        navigation.searchActive = [.library, .lists]

        navigation.popToRoot(.library)

        XCTAssertEqual(navigation.path(.library), [])
        XCTAssertEqual(navigation.path(.lists), [.search])
        XCTAssertEqual(navigation.searchActive, [.lists])
    }

    func testGoingBackOnePageClosesTheSearch() {
        var navigation = TabNavigation()
        navigation.push(.search, on: .downloads)
        navigation.push(.browse(.series), on: .downloads)
        navigation.searchActive = [.downloads]

        navigation.popLast(.downloads)

        XCTAssertEqual(navigation.path(.downloads), [.search])
        XCTAssertEqual(navigation.searchActive, [])
    }

    /// The stack's own Back sets a shorter path, which closes the search as
    /// popLast does, so the next screen with one doesn't open it active.
    func testGoingBackThroughTheStackClosesTheSearch() {
        var navigation = TabNavigation()
        navigation.push(.search, on: .library)
        navigation.searchActive = [.library]

        navigation.setPath([], on: .library)

        XCTAssertEqual(navigation.path(.library), [])
        XCTAssertEqual(navigation.searchActive, [])
    }
}
