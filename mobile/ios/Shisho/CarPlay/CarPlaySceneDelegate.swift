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

/// CarPlay's templates: a list of Continue Listening, Downloads, and the
/// Libraries with audiobooks, and the shared Now Playing template with
/// chapter skips beside the system's skips, which use the listener's
/// lengths (`AudioPlayer.applySkipIntervals`).
@MainActor
final class CarPlayScreens: NSObject, CPInterfaceControllerDelegate, CPSessionConfigurationDelegate {
    private let model: AppModel
    private let interfaceController: CPInterfaceController
    private let root: CPListTemplate
    /// The account the lists show, which every row checks is still the one
    /// signed in before it plays.
    private var session: AccountSession?
    /// Keeps the root list up to date with the account's database.
    private var rootUpdates: Task<Void, Never>?
    /// What the root list last read, to draw again when the player changes.
    private var lastRoot: RootContent?
    /// The list pushed over the root, kept up to date by listUpdates.
    private weak var pushed: CPListTemplate?
    private var listUpdates: Task<Void, Never>?
    /// What the pushed list last read, to draw again when the player
    /// changes.
    private var lastPushed: [CarPlayAudiobook] = []
    /// The player's last error, said once.
    private var lastError: String?
    /// Says when the car limits its lists, as some do while moving.
    private var carSession: CPSessionConfiguration?

    private struct RootContent: Equatable, Sendable {
        var continueListening: [CarPlayAudiobook]
        var libraries: [LibrarySummary]
    }

    init(model: AppModel, interfaceController: CPInterfaceController) {
        self.model = model
        self.interfaceController = interfaceController
        root = CPListTemplate(title: String(localized: "Shisho"), sections: [])
        super.init()
    }

    func start() {
        interfaceController.delegate = self
        interfaceController.setRootTemplate(root, animated: false, completion: completion("show the root list"))
        carSession = CPSessionConfiguration(delegate: self)
        configureNowPlaying()
        watchSession()
        watchPlayer()
        log("connected")
        model.carPlayConnected()
    }

    func stop() {
        rootUpdates?.cancel()
        listUpdates?.cancel()
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

    // MARK: - Watching

    /// Fills the root from the signed-in account, again whenever the account
    /// changes, leaving nothing of the last one's to play.
    private func watchSession() {
        let session = withObservationTracking {
            model.session
        } onChange: { [weak self] in
            Task { @MainActor in self?.watchSession() }
        }
        guard session !== self.session || rootUpdates == nil else { return }
        self.session = session
        rootUpdates?.cancel()
        listUpdates?.cancel()
        lastRoot = nil
        lastPushed = []
        pushed = nil
        if interfaceController.templates.count > 1 {
            interfaceController.popToRootTemplate(animated: false, completion: completion("go back to the root list"))
        }
        root.updateSections([])
        guard let session else {
            root.emptyViewTitleVariants = [String(localized: "Not Signed In")]
            root.emptyViewSubtitleVariants = [String(localized: "Shisho lists your audiobooks here once it's signed in to your server.")]
            rootUpdates = Task {}
            return
        }
        root.emptyViewTitleVariants = []
        root.emptyViewSubtitleVariants = []
        let observation = ValueObservation.tracking(regions: CatalogStore.carPlayTables, fetch: rootFetch(session))
            .removeDuplicates()
        rootUpdates = Task { [weak self] in
            do {
                for try await content in observation.values(in: session.store.pool) {
                    guard let self, self.session === session else { return }
                    self.showRoot(content)
                }
            } catch {
                self?.log("couldn't read the root list: \(Diagnostics.describe(error))")
            }
        }
    }

    private func rootFetch(_ session: AccountSession) -> @Sendable (Database) throws -> RootContent {
        let deviceID = session.account.deviceID
        return { db in
            RootContent(continueListening: try CatalogStore.continueListening(db, limit: CarPlayRootLayout.continueListeningLimit,
                                                                               ownDeviceID: deviceID),
                        libraries: try CatalogStore.audiobookLibraries(db))
        }
    }

    /// Reads the root again when what's playing changes, for the playing
    /// indicator and for a File played to its end, whose place the root's
    /// observation doesn't track. A stream that fails is said in the car.
    private func watchPlayer() {
        withObservationTracking {
            _ = model.player?.file?.fileID
            _ = model.player?.isPlaying
            _ = model.player?.error
        } onChange: { [weak self] in
            Task { @MainActor in
                guard let self else { return }
                self.refreshRoot()
                self.pushed?.updateSections([CPListSection(items: self.lastPushed.map(self.item))])
                let error = self.model.player?.error
                if let error, error != self.lastError { self.alert(error) }
                self.lastError = error
                self.watchPlayer()
            }
        }
    }

    private func refreshRoot() {
        guard let session, lastRoot != nil else { return }
        let fetch = rootFetch(session)
        Task { [weak self] in
            guard let content = try? await session.store.pool.read(fetch) else { return }
            guard let self, self.session === session else { return }
            self.showRoot(content)
        }
    }

    /// The car changed what it limits: the root fits its rows again.
    nonisolated func sessionConfiguration(_ sessionConfiguration: CPSessionConfiguration,
                                          limitedUserInterfacesChanged limitedUserInterfaces: CPLimitableUserInterface) {
        Task { @MainActor in
            if let lastRoot = self.lastRoot { self.showRoot(lastRoot) }
        }
    }

    // MARK: - Lists

    private func showRoot(_ content: RootContent) {
        lastRoot = content
        let layout = CarPlayRootLayout.fit(continueListening: content.continueListening.count, libraries: content.libraries.count,
                                           maxItems: CPListTemplate.maximumItemCount)
        var sections: [CPListSection] = []
        if layout.continueListening > 0 {
            sections.append(CPListSection(items: content.continueListening.prefix(layout.continueListening).map(item),
                                          header: String(localized: "Continue Listening"), sectionIndexTitle: nil))
        }
        let downloads = row(String(localized: "Downloads"), icon: .download) { $0.showDownloads() }
        if content.libraries.isEmpty {
            sections.append(CPListSection(items: [downloads]))
        } else if layout.librariesInline {
            sections.append(CPListSection(items: [downloads]))
            sections.append(CPListSection(items: content.libraries.map(libraryRow), header: String(localized: "Libraries"),
                                          sectionIndexTitle: nil))
        } else {
            let libraries = row(String(localized: "Libraries"), icon: .library) { $0.showLibraries(content.libraries) }
            sections.append(CPListSection(items: [downloads, libraries]))
        }
        root.updateSections(sections)
    }

    /// A row that leads to another list, for the account it was listed for
    /// only.
    private func row(_ text: String, icon: Lucide, open: @escaping @MainActor (CarPlayScreens) -> Void) -> CPListItem {
        let row = CPListItem(text: text, detailText: nil, image: icon.uiImage, accessoryImage: nil, accessoryType: .disclosureIndicator)
        let listed = session
        row.handler = { [weak self] _, done in
            if let self, let listed, self.model.session === listed { open(self) }
            done()
        }
        return row
    }

    private func libraryRow(_ library: LibrarySummary) -> CPListItem {
        row(library.name, icon: .library) { $0.showLibrary(library) }
    }

    /// The Libraries, when there are more than the root has rows for.
    private func showLibraries(_ libraries: [LibrarySummary]) {
        let list = CPListTemplate(title: String(localized: "Libraries"),
                                  sections: [CPListSection(items: libraries.prefix(CPListTemplate.maximumItemCount).map(libraryRow))])
        interfaceController.pushTemplate(list, animated: true, completion: completion("show the Libraries"))
    }

    private func showDownloads() {
        push(title: String(localized: "Downloads"), empty: String(localized: "No Audiobooks Downloaded"), logName: "downloads",
             fetch: CatalogStore.downloadedAudiobooks)
    }

    private func showLibrary(_ library: LibrarySummary) {
        let id = library.id
        let sort = model.sortSpec(libraryID: id)
        push(title: library.name, empty: String(localized: "No Audiobooks"), logName: "library \(id)",
             fetch: { try CatalogStore.audiobooks($0, libraryID: id, sort: sort) })
    }

    /// Pushes a list of audiobooks, kept up to date while it shows. It
    /// shows as many as the car allows.
    private func push(title: String, empty: String, logName: String,
                      fetch: @escaping @Sendable (Database) throws -> [CarPlayAudiobook]) {
        guard let session else { return }
        let list = CPListTemplate(title: title, sections: [])
        list.emptyViewTitleVariants = [empty]
        pushed = list
        lastPushed = []
        listUpdates?.cancel()
        let maximum = CPListTemplate.maximumItemCount
        let observation = ValueObservation.tracking(regions: CatalogStore.carPlayTables) { db in
            Array(try fetch(db).prefix(maximum))
        }.removeDuplicates()
        listUpdates = Task { [weak self, weak list] in
            do {
                for try await books in observation.values(in: session.store.pool) {
                    guard let self, let list, self.session === session else { return }
                    self.lastPushed = books
                    list.updateSections([CPListSection(items: books.map(self.item))])
                }
            } catch {
                self?.log("couldn't read the \(logName) list: \(Diagnostics.describe(error))")
            }
        }
        interfaceController.pushTemplate(list, animated: true, completion: completion("show the \(logName) list"))
    }

    nonisolated func templateDidDisappear(_ aTemplate: CPTemplate, animated: Bool) {
        let gone = ObjectIdentifier(aTemplate)
        Task { @MainActor in
            // Popped: the list stops updating.
            guard let pushed = self.pushed, ObjectIdentifier(pushed) == gone,
                  !self.interfaceController.templates.contains(where: { $0 === pushed })
            else { return }
            self.listUpdates?.cancel()
            self.pushed = nil
            self.lastPushed = []
        }
    }

    /// A row for an audiobook, with its cover, how far through it the user
    /// is, and whether it's playing; selecting it plays it.
    private func item(_ book: CarPlayAudiobook) -> CPListItem {
        let item = CPListItem(text: book.title, detailText: book.authors.isEmpty ? nil : book.authors)
        item.playbackProgress = CGFloat(book.progress ?? 0)
        if let player = model.player, player.file?.fileID == book.fileID {
            item.isPlaying = player.isPlaying
        }
        let listed = session
        item.handler = { [weak self] _, done in
            self?.play(book, listedFor: listed)
            done()
        }
        setCover(of: book, on: item)
        return item
    }

    private func setCover(of book: CarPlayAudiobook, on item: CPListItem) {
        guard let covers = session?.covers, !book.coverVersion.isEmpty else { return }
        let size = CoverCache.pixelSize(forWidth: CPListItem.maximumImageSize.height,
                                        scale: interfaceController.carTraitCollection.displayScale)
        if let image = covers.memoryImage(book.cover, version: book.coverVersion, size: size) {
            item.setImage(image)
            return
        }
        Task {
            if let image = await covers.image(book.cover, version: book.coverVersion, size: size) {
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
