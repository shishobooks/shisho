import XCTest
@testable import Shisho

/// Tag namespaces split tag names as the web does (`app/utils/tagNamespaces.test.ts`)
/// and the server filters by them (`pkg/tags/namespaces.go`).
final class TagNamespacesTests: XCTestCase {
    private let namespaces = ["Circle", "Magazine"]

    func testSplitsATagWithOrWithoutASpaceAfterTheColon() {
        XCTAssertEqual(TagNamespaces.split("Circle: Moon Factory", namespaces), SplitTag(namespace: "Circle", name: "Moon Factory"))
        XCTAssertEqual(TagNamespaces.split("Circle:Apple Lab", namespaces), SplitTag(namespace: "Circle", name: "Apple Lab"))
    }

    func testMatchesTheNamespaceCaseInsensitivelyAndReportsItAsConfigured() {
        XCTAssertEqual(TagNamespaces.split("circle: Neko", namespaces), SplitTag(namespace: "Circle", name: "Neko"))
    }

    func testLeavesOtherTagsPlain() {
        for name in ["Full Color", "Circle:", "Circle:   ", "Circles: Plural", "Star Wars: The Clone Wars"] {
            XCTAssertEqual(TagNamespaces.split(name, namespaces), SplitTag(namespace: nil, name: name), name)
        }
    }

    func testFoldsOnlyASCIILettersAsTheServerDoes() {
        XCTAssertNil(TagNamespaces.split("ÉCOLE: Foo", ["école"]).namespace)
        XCTAssertEqual(TagNamespaces.split("école: Foo", ["école"]).namespace, "école")
        XCTAssertEqual(TagNamespaces.split("ÉCOLE: Foo", ["École"]).namespace, "École")
    }

    func testLeavesEveryTagPlainWithNoNamespaces() {
        XCTAssertEqual(TagNamespaces.split("Circle: Foo", []), SplitTag(namespace: nil, name: "Circle: Foo"))
    }

    func testGroupsABooksTagsIntoTagsThenEachNamespaceInTheLibrarysOrder() {
        let groups = TagNamespaces.group(["Magazine: Comic Bloom", "Full Color", "Circle: Moon Factory", "Romance"],
                                         name: { $0 }, namespaces: namespaces)
        XCTAssertEqual(groups.map(\.namespace), [nil, "Circle", "Magazine"])
        XCTAssertEqual(groups.map { $0.tags.map(\.name) }, [["Full Color", "Romance"], ["Moon Factory"], ["Comic Bloom"]])
    }

    func testANamespaceWithNoTagsOnTheBookHasNoRow() {
        let groups = TagNamespaces.group(["Circle: Moon Factory"], name: { $0 }, namespaces: namespaces)
        XCTAssertEqual(groups.map(\.namespace), ["Circle"])
    }

    func testEveryLibrarysNamespacesOnceInTheirOrder() {
        XCTAssertEqual(TagNamespaces.merged([["Circle", "Source"], ["source", "Magazine"], []]), ["Circle", "Source", "Magazine"])
    }

    func testAScopeHoldsTheTagsInIt() {
        XCTAssertTrue(TagScope.all.contains(namespace: nil))
        XCTAssertTrue(TagScope.all.contains(namespace: "Circle"))
        XCTAssertTrue(TagScope.plain.contains(namespace: nil))
        XCTAssertFalse(TagScope.plain.contains(namespace: "Circle"))
        XCTAssertTrue(TagScope.namespace("Circle").contains(namespace: "Circle"))
        XCTAssertTrue(TagScope.namespace("Circle").contains(namespace: "circle"), "as the server compares namespaces")
        XCTAssertFalse(TagScope.namespace("Circle").contains(namespace: nil))
        XCTAssertFalse(TagScope.namespace("Circle").contains(namespace: "Magazine"))
    }
}
