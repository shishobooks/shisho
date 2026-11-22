import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// The whole catalog on the device (#914): Series, Persons, Genres, Tags,
/// Lists, the user's own record of each Book, search, and access changes.
final class CatalogBrowseTests: XCTestCase {
    private var store: CatalogStore!

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func library(_ id: Int, _ name: String) -> String {
        #"{"kind":"library","id":\#(id),"library_id":\#(id),"library":{"id":\#(id),"name":"\#(name)","cover_aspect_ratio":"book"}}"#
    }

    private func book(_ id: Int, _ title: String, library: Int = 1, extra: String = "") -> String {
        """
        {"kind":"book","id":\(id),"library_id":\(library),"book":{"id":\(id),"library_id":\(library),"title":"\(title)","sort_title":"\(title)",
         "created_at":"2026-01-01T00:00:00Z","description":"About \(title).",
         "authors":[{"person_id":1,"name":"Frank Herbert","sort_name":"Herbert, Frank"}],
         "series":[{"series_id":1,"name":"Dune Chronicles","sort_name":"Dune Chronicles","number":\(id)}],
         "genres":[{"genre_id":1,"name":"Science Fiction"}],"tags":[{"tag_id":1,"name":"Desert"}],
         "files":[{"id":\(id * 10),"file_type":"epub","file_role":"main","size_bytes":10,"display_name":"\(title).epub",
           "publisher":{"publisher_id":1,"name":"Chilton"},"identifiers":[{"type":"isbn_13","value":"978044101359\(id)"}],
           "narrators":[{"person_id":2,"name":"Scott Brick","sort_name":"Brick, Scott"}]}],"cover_version":"\(id)-1"\(extra)}}
        """
    }

    private let resources = """
        {"kind":"series","id":1,"library_id":1,"series":{"id":1,"library_id":1,"name":"Dune Chronicles","sort_name":"Dune Chronicles","description":"Arrakis.","aliases":["Dune Saga"]}},
        {"kind":"person","id":1,"library_id":1,"person":{"id":1,"library_id":1,"name":"Frank Herbert","sort_name":"Herbert, Frank","aliases":["F. P. Herbert"]}},
        {"kind":"person","id":2,"library_id":1,"person":{"id":2,"library_id":1,"name":"Scott Brick","sort_name":"Brick, Scott","aliases":[]}},
        {"kind":"genre","id":1,"library_id":1,"genre":{"id":1,"library_id":1,"name":"Science Fiction","aliases":["SF"]}},
        {"kind":"tag","id":1,"library_id":1,"tag":{"id":1,"library_id":1,"name":"Desert","aliases":[]}}
        """

    private func catalog(access: Shishoapp.FeedAccess? = Shishoapp.FeedAccess(libraryIds: [1], listIds: [])) throws {
        try store.apply(page(#"{"entries":[\#(library(1, "Fiction")),\#(book(1, "Dune")),\#(book(2, "Dune Messiah")),\#(resources)],"cursor":9}"#),
                        access: access)
    }

    func testBooksCarryTheirDetails() throws {
        try catalog()
        let detail = try XCTUnwrap(store.pool.read { try CatalogStore.book($0, id: 1) })
        XCTAssertEqual(detail.description, "About Dune.")
        XCTAssertEqual(detail.genres.map(\.name), ["Science Fiction"])
        XCTAssertEqual(detail.tags.map(\.name), ["Desert"])
        XCTAssertEqual(detail.series.first?.seriesID, 1)
        XCTAssertEqual(detail.authors.first?.personID, 1)
        XCTAssertEqual(detail.files.first?.publisher, "Chilton")
        XCTAssertEqual(detail.files.first?.identifiers, [BookDetail.Identifier(type: "isbn_13", value: "9780441013591")])
        XCTAssertEqual(detail.files.first?.narratorIDs, [2])
    }

    func testBrowsesResources() throws {
        try catalog()
        let series = try store.pool.read { try CatalogStore.resources($0, kind: .series, libraryID: nil) }
        XCTAssertEqual(series.map(\.name), ["Dune Chronicles"])
        XCTAssertEqual(series.first?.bookCount, 2)
        XCTAssertEqual(series.first?.previews.map(\.title), ["Dune", "Dune Messiah"], "its first Books, in its order")
        XCTAssertEqual(series.first?.detail, "Dune Saga", "its other names")
        let persons = try store.pool.read { try CatalogStore.resources($0, kind: .person, libraryID: 1) }
        XCTAssertEqual(persons.map(\.name), ["Scott Brick", "Frank Herbert"], "by sort name")
        XCTAssertEqual(persons.map(\.detail), ["Brick, Scott", "Herbert, Frank, F. P. Herbert"])
        XCTAssertEqual(persons.map(\.authoredCount), [0, 2])
        XCTAssertEqual(persons.map(\.narratedCount), [2, 0], "Files narrated")
        XCTAssertEqual(try store.pool.read { try CatalogStore.books($0, kind: .person, id: 2, narrated: true) }.map(\.id), [1, 2])
        XCTAssertEqual(try store.pool.read { try CatalogStore.books($0, kind: .person, id: 2) }.map(\.id), [])
        XCTAssertEqual(try store.pool.read { try CatalogStore.books($0, kind: .series, id: 1) }.map(\.title), ["Dune", "Dune Messiah"])
        let detail = try XCTUnwrap(store.pool.read { try CatalogStore.resource($0, kind: .series, id: 1) })
        XCTAssertEqual(detail.description, "Arrakis.")
        XCTAssertEqual(detail.aliases, ["Dune Saga"])

        // A renamed Series reaches its page.
        try store.apply(page(#"{"entries":[{"kind":"series","id":1,"library_id":1,"series":{"id":1,"library_id":1,"name":"Dune","sort_name":"Dune","aliases":[]}}],"cursor":10}"#))
        XCTAssertEqual(try store.pool.read { try CatalogStore.resource($0, kind: .series, id: 1) }?.name, "Dune")
    }

    /// Search reads the device's own index, built as the server builds its
    /// own, so it finds Books by title, author, alias, narrator, and Series.
    func testSearchesOffline() throws {
        try catalog()
        let search = { (text: String) in try self.store.pool.read { try CatalogStore.search($0, text: text, libraryID: nil) } }
        XCTAssertEqual(try search("Messi").books.map(\.title), ["Dune Messiah"], "a prefix of a title word")
        XCTAssertEqual(Set(try search("herbert").books.map(\.id)), [1, 2], "an author, ignoring case")
        XCTAssertEqual(Set(try search("F. P.").books.map(\.id)), [1, 2], "an author's alias")
        XCTAssertEqual(Set(try search("Scott").books.map(\.id)), [1, 2], "a narrator")
        XCTAssertEqual(try search("Dune Saga").series.map(\.name), ["Dune Chronicles"], "a Series' alias")
        XCTAssertEqual(try search("Frank").persons.map(\.name), ["Frank Herbert"])
        XCTAssertEqual(try search("SF").genres.map(\.name), ["Science Fiction"])
        XCTAssertEqual(try search("978-0441013592").books.map(\.title), ["Dune Messiah"], "an ISBN as typed with hyphens")
        XCTAssertTrue(try search("\"OR NOT").isEmpty, "operators are literal")

        // An edit replaces the old words.
        try store.apply(page(#"{"entries":[\#(book(2, "Children of Dune"))],"cursor":11}"#))
        XCTAssertEqual(try search("Messiah").books, [])
        XCTAssertEqual(try search("Children").books.map(\.id), [2])
        // A deleted Book leaves the index.
        try store.apply(page(#"{"entries":[{"kind":"book","id":2,"deleted":true,"library_id":1}],"cursor":12}"#))
        XCTAssertEqual(try search("Children").books, [])
    }

    func testReadingStatusAndRating() throws {
        try catalog()
        let standing = { try self.store.pool.read { try CatalogStore.standing($0, bookID: 1) } }
        XCTAssertEqual(try standing(), BookStanding(status: nil, rating: nil))
        try store.apply(page(#"{"entries":[{"kind":"user_book","id":1,"user_book":{"book_id":1,"rating":18,"to_read_position":1,"reference":false}}],"cursor":10}"#))
        XCTAssertEqual(try standing(), BookStanding(status: "to_read", rating: 18))
        try store.apply(page(#"{"entries":[{"kind":"read_through","id":5,"read_through":{"id":5,"book_id":1,"outcome":"underway","paused":true}}],"cursor":11}"#))
        XCTAssertEqual(try standing().status, "paused", "a Read-through underway comes first")
        try store.apply(page(#"{"entries":[{"kind":"read_through","id":5,"read_through":{"id":5,"book_id":1,"outcome":"finished","paused":false,"end_day":"2026-02-01"}},{"kind":"user_book","id":1,"deleted":true}],"cursor":12}"#))
        XCTAssertEqual(try standing(), BookStanding(status: "read", rating: nil))
    }

    func testListsInTheirOrder() throws {
        try catalog(access: Shishoapp.FeedAccess(libraryIds: [1], listIds: [7]))
        let list = """
        {"kind":"list","id":7,"list":{"id":7,"name":"Favorites","is_ordered":true,"default_sort":"manual","owner":{"id":3,"username":"ann"},
         "permission":"viewer","books":[{"book_id":2,"sort_order":1,"added_at":"2026-01-02T00:00:00Z"},{"book_id":1,"sort_order":2,"added_at":"2026-01-01T00:00:00Z"},
         {"book_id":99,"sort_order":3,"added_at":"2026-01-01T00:00:00Z"}]}}
        """
        try store.apply(page(#"{"entries":[\#(list)],"cursor":10}"#))
        let lists = try store.pool.read(CatalogStore.lists)
        XCTAssertEqual(lists.map(\.name), ["Favorites"])
        XCTAssertEqual(lists.first?.bookCount, 2, "a Book the device doesn't have isn't counted")
        XCTAssertEqual(lists.first?.ownerName, "ann")
        XCTAssertEqual(try store.pool.read { try CatalogStore.listBooks($0, listID: 7) }.map(\.id), [2, 1])

        try store.apply(page(#"{"entries":[{"kind":"list","id":7,"deleted":true}],"cursor":11}"#))
        XCTAssertEqual(try store.pool.read(CatalogStore.lists), [])
    }

    /// Access is answered outside the cursor (ADR 0023): a Library lost goes
    /// with everything in it, and one gained waits to be fetched whole.
    func testAccessChanges() throws {
        try catalog(access: Shishoapp.FeedAccess(libraryIds: [1, 2], listIds: [7]))
        try store.apply(page(#"{"entries":[\#(library(2, "Other")),\#(book(3, "Emma", library: 2))],"cursor":10}"#))
        XCTAssertEqual(try store.pendingAccess().libraries.count, 0, "a download from the start brings everything")

        try store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [2, 4], listIds: []))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [3])
        XCTAssertEqual(try store.pool.read { try CatalogStore.resources($0, kind: .series, libraryID: nil) }, [])
        XCTAssertEqual(try store.pool.read { try CatalogStore.search($0, text: "Messiah", libraryID: nil) }.books, [])
        let pending = try store.pendingAccess()
        XCTAssertEqual(pending.libraries.map(\.id), [4])

        try store.applyScoped(page(#"{"entries":[\#(library(4, "Gained")),\#(book(5, "Persuasion", library: 4))],"cursor":30,"has_more":false}"#), libraryID: 4)
        XCTAssertEqual(try store.pendingAccess().libraries.count, 0)
        XCTAssertEqual(Set(try store.books(libraryID: nil).map(\.id)), [3, 5])
        XCTAssertEqual(try store.syncState().cursor, 10, "the feed's cursor stays where it was")
    }

    /// A Book moved into a Library the user can't access leaves; one that
    /// arrived in a Library they can stays.
    func testABookThatLeftItsLibrary() throws {
        try catalog()
        try store.apply(page(#"{"entries":[{"kind":"book_left","id":1,"deleted":true,"library_id":1}],"cursor":10}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [2])
        try store.apply(page(#"{"entries":[\#(library(3, "New")),\#(book(2, "Dune Messiah", library: 3)),{"kind":"book_left","id":2,"deleted":true,"library_id":1}],"cursor":11}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [2])
    }

    /// A server from before access was sent says nothing about it, which
    /// must never read as access to nothing.
    func testAPageWithoutAccessSaysNothing() throws {
        XCTAssertNil(APIClient.feedAccess(Data(#"{"entries":[],"cursor":0,"has_more":false,"start_over":false}"#.utf8)))
        XCTAssertEqual(APIClient.feedAccess(Data(#"{"access":{"library_ids":[1],"list_ids":[]}}"#.utf8))?.libraryIds, [1])
    }

    func testSortsByTheReadersOwnData() throws {
        try catalog()
        try store.apply(page(#"{"entries":[{"kind":"user_book","id":2,"user_book":{"book_id":2,"rating":20,"reference":false}},{"kind":"user_book","id":1,"user_book":{"book_id":1,"rating":4,"reference":false}}],"cursor":10}"#))
        let sorted = try store.pool.read { db in
            try Row.fetchAll(db, sql: "SELECT b.id FROM books b ORDER BY \(LibrarySort.orderBy("rating:desc"))").map { $0["id"] as Int }
        }
        XCTAssertEqual(sorted, [2, 1])
    }

    /// A Library gained while a download from the start runs has records
    /// behind its cursor, so it's fetched whole once the download ends.
    func testAccessGainedDuringADownloadIsFetched() throws {
        let start = Shishoapp.FeedAccess(libraryIds: [1], listIds: [])
        let later = Shishoapp.FeedAccess(libraryIds: [1, 2], listIds: [])
        try store.apply(page(#"{"entries":[\#(library(1, "Fiction"))],"cursor":5,"has_more":true}"#), after: 0, access: start)
        try store.reconcileAccess(start)
        try store.apply(page(#"{"entries":[\#(book(1, "Dune"))],"cursor":9}"#), after: 5, access: later)
        try store.reconcileAccess(later)
        XCTAssertEqual(try store.pendingAccess().libraries.map(\.id), [2])
    }

    /// A role that can't read Books anymore leaves nothing on the device.
    func testLosingEveryLibraryDropsTheCatalog() throws {
        try catalog()
        try store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [], listIds: []), force: true)
        XCTAssertEqual(try store.books(libraryID: nil), [])
        XCTAssertEqual(try store.libraries(), [])
    }

    /// A List holds only the Books the user can access, so a Library gained
    /// or lost fetches every List again.
    func testALibraryAccessChangeFetchesListsAgain() throws {
        try catalog(access: Shishoapp.FeedAccess(libraryIds: [1], listIds: [7]))
        XCTAssertEqual(try store.pendingAccess().lists, [])
        try store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [1, 2], listIds: [7]))
        XCTAssertEqual(try store.pendingAccess().lists, [7])
    }

    /// A Book that leaves a Series leaves the Series' search row too.
    func testSeriesSearchForgetsABookThatLeft() throws {
        try catalog()
        let alone = """
        {"kind":"book","id":2,"library_id":1,"book":{"id":2,"library_id":1,"title":"Dune Messiah","sort_title":"Dune Messiah",
         "created_at":"2026-01-01T00:00:00Z","authors":[],"series":[],"files":[],"cover_version":""}}
        """
        try store.apply(page(#"{"entries":[\#(alone)],"cursor":10}"#))
        XCTAssertEqual(try store.pool.read { try CatalogStore.search($0, text: "Messiah", libraryID: nil) }.series, [])
    }

    /// A Library granted and renamed at once arrives in the feed before its
    /// Books, which are behind the cursor: it still counts as gained.
    func testALibraryThatArrivedIsStillGained() throws {
        try catalog()
        try store.apply(page(#"{"entries":[\#(library(2, "Renamed"))],"cursor":10}"#))
        try store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [1, 2], listIds: []))
        XCTAssertEqual(try store.pendingAccess().libraries.map(\.id), [2])
    }

    func testAnISBNWithItsPrefixFindsTheBook() throws {
        try catalog()
        XCTAssertEqual(try store.pool.read { try CatalogStore.search($0, text: "ISBN: 978 0441013592", libraryID: nil) }.books.map(\.id), [2])
    }

    /// A Book that left its Library leaves its Series' search row.
    func testABookThatLeftLeavesItsSeriesSearch() throws {
        try catalog()
        try store.apply(page(#"{"entries":[{"kind":"book_left","id":2,"deleted":true,"library_id":1}],"cursor":10}"#))
        XCTAssertEqual(try store.pool.read { try CatalogStore.search($0, text: "Messiah", libraryID: nil) }.series, [])
    }

    /// A Library gained that has nothing behind the feed's cursor, such as
    /// one just made, arrives in the feed, so it isn't fetched whole too.
    func testALibraryNotBehindTheCursorIsntFetchedAgain() throws {
        try catalog()
        try store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [1, 2, 3], listIds: [], behindLibraryIds: [1, 3]), fromFeed: true)
        XCTAssertEqual(try store.pendingAccess().libraries.map(\.id), [3])
    }

    /// The grid names each creator once, and for a comic only its writers
    /// and pencillers, as the web gallery does.
    func testTheGridNamesCreatorsAsTheWebDoes() throws {
        let comic = """
        {"kind":"book","id":7,"library_id":1,"book":{"id":7,"library_id":1,"title":"Saga","sort_title":"Saga","created_at":"2026-01-01T00:00:00Z",
         "authors":[{"person_id":1,"name":"Fujita","sort_name":"Fujita","role":"writer"},{"person_id":1,"name":"Fujita","sort_name":"Fujita","role":"penciller"},
           {"person_id":2,"name":"AndWorld Design","sort_name":"AndWorld Design","role":"letterer"},{"person_id":3,"name":"Phil","sort_name":"Phil","role":"cover_artist"}],
         "files":[{"id":70,"file_type":"cbz","file_role":"main","size_bytes":10}],"cover_version":""}}
        """
        try store.apply(page(#"{"entries":[\#(library(1, "Comics")),\#(comic)],"cursor":2}"#))
        XCTAssertEqual(try store.books(libraryID: nil).first?.authors, "Fujita")
    }

    func testFiltersAndReadingOnSummaries() throws {
        try catalog()
        try store.apply(page(#"{"entries":[{"kind":"user_book","id":1,"user_book":{"book_id":1,"rating":16,"to_read_position":1,"reference":false}},{"kind":"read_through","id":5,"read_through":{"id":5,"book_id":2,"outcome":"underway","paused":false}},{"kind":"reading_position","id":20,"reading_position":{"file_id":20,"percentage":0.4,"occurred_at":"2026-01-01T00:00:00Z","source":"web_reader"}}],"cursor":10}"#))
        let all = try store.books(libraryID: nil)
        XCTAssertEqual(all.first { $0.id == 1 }?.status, "to_read")
        XCTAssertEqual(all.first { $0.id == 1 }?.rating, 16)
        XCTAssertEqual(all.first { $0.id == 2 }?.status, "currently_reading")
        XCTAssertEqual(all.first { $0.id == 2 }?.progress, 0.4)
        let filtered = { (filter: LibraryFilter) in try self.store.pool.read { try CatalogStore.books($0, libraryID: nil, filter: filter) }.map(\.id) }
        XCTAssertEqual(try filtered(LibraryFilter(statuses: ["to_read"])), [1])
        XCTAssertEqual(try filtered(LibraryFilter(minRating: 12)), [1])
        XCTAssertEqual(try filtered(LibraryFilter(maxRating: 12)), [], "a range leaves out unrated Books")
        XCTAssertEqual(try filtered(LibraryFilter(minRating: 16, maxRating: 16)), [1])
        XCTAssertEqual(try filtered(LibraryFilter(genres: ["science fiction"])).sorted(), [1, 2], "by name, ignoring case")
        XCTAssertEqual(try filtered(LibraryFilter(fileTypes: ["m4b"])), [])
        let sorted = try store.pool.read { try CatalogStore.books($0, libraryID: nil, sort: "title:desc") }.map(\.title)
        XCTAssertEqual(sorted, ["Dune Messiah", "Dune"])
    }

    func testTheSortSheetWorksOnLevelsAsTheWebDoes() {
        XCTAssertEqual(LibrarySort.levels("title:asc,rating:desc,title:desc,bogus:asc,author"),
                       [SortLevel(field: "title", descending: false), SortLevel(field: "rating", descending: true)])
        XCTAssertEqual(LibrarySort.spec([SortLevel(field: "series", descending: false), SortLevel(field: "rating", descending: true)]),
                       "series:asc,rating:desc")

        let saved = GallerySort(saved: "title:asc", device: nil)
        XCTAssertEqual(saved.levels, [SortLevel(field: "title", descending: false)])
        XCTAssertFalse(saved.isDirty)
        let builtIn = GallerySort(saved: "", device: nil)
        XCTAssertEqual(builtIn.levels, LibrarySort.builtInDefault)

        let picked = GallerySort(saved: "title:asc", device: "rating:desc")
        XCTAssertEqual(picked.levels, [SortLevel(field: "rating", descending: true)])
        XCTAssertTrue(picked.isDirty)
        XCTAssertEqual(picked.deviceSpec(for: [SortLevel(field: "author", descending: false)]), "author:asc")
        XCTAssertNil(picked.deviceSpec(for: [SortLevel(field: "title", descending: false)]), "the saved sort isn't kept as a pick")
        XCTAssertNil(picked.deviceSpec(for: []), "no levels goes back to the saved sort")

        XCTAssertTrue(LibrarySort.initiallyDescending("rating"))
        XCTAssertFalse(LibrarySort.initiallyDescending("title"))
    }

    func testRatingRangeLabelsMatchTheWeb() {
        XCTAssertEqual(Ratings.range(min: nil, max: nil), "Any rating")
        XCTAssertEqual(Ratings.range(min: 14, max: nil), "3.5+ stars")
        XCTAssertEqual(Ratings.range(min: nil, max: 8), "Up to 2 stars")
        XCTAssertEqual(Ratings.range(min: 4, max: 13), "1 to 3.25 stars")
        XCTAssertEqual(Ratings.range(min: 4, max: 4), "1 star")
        XCTAssertEqual(Ratings.range(min: 16, max: 16), "4 stars")
    }

    /// A Book whose Files are given as JSON, for the File page's tests.
    private func book(_ id: Int, files: String) -> String {
        """
        {"kind":"book","id":\(id),"library_id":1,"book":{"id":\(id),"library_id":1,"title":"Book \(id)","sort_title":"Book \(id)",
         "created_at":"2026-01-01T00:00:00Z","authors":[],"series":[],"genres":[],"tags":[],"files":[\(files)],"cover_version":""}}
        """
    }

    func testAFilesChaptersNestAsItsTableOfContents() throws {
        let file = """
            {"id":30,"file_type":"epub","file_role":"main","size_bytes":10,"display_name":"Book.epub","narrators":[],"identifiers":[],
             "chapters":[{"id":1,"title":"Part One"},{"id":2,"parent_id":1,"title":"Prologue","href":"p.xhtml"},
               {"id":3,"parent_id":2,"title":"Deeper"},{"id":4,"title":"Part Two","start_page":4,"start_timestamp_ms":61000}]}
            """
        try store.apply(page(#"{"entries":[\#(library(1, "Fiction")),\#(book(3, files: file))],"cursor":9}"#),
                        access: Shishoapp.FeedAccess(libraryIds: [1], listIds: []))
        let chapters = try store.pool.read { try CatalogStore.fileChapters($0, fileID: 30) }
        XCTAssertEqual(chapters.map(\.title), ["Part One", "Prologue", "Deeper", "Part Two"])
        XCTAssertEqual(chapters.map(\.depth), [0, 1, 2, 0])
        XCTAssertEqual(chapters.last?.startPage, 4)
        XCTAssertEqual(chapters.last?.startSeconds, 61)
    }

    func testListenProgressForOneAudiobookOfSeveral() throws {
        let audiobook = { (id: Int, name: String) in
            """
            {"id":\(id),"file_type":"m4b","file_role":"main","size_bytes":10,"display_name":"\(name)","narrators":[],"identifiers":[],
             "audiobook_duration_seconds":600,"chapters":[{"id":\(id),"title":"One","start_timestamp_ms":0}]}
            """
        }
        try store.apply(page(#"{"entries":[\#(library(1, "Fiction")),\#(book(4, files: audiobook(40, "Unabridged") + "," + audiobook(41, "Abridged")))],"cursor":9}"#),
                        access: Shishoapp.FeedAccess(libraryIds: [1], listIds: []))
        let book = try XCTUnwrap(store.pool.read { try CatalogStore.listenProgress($0, bookID: 4, ownDeviceID: "d") })
        XCTAssertEqual(book.fileID, 40, "the Book's first audiobook, with nothing listened to")
        XCTAssertEqual(book.audiobookCount, 2)
        let file = try XCTUnwrap(store.pool.read { try CatalogStore.listenProgress($0, bookID: 4, fileID: 41, ownDeviceID: "d") })
        XCTAssertEqual(file.fileID, 41)
        XCTAssertEqual(file.audiobookCount, 1, "a File's own page doesn't say which File it is")
    }
}
