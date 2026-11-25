import CarPlay
import GRDB
import UIKit

/// CarPlay's scene, which iOS connects when the phone joins a car, whether
/// or not the app's screen is running: CarPlay can launch the app on its
/// own. Everything it shows comes from the account's database, and
/// everything it plays goes through the phone's own `AudioPlayer`, so
/// listening in the car is tracked as it is anywhere else.
final class CarPlaySceneDelegate: UIResponder, CPTemplateApplicationSceneDelegate {
    private var screens: CarPlayScreens?

    func templateApplicationScene(_ scene: CPTemplateApplicationScene, didConnect interfaceController: CPInterfaceController) {
        let screens = CarPlayScreens(model: AppModel.shared, interfaceController: interfaceController)
        self.screens = screens
        screens.start()
    }

    func templateApplicationScene(_ scene: CPTemplateApplicationScene,
                                  didDisconnectInterfaceController interfaceController: CPInterfaceController) {
        screens?.stop()
        screens = nil
    }
}

/// A list CarPlay shows, kept up to date from the account's database while
/// it shows, by one observation, so updates arrive in order.
@MainActor
private final class LiveList {
    let template: CPListTemplate
    /// Names the list in the log, never by a title.
    let name: String
    /// What the list says when it has nothing, once its first read is in.
    let empty: String
    let fetch: @Sendable (Database) throws -> [CarPlayRow]
    /// What it last read, to draw again when the car's limits change.
    var rows: [CarPlayRow] = []
    var updates: Task<Void, Never>?
    /// Each audiobook row's item by File, reused while its audiobook is
    /// unchanged, so a redraw neither rebuilds it nor loads its cover again.
    var items: [Int: (book: CarPlayAudiobook, item: CPListItem)] = [:]

    init(_ template: CPListTemplate, name: String, empty: String, fetch: @escaping @Sendable (Database) throws -> [CarPlayRow]) {
        self.template = template
        self.name = name
        self.empty = empty
        self.fetch = fetch
    }
}

/// CarPlay's templates: tabs for Recent (the audiobooks partway through),
/// Downloads, and Library, beside the player CarPlay draws itself, and the
/// shared Now Playing template with chapter skips beside the system's skips,
/// which use the listener's lengths (`AudioPlayer.applySkipIntervals`).
@MainActor
final class CarPlayScreens: NSObject, CPInterfaceControllerDelegate, CPSessionConfigurationDelegate {
    /// The most audiobooks Recent reads; each list shows as many as the car
    /// allows, which can change while driving.
    static let recentLimit = 100

    private let model: AppModel
    private let interfaceController: CPInterfaceController
    private let tabs: CPTabBarTemplate
    private let recent: CPListTemplate
    private let downloads: CPListTemplate
    private let library: CPListTemplate
    /// What shows while no account is signed in.
    private let signedOut: CPListTemplate
    /// The account the lists show, which every row checks is still the one
    /// signed in before it acts.
    private var session: AccountSession?
    /// The first account, or no account, has been shown.
    private var showedSession = false
    /// The tabs' lists by name, and a Library's list pushed over them.
    private var tabLists: [String: LiveList] = [:]
    private var pushed: LiveList?
    /// What's playing, as the lists last showed it.
    private var playing: (fileID: Int?, isPlaying: Bool) = (nil, false)
    /// The player's last error, said once.
    private var lastError: String?
    /// Says when the car limits its lists, as some do while moving.
    private var carSession: CPSessionConfiguration?

    private var allLists: [LiveList] { Array(tabLists.values) + [pushed].compactMap { $0 } }

    init(model: AppModel, interfaceController: CPInterfaceController) {
        self.model = model
        self.interfaceController = interfaceController
        recent = CarPlayScreens.tab(String(localized: "Recent"), icon: .clockArrowUp)
        downloads = CarPlayScreens.tab(String(localized: "Downloads"), icon: .download)
        library = CarPlayScreens.tab(String(localized: "Library"), icon: .library)
        tabs = CPTabBarTemplate(templates: [recent, downloads, library])
        signedOut = CPListTemplate(title: String(localized: "Shisho"), sections: [])
        signedOut.emptyViewTitleVariants = [String(localized: "Not Signed In")]
        signedOut.emptyViewSubtitleVariants = [String(localized: "Shisho lists your audiobooks here once it's signed in to your server.")]
        super.init()
    }

    private static func tab(_ title: String, icon: Lucide) -> CPListTemplate {
        let list = CPListTemplate(title: title, sections: [])
        list.tabTitle = title
        list.tabImage = icon.uiImage
        return list
    }

    func start() {
        interfaceController.delegate = self
        carSession = CPSessionConfiguration(delegate: self)
        configureNowPlaying()
        playing = (model.player?.file?.fileID, model.player?.isPlaying == true)
        watchSession()
        watchPlayer()
        watchSort()
        log("connected")
        model.carPlayConnected()
    }

    func stop() {
        for list in allLists { list.updates?.cancel() }
        log("disconnected")
        model.carPlayDisconnected()
    }

    /// Logs a template change that failed: one with no completion raises an
    /// exception instead.
    private func completion(_ change: String) -> (Bool, (any Error)?) -> Void {
        { [weak self] _, error in
            guard let error else { return }
            Task { @MainActor in self?.log("couldn't \(change): \(Diagnostics.describe(error))") }
        }
    }

    /// Makes root the root, or, when it already is, goes back to it, so
    /// nothing pushed for the last account stays.
    private func show(root: CPTemplate, _ change: String) {
        if interfaceController.rootTemplate === root {
            interfaceController.popToRootTemplate(animated: false, completion: completion(change))
        } else {
            interfaceController.setRootTemplate(root, animated: false, completion: completion(change))
        }
    }

    // MARK: - Watching

    /// Shows the signed-in account's tabs, again whenever the account
    /// changes, leaving nothing of the last one's to play.
    private func watchSession() {
        let session = withObservationTracking {
            model.session
        } onChange: { [weak self] in
            Task { @MainActor in self?.watchSession() }
        }
        guard !showedSession || session !== self.session else { return }
        showedSession = true
        self.session = session
        for list in allLists { list.updates?.cancel() }
        tabLists = [:]
        pushed = nil
        guard let session else {
            show(root: signedOut, "show the signed-out list")
            return
        }
        let deviceID = session.account.deviceID
        let account = AccountData(accountID: session.account.id)
        let limit = CarPlayScreens.recentLimit
        let lists = [
            LiveList(recent, name: "recent", empty: String(localized: "Nothing Partway Through")) { db in
                try CatalogStore.continueListening(db, limit: limit, ownDeviceID: deviceID).map(CarPlayRow.audiobook)
            },
            LiveList(downloads, name: "downloads", empty: String(localized: "No Audiobooks Downloaded")) { db in
                try CatalogStore.downloadedAudiobooks(db).map(CarPlayRow.audiobook)
            },
            LiveList(library, name: "library", empty: String(localized: "No Audiobooks")) { db in
                try CatalogStore.carPlayLibraryTab(db) { account.pickedSort(libraryID: $0) }
            },
        ]
        for list in lists {
            tabLists[list.name] = list
            // The last account's empty text waits for this one's first read.
            list.template.emptyViewTitleVariants = []
            draw(list)
            watch(list)
        }
        show(root: tabs, "show the tabs")
    }

    /// Keeps list up to date with the account's database, reading it again
    /// from the start.
    private func watch(_ list: LiveList) {
        guard let session else { return }
        list.updates?.cancel()
        let name = list.name
        let observation = ValueObservation.tracking(regions: CatalogStore.carPlayTables, fetch: list.fetch).removeDuplicates()
        list.updates = Task { [weak self, weak list] in
            do {
                for try await rows in observation.values(in: session.store.pool) {
                    guard let self, let list, self.session === session else { return }
                    list.rows = rows
                    list.template.emptyViewTitleVariants = [list.empty]
                    self.draw(list)
                }
            } catch {
                self?.log("couldn't read the \(name) list: \(Diagnostics.describe(error))")
            }
        }
    }

    /// Marks the playing audiobook in every list when what's playing
    /// changes, and reads Recent again when playback stops or moves to
    /// another File, whose last place its observation doesn't track. A stream that fails is said in the car.
    private func watchPlayer() {
        withObservationTracking {
            _ = model.player?.file?.fileID
            _ = model.player?.isPlaying
            _ = model.player?.error
        } onChange: { [weak self] in
            Task { @MainActor in
                guard let self else { return }
                let now = (fileID: self.model.player?.file?.fileID, isPlaying: self.model.player?.isPlaying == true)
                if now.fileID != self.playing.fileID || now.isPlaying != self.playing.isPlaying {
                    // Playback stopped, or moved on to another File.
                    let left = self.playing.isPlaying && (!now.isPlaying || now.fileID != self.playing.fileID)
                    self.playing = now
                    self.markPlaying()
                    if left, let recent = self.tabLists["recent"] { self.watch(recent) }
                }
                let error = self.model.player?.error
                if let error, error != self.lastError { self.alert(error) }
                self.lastError = error
                self.watchPlayer()
            }
        }
    }

    private func markPlaying() {
        for list in allLists {
            for (fileID, entry) in list.items {
                entry.item.isPlaying = fileID == playing.fileID && playing.isPlaying
            }
        }
    }

    /// Reads the Library tab and a Library pushed over the tabs again when
    /// the order picked on the phone changes.
    private func watchSort() {
        withObservationTracking {
            _ = model.sortSpec
            _ = model.libraryID
        } onChange: { [weak self] in
            Task { @MainActor in
                guard let self else { return }
                if let library = self.tabLists["library"] { self.watch(library) }
                if let pushed = self.pushed { self.watch(pushed) }
                self.watchSort()
            }
        }
    }

    /// The car changed what it limits: the lists fit its rows again.
    nonisolated func sessionConfiguration(_ sessionConfiguration: CPSessionConfiguration,
                                          limitedUserInterfacesChanged limitedUserInterfaces: CPLimitableUserInterface) {
        Task { @MainActor in
            for list in self.allLists { self.draw(list) }
        }
    }

    // MARK: - Lists

    /// Draws list's rows, as many as the car allows.
    private func draw(_ list: LiveList) {
        var items: [Int: (book: CarPlayAudiobook, item: CPListItem)] = [:]
        let shown = list.rows.prefix(CPListTemplate.maximumItemCount).map { row -> CPListItem in
            switch row {
            case .audiobook(let book):
                let entry: (book: CarPlayAudiobook, item: CPListItem) =
                    list.items[book.fileID].flatMap { $0.book == book ? $0 : nil } ?? (book, audiobookItem(book))
                items[book.fileID] = entry
                return entry.item
            case .library(let library):
                return libraryItem(library)
            }
        }
        list.items = items
        list.template.updateSections(shown.isEmpty ? [] : [CPListSection(items: shown)])
    }

    private func libraryItem(_ library: LibrarySummary) -> CPListItem {
        let item = CPListItem(text: library.name, detailText: nil, image: Lucide.library.uiImage, accessoryImage: nil,
                              accessoryType: .disclosureIndicator)
        let listed = session
        item.handler = { [weak self] _, done in
            if let self, let listed, self.model.session === listed { self.showLibrary(library, session: listed) }
            done()
        }
        return item
    }

    /// A Library's audiobooks, pushed over the tabs, kept up to date while
    /// they show.
    private func showLibrary(_ library: LibrarySummary, session: AccountSession) {
        let id = library.id
        let account = AccountData(accountID: session.account.id)
        let template = CPListTemplate(title: library.name, sections: [])
        let list = LiveList(template, name: "library \(id)", empty: String(localized: "No Audiobooks")) { db in
            try CatalogStore.audiobooks(db, libraryID: id, sort: account.pickedSort(libraryID: id)).map(CarPlayRow.audiobook)
        }
        pushed?.updates?.cancel()
        pushed = list
        watch(list)
        interfaceController.pushTemplate(template, animated: true, completion: completion("show library \(id)"))
    }

    nonisolated func templateDidDisappear(_ aTemplate: CPTemplate, animated: Bool) {
        let gone = ObjectIdentifier(aTemplate)
        Task { @MainActor in
            // Popped: the list stops updating.
            guard let pushed = self.pushed, ObjectIdentifier(pushed.template) == gone,
                  !self.interfaceController.templates.contains(where: { $0 === pushed.template })
            else { return }
            pushed.updates?.cancel()
            self.pushed = nil
        }
    }

    /// A row for an audiobook: its own cover, its Book's title and authors,
    /// its label when it needs one to be told apart, how far through it the
    /// user is, and whether it's playing. Selecting it plays it.
    private func audiobookItem(_ book: CarPlayAudiobook) -> CPListItem {
        let detail = [book.authors.isEmpty ? nil : book.authors, book.label].compactMap { $0 }.joined(separator: " · ")
        let item = CPListItem(text: book.title, detailText: detail.isEmpty ? nil : detail, image: Lucide.headphones.uiImage)
        item.playbackProgress = CGFloat(book.progress ?? 0)
        item.isPlaying = playing.fileID == book.fileID && playing.isPlaying
        let listed = session
        item.handler = { [weak self] _, done in
            self?.play(book, listedFor: listed)
            done()
        }
        setCover(of: book, on: item)
        return item
    }

    /// The audiobook's own cover; one without keeps the headphones.
    private func setCover(of book: CarPlayAudiobook, on item: CPListItem) {
        guard let covers = session?.covers, !book.coverVersion.isEmpty else { return }
        let size = CoverCache.pixelSize(forWidth: CPListItem.maximumImageSize.height,
                                        scale: interfaceController.carTraitCollection.displayScale)
        if let image = covers.memoryImage(.file(book.fileID), version: book.coverVersion, size: size) {
            item.setImage(image)
            return
        }
        Task {
            if let image = await covers.image(.file(book.fileID), version: book.coverVersion, size: size) {
                item.setImage(image)
            }
        }
    }

    // MARK: - Playing

    /// Plays book, listed for the account listed: a row left from an
    /// account since switched from, or one being signed out, plays nothing.
    private func play(_ book: CarPlayAudiobook, listedFor listed: AccountSession?) {
        guard let listed, model.session === listed, !model.isSigningOut, let player = model.player else { return }
        guard model.canOpen(fileID: book.fileID) else {
            // The server may be back; the next try finds out.
            model.checkConnection()
            alert(ConnectionRules.audiobook(model.connection))
            return
        }
        player.open(fileID: book.fileID, autoplay: true)
        // A File gone since it was listed, as by a sync, opens nothing.
        guard player.file?.fileID == book.fileID else { return }
        showNowPlaying()
    }

    private func showNowPlaying() {
        guard interfaceController.topTemplate !== CPNowPlayingTemplate.shared else { return }
        interfaceController.pushTemplate(CPNowPlayingTemplate.shared, animated: true, completion: completion("show Now Playing"))
    }

    /// Now Playing's own buttons: the previous and next chapter. Play,
    /// pause, and the skips come from the remote commands the player
    /// handles.
    private func configureNowPlaying() {
        let previous = CPNowPlayingImageButton(image: Lucide.skipBack.uiImage ?? UIImage()) { [weak self] _ in
            Task { @MainActor in self?.model.player?.previousChapter() }
        }
        previous.accessibilityLabel = String(localized: "Previous Chapter")
        let next = CPNowPlayingImageButton(image: Lucide.skipForward.uiImage ?? UIImage()) { [weak self] _ in
            Task { @MainActor in self?.model.player?.nextChapter() }
        }
        next.accessibilityLabel = String(localized: "Next Chapter")
        let nowPlaying = CPNowPlayingTemplate.shared
        nowPlaying.isUpNextButtonEnabled = false
        nowPlaying.isAlbumArtistButtonEnabled = false
        nowPlaying.updateNowPlayingButtons([previous, next])
    }

    /// Says message, unless something is already showing over the lists.
    private func alert(_ message: String) {
        guard interfaceController.presentedTemplate == nil else { return }
        let ok = CPAlertAction(title: String(localized: "OK"), style: .cancel) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                self.interfaceController.dismissTemplate(animated: true, completion: self.completion("close the alert"))
            }
        }
        interfaceController.presentTemplate(CPAlertTemplate(titleVariants: [message], actions: [ok]), animated: true,
                                            completion: completion("show an alert"))
    }

    private func log(_ message: String) {
        Task { await Diagnostics.shared.log(.carPlay, message) }
    }
}
