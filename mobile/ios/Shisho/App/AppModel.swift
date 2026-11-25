import BackgroundTasks
import Foundation
import Network
import Observation
import os
import ShishoAPI
import UIKit

/// The signed-in account and everything that works on it: its HTTP layer,
/// its copy of the catalog, its sync, and its covers.
final class AccountSession {
    let account: Account
    let client: APIClient
    let store: CatalogStore
    let sync: SyncEngine
    let covers: CoverCache
    let downloads: DownloadManager

    /// The capabilities the server listed in its last response, which any
    /// thread may read.
    let capabilities: OSAllocatedUnfairLock<[String]>

    @MainActor
    init(account: Account, credential: String, onCapabilities: @escaping @Sendable ([String]) -> Void,
         onInstance: @escaping @Sendable (String) -> Void = { _ in }) throws {
        self.account = account
        let capabilities = OSAllocatedUnfairLock(initialState: account.capabilities)
        self.capabilities = capabilities
        client = APIClient(
            baseURL: account.serverURL, credential: credential, instanceID: account.instanceID ?? "",
            onCapabilities: { listed in
                capabilities.withLock { $0 = listed }
                onCapabilities(listed)
            },
            onInstance: onInstance
        )
        try FileManager.default.createDirectory(at: account.folder, withIntermediateDirectories: true)
        store = try CatalogStore(path: AccountData(accountID: account.id).catalogPath)
        sync = SyncEngine(client: client, store: store, deviceID: account.deviceID) { capabilities.withLock { $0 } }
        covers = CoverCache(client: client, accountID: account.id, renderKey: account.thumbnailRenderKey)
        try? FileManager.default.removeItem(at: AccountData(accountID: account.id).legacyEbookFolder)
        downloads = DownloadManager(account: account, client: client, store: store)
    }

    /// The server lists sharing a File out with its current metadata.
    var sharesFiles: Bool {
        capabilities.withLock { $0.contains(Shishoapp.Capability.shareFiles.rawValue) }
    }

    /// The server sends a comic's or PDF's pages as thumbnails.
    var sendsPageImages: Bool {
        capabilities.withLock { $0.contains(Shishoapp.Capability.pageImages.rawValue) }
    }
}

/// What the last sync found, shown above the Library until the next one
/// finishes.
enum SyncStatus: Equatable {
    case idle
    /// The device has no network, or the server couldn't be reached; the
    /// copy on the device still works.
    case offline
    /// iOS keeps the app off the local network, where the server is.
    case localNetworkDenied
    /// The server and the app don't share a protocol. Syncing waits for an
    /// update; everything on the device keeps working (ADR 0020).
    /// appTooOld says the update is the app's.
    case versionGap(String, appTooOld: Bool)
    case failed(String)
}

@MainActor
@Observable
final class AppModel {
    /// The one model, which the phone's screen and CarPlay share. CarPlay
    /// can launch the app with no screen, so whichever comes first makes it.
    static let shared = AppModel()

    private let accounts = AccountStore()

    private(set) var session: AccountSession? {
        didSet { sessionChanged(from: oldValue) }
    }
    private(set) var syncStatus: SyncStatus = .idle
    /// The capabilities the signed-in account's server listed last, so a
    /// feature that writes an event type it doesn't list stays hidden (ADR
    /// 0024).
    private(set) var capabilities: Set<String> = []
    /// The signed-in account's audiobook player.
    private(set) var player: AudioPlayer?
    /// Now Playing is open.
    var showingPlayer = false
    /// The ebook open in the reader, which records reading whatever the
    /// screen does, as the player records listening.
    private(set) var reader: OpenReader?
    /// The tab showing, and each tab's navigation: the pages pushed on it.
    var tab: AppTab = .library
    /// The Related tab a Book opened from the Related panel starts on, as
    /// the web carries its tab along the panel's links. The Book's page takes
    /// it when it opens.
    var relatedTabForNextBook: String?
    /// Each tab's pages, and which tabs' searches are active.
    var navigation = TabNavigation()
    var libraryPath: [Route] {
        get { navigation.path(.library) }
        set { navigation.setPath(newValue, on: .library) }
    }
    var readingPath: [Route] {
        get { navigation.path(.reading) }
        set { navigation.setPath(newValue, on: .reading) }
    }
    var listsPath: [Route] {
        get { navigation.path(.lists) }
        set { navigation.setPath(newValue, on: .lists) }
    }
    var downloadsPath: [Route] {
        get { navigation.path(.downloads) }
        set { navigation.setPath(newValue, on: .downloads) }
    }
    /// The Account sheet is open.
    var showingAccount = false
    /// The sign-in sheet for adding another account is open.
    var addingAccount = false
    /// The Library picked in the Library, kept for each account, or 0 for
    /// every Library.
    var libraryID = 0 {
        didSet {
            guard let session, libraryID != oldValue else { return }
            UserDefaults.standard.set(libraryID, forKey: AccountData(accountID: session.account.id).libraryIDKey)
            loadSort()
            // Filters belong to the Library they were picked in, as the web
            // drops them with the Library's address: another Library's
            // genres and tags would match nothing here.
            filter = LibraryFilter()
        }
    }
    /// What the Library grid is narrowed to, for the account showing.
    var filter = LibraryFilter()
    /// The order picked on this device for the Library showing, in the sort
    /// parameter form, or nil for the one the user saved on the web. It's
    /// kept for each account and Library.
    var sortSpec: String? {
        didSet {
            guard let session else { return }
            UserDefaults.standard.set(sortSpec, forKey: sortKey(session))
        }
    }

    private func sortKey(_ session: AccountSession) -> String {
        AccountData(accountID: session.account.id).sortKey(libraryID: libraryID)
    }

    /// The order picked on this device for a Library of the account showing,
    /// or nil for the one saved on the web.
    func sortSpec(libraryID: Int) -> String? {
        session.flatMap { AccountData(accountID: $0.account.id).pickedSort(libraryID: libraryID) }
    }

    private func loadSort() {
        let spec = sortSpec(libraryID: libraryID)
        if spec != sortSpec { sortSpec = spec }
    }
    private let syncs = SyncRunner()
    var isSyncing: Bool { syncs.isRunning || isSigningOut }
    /// The account showing is being signed out, sending its unsent changes.
    var isSigningOut: Bool { session.map { signingOut.contains($0.account.id) } == true }
    /// False while the device has no network at all, as in airplane mode
    /// without Wi-Fi, so a sync says so at once instead of waiting on a
    /// request that can't go anywhere.
    private var networkAvailable = true
    /// Set when the server ended this installation's sign-in, for the
    /// sign-in screen to explain.
    private(set) var signedOutAccount: Account?
    /// The account signed out because the server at its address is no longer
    /// the one it was made on, which the sign-in screen explains instead.
    private(set) var serverReplacedAccountID: String?

    init() {
        ReaderPagePool.loadPreviewSupport()
        if let current = accounts.current {
            if current.signedOut {
                signedOutAccount = current
            } else if let credential = Keychain.credential(for: current.id) {
                session = try? openSession(account: current, credential: credential)
            }
        }
        Task { [weak self] in
            for await path in NWPathMonitor() {
                self?.networkChanged(available: path.status != .unsatisfied,
                                     wifi: (path.usesInterfaceType(.wifi) || path.usesInterfaceType(.wiredEthernet))
                                         && !path.isExpensive && !path.isConstrained)
            }
        }
        // Observers don't run for what init sets.
        sessionChanged(from: nil)
        let device = Installation.deviceName
        Task {
            await Diagnostics.shared.log(.app, "launched \(Installation.appVersion) on \(device), iOS \(UIDevice.current.systemVersion)")
        }
        #if DEBUG
        if session == nil, let harness = HarnessSignIn.launched {
            Task { [weak self] in
                do {
                    try await self?.signIn(server: harness.server, username: harness.username, password: harness.password)
                } catch {
                    await Diagnostics.shared.log(.app, "the harness's sign-in failed: \(Diagnostics.describe(error))")
                }
            }
        }
        #endif
    }

    /// A new session gets its own player, which picks up where the last
    /// launch left off; the old one stops, recording what was listened.
    private func sessionChanged(from old: AccountSession?) {
        guard old !== session else { return }
        player?.stop()
        player = nil
        showingPlayer = false
        closeReader()
        // Another account's transfers stop where they are, and carry on
        // when it's back.
        old?.downloads.stop()
        navigation = TabNavigation()
        showingAccount = false
        stopPrefetching()
        stopChecking()
        libraryID = session.map { UserDefaults.standard.integer(forKey: AccountData(accountID: $0.account.id).libraryIDKey) } ?? 0
        capabilities = Set(session?.capabilities.withLock { $0 } ?? [])
        filter = LibraryFilter()
        loadSort()
        if let session {
            let player = AudioPlayer(session: session) { [weak self] in
                Task { await self?.syncNow() }
            }
            self.player = player
            player.restore()
            session.downloads.onChange = { [weak self] fileID in
                self?.player?.downloadChanged(fileID: fileID)
            }
            session.downloads.networkChanged(to: downloadNetwork)
            // The account's server: reachable until a request or a check
            // says not, on whatever network the device has.
            session.client.reachability.networkChanged(available: networkAvailable)
            let opened = ObjectIdentifier(session)
            session.client.reachability.observe { [weak self] connection in
                Task { @MainActor in
                    // Not for an account since switched away from.
                    guard let self, let current = self.session, ObjectIdentifier(current) == opened else { return }
                    self.connectionChanged(to: connection)
                }
            }
            connection = session.client.reachability.connection
        } else {
            connection = networkAvailable ? .online : .offline
        }
    }

    // MARK: - Connection

    /// Whether the account's server can be reached, which every screen
    /// shows: things not on the device say at once that they need a
    /// connection rather than trying and waiting.
    private(set) var connection: Connection = .online
    /// Checks the server every so often while it can't be reached.
    @ObservationIgnored private var checking: Task<Void, Never>?
    @ObservationIgnored private var active = true
    /// CarPlay is showing, which, like the app being in front, keeps
    /// checking a server that can't be reached.
    @ObservationIgnored private var carPlayShowing = false

    private func connectionChanged(to new: Connection) {
        let old = connection
        connection = new
        player?.connectionChanged(new)
        reader?.connectionChanged(new)
        switch new {
        case .online:
            stopChecking()
            // Back: what waited goes now.
            if old != .online { Task { await syncNow() } }
        case .offline:
            stopChecking()
            // A sync in flight can't finish; it would only time out.
            syncs.stop()
            syncStatus = .offline
        case .unreachable:
            syncs.stop()
            syncStatus = .offline
            startChecking()
        }
    }

    /// Whether a File can be listened to or read now: it's on the device, or
    /// the server can be reached, or it's already playing.
    func canOpen(fileID: Int) -> Bool {
        ConnectionRules.canOpen(connection: connection,
                                downloaded: connection != .online && session?.downloads.localFile(fileID: fileID) != nil,
                                playing: player?.file?.fileID == fileID && player?.isPlaying == true)
    }

    /// Asks the server whether it answers now, quickly.
    private func checkServer() {
        guard let session else { return }
        Task { await session.client.checkServer() }
    }

    /// While the server can't be reached and the app is in front, asks it
    /// again every so often, so the app notices it's back without anyone
    /// waiting on a request.
    private func startChecking() {
        guard checking == nil, active || carPlayShowing, let session else { return }
        checking = Task { [weak self] in
            while !Task.isCancelled {
                if await session.client.checkServer() { break }
                try? await Task.sleep(for: .seconds(20))
            }
            self?.checking = nil
        }
    }

    private func stopChecking() {
        checking?.cancel()
        checking = nil
    }

    /// Audio was playing when the app went to the background.
    @ObservationIgnored private var playingInBackground = false

    func didEnterBackground() {
        playingInBackground = player?.isPlaying == true
        reader?.background()
        active = false
        if !carPlayShowing { stopChecking() }
    }

    // MARK: - CarPlay

    /// CarPlay connected, perhaps launching the app with no screen: the
    /// server is checked as it is with the app in front, and synced, since
    /// nothing else has asked for the places of other devices yet.
    func carPlayConnected() {
        carPlayShowing = true
        checkConnection()
        Task { await syncNow() }
    }

    /// CarPlay went, as when the car turns off, with the app perhaps never
    /// on screen: what was listened goes now, and the background refresh
    /// sends it later if the server can't be reached from the car.
    func carPlayDisconnected() {
        carPlayShowing = false
        if !active { stopChecking() }
        scheduleBackgroundRefresh()
        Task { await syncNow() }
    }

    /// Asks the server now, and keeps asking while it can't be reached, as
    /// when CarPlay is asked to stream with the server out of reach.
    func checkConnection() {
        guard connection != .online else { return }
        checkServer()
        if connection == .unreachable { startChecking() }
    }

    /// The app is leaving the screen, perhaps to be swiped away, which skips
    /// the background: reading time ends here, and goes on if the reader is
    /// back within 30 seconds.
    func didResignActive() {
        reader?.background()
    }

    /// Coming back while the audio played on, as by tapping it on the lock
    /// screen or in Control Center, opens Now Playing, where its controls
    /// are. iOS doesn't say which of those brought the app back.
    func didBecomeActive() {
        reader?.foreground()
        session?.downloads.resume()
        active = true
        // The network may have changed while the app was away.
        checkConnection()
        // Not over the reader, which was open as the audio played on.
        if playingInBackground, reader == nil, player?.isPlaying == true, player?.file != nil {
            showingPlayer = true
        }
        playingInBackground = false
    }

    /// Closes Now Playing onto book's page in the tab showing, pushing it
    /// unless it's showing.
    func showBook(_ book: BookSummary) {
        showFromPlayer(.book(book))
    }

    /// Goes back to tab's first screen.
    /// Opens a Book from one of its own pages, such as a File's: back to it
    /// when it's the screen before, rather than a second copy on top.
    func returnToBook(_ book: BookSummary) {
        let path = navigation.path(tab)
        if path.count >= 2, case .book(let previous) = path[path.count - 2], previous.id == book.id {
            navigation.popLast(tab)
        } else {
            push(.book(book))
        }
    }

    func popToRoot(_ tab: AppTab) {
        navigation.popToRoot(tab)
    }

    /// Asks a tab's first screen to scroll to its top; each ask is a new
    /// number, which the screen watches.
    private(set) var scrollToTop: [AppTab: Int] = [:]

    /// The tab showing, tapped again: back to its first screen, or on it,
    /// back to its top, as a native tab bar does.
    func reselect(_ tab: AppTab) {
        if navigation.path(tab).isEmpty {
            scrollToTop[tab, default: 0] += 1
        } else {
            popToRoot(tab)
        }
    }

    /// Closes Now Playing onto route's page in the tab showing, pushing it
    /// unless it's the page under the player.
    func showFromPlayer(_ route: Route) {
        showingPlayer = false
        if navigation.path(tab).last.map({ !$0.isSamePage(as: route) }) ?? true {
            push(route)
        }
    }

    /// Opens route on the tab showing.
    func push(_ route: Route) {
        navigation.push(route, on: tab)
    }

    // MARK: - Accounts

    /// Every account this installation signed in to, the current one first.
    var allAccounts: [Account] {
        // The account store isn't observable; reading session makes this
        // update whenever the account showing changes, which is when the
        // list does.
        _ = session
        let current = accounts.current?.id
        return accounts.accounts.sorted { ($0.id == current ? 0 : 1) < ($1.id == current ? 0 : 1) }
    }

    /// Opens another account at once: each has its own database, so what
    /// shows is that account's copy, and a sync brings it up to date. One
    /// whose sign-in ended opens the sign-in screen for it.
    func switchAccount(to id: String) {
        guard id != session?.account.id, let account = accounts.accounts.first(where: { $0.id == id }) else { return }
        syncs.stop()
        syncStatus = .idle
        accounts.save(account)
        guard !account.signedOut, let credential = Keychain.credential(for: account.id),
              let opened = try? openSession(account: account, credential: credential)
        else {
            signedOutAccount = account
            session = nil
            return
        }
        signedOutAccount = nil
        session = opened
        Task { await syncNow() }
    }

    /// Opens an audiobook File in the player and shows Now Playing.
    func listen(fileID: Int) {
        player?.open(fileID: fileID, autoplay: true)
        showingPlayer = true
    }

    /// Plays an audiobook File from seconds in, such as a chapter's start.
    func listen(fileID: Int, from seconds: Double) {
        guard let player else { return }
        player.open(fileID: fileID, autoplay: false)
        // A File that couldn't be opened, such as one a sync just removed,
        // leaves the one before it loaded, whose place mustn't move.
        guard player.file?.fileID == fileID else { return }
        player.seek(to: seconds)
        player.play()
        showingPlayer = true
    }

    /// Opens an ebook File in the reader: foliate's for a Reflowable File,
    /// the image pager for a Page-based one. page opens a Page-based File
    /// there, as a tile of its page grid does, rather than at its place.
    func read(fileID: Int, page: Int? = nil) {
        guard let session else { return }
        let file: ReadingFile?
        do {
            file = try session.store.pool.read { try CatalogStore.readingFile($0, fileID: fileID) }
        } catch {
            Task { await Diagnostics.shared.log(.reader, "file \(fileID): couldn't read it from the catalog: \(Diagnostics.describe(error))") }
            return
        }
        guard let file else { return }
        closeReader()
        showingPlayer = false
        let requestSync: @MainActor () -> Void = { [weak self] in Task { await self?.syncNow() } }
        let onGone: @MainActor () -> Void = { [weak self] in self?.closeReader() }
        reader = file.isPageBased
            ? .pages(PageReaderModel(file: file, session: session, page: page, requestSync: requestSync, onGone: onGone))
            : .reflowable(ReaderModel(file: file, session: session, requestSync: requestSync, onGone: onGone))
    }

    /// Closes the reader, recording what was read.
    func closeReader() {
        reader?.close()
        reader = nil
    }

    /// The diagnostics export, for the user to read before sharing it: the
    /// log with every server address and credential removed, and titles
    /// only when includeTitles.
    func diagnosticsText(includeTitles: Bool) async -> String {
        var summaries: [Diagnostics.AccountSummary] = []
        var titles: [String: String]?
        if let session {
            let pending = (try? await session.store.pool.read(CatalogStore.pendingCount)) ?? 0
            summaries.append(Diagnostics.AccountSummary(
                serverVersion: session.account.serverVersion,
                capabilities: session.capabilities.withLock { $0 },
                pendingEvents: pending
            ))
            if includeTitles {
                let ids = await Diagnostics.shared.mentionedIDs()
                titles = try? await session.store.pool.read { db in
                    try CatalogStore.titles(db, books: ids.books, files: ids.files)
                }
            }
        }
        return await Diagnostics.shared.export(
            accounts: summaries, servers: accounts.accounts.map(\.serverURL), titles: titles,
            device: Installation.deviceName, system: "iOS \(UIDevice.current.systemVersion)"
        )
    }

    func dismissJournalNotes() {
        try? session?.store.dismissJournalNotes()
    }

    // MARK: - The reading record

    /// Fetches Reading Stats for a year, or all time, that the device has
    /// no copy of yet. Offline, the page says it needs a connection.
    func loadStats(year: Int?) async {
        guard let session, networkAvailable else { return }
        do {
            try await session.sync.fetchStats(only: .some(year))
        } catch {
            await Diagnostics.shared.log(.sync, "reading stats not fetched: \(Diagnostics.describe(error))")
        }
    }

    /// The account's server lists capability.
    func offers(_ capability: Shishoapp.Capability) -> Bool {
        capabilities.contains(capability.rawValue)
    }

    /// Records a change to the user's reading record: it shows at once, and
    /// goes to the server now when it can, or at the next sync (ADR 0024).
    /// A List it makes returns its id here.
    @discardableResult
    func change(_ change: ReadingChange) -> Int? {
        guard let session, offers(change.capability) else { return nil }
        do {
            let made = try session.store.record(change)
            if case .clearPosition(let fileID) = change {
                player?.positionCleared(fileID: fileID)
            }
            Task { await syncNow() }
            return made
        } catch {
            Task { await Diagnostics.shared.log(.app, "reading change not saved: \(Diagnostics.describe(error))") }
            return nil
        }
    }

    // MARK: - Signing in and out

    /// Signs in, and downloads the catalog. Throws a message to show.
    func signIn(server: String, username: String, password: String) async throws {
        let address: ServerAddress
        do {
            address = try ServerAddress(server)
        } catch let problem as ServerAddress.Problem {
            throw SignInError(message: Messages.address(problem))
        }
        let client = APIClient(baseURL: address.url, credential: nil)
        let startedAt = accountsOpened
        // Closing Add Server cancels a sign-in started there.
        let addServerClosingsAtStart = addingAccount ? addServerClosings : nil
        let response: Shishoapp.SignInResponse
        do {
            response = try await client.signIn(username: username, password: password, deviceName: Installation.deviceName)
        } catch let error as APIError {
            throw SignInError(
                message: Messages.signIn(error), offersUpdate: Messages.isAppTooOld(error),
                offersSettings: error == .localNetworkDenied, retriesWhenActive: Messages.isUnreachable(error)
            )
        }
        await beginAccountChange()
        do {
            try await open(response, at: address, startedAt: startedAt, addServerClosingsAtStart: addServerClosingsAtStart)
        } catch {
            endAccountChange()
            throw error
        }
        endAccountChange()
        await syncNow()
    }

    /// How many times the Add Server sheet closed, so a sign-in started in it
    /// can tell it was canceled. It also counts the close that follows a
    /// sign-in's own success, after which nothing checks.
    private var addServerClosings = 0

    func addServerClosed() {
        addServerClosings += 1
    }

    /// Ends a sign-in that won't open its account: the server forgets the
    /// Device it made.
    private func abandon(_ response: Shishoapp.SignInResponse, at address: ServerAddress, saying message: String) -> SignInError {
        let device = APIClient(baseURL: address.url, credential: response.credential)
        Task { await device.signOut() }
        return SignInError(message: message)
    }

    /// How many accounts sign-ins have opened, and when each last was, so a
    /// sign-in answered late can tell what opened while it waited.
    private var accountsOpened = 0
    private var openedAt: [String: Int] = [:]

    /// Opens the account a sign-in is for, replacing the one for a server
    /// that was here before, and shows it. Each sign-in matches accounts
    /// only once those before it have finished, so two can't both replace
    /// the same account.
    private func open(_ response: Shishoapp.SignInResponse, at address: ServerAddress, startedAt: Int,
                      addServerClosingsAtStart: Int?) async throws {
        // A sign-in from Add Server, canceled since, mustn't replace an
        // account once the user has turned away from it. Checked again after
        // the last wait below, before anything is kept.
        let canceled = { [unowned self] in addServerClosingsAtStart.map { $0 != addServerClosings } ?? false }
        if canceled() {
            throw abandon(response, at: address, saying: String(localized: "Signing in was canceled."))
        }
        let instanceID = response.server.instanceId
        let known: Account?
        var replaced: Account?
        switch accounts.match(serverURL: address.url, userID: response.user.id, instanceID: instanceID) {
        case .new: known = nil
        case .same(let account): known = account
        case .replaced(let account):
            known = nil
            replaced = account
        }
        // An account a sign-in opened while this one waited for its answer
        // is newer than that answer, which may be from a server since
        // replaced again, so it isn't removed on that answer's word.
        if let replaced, openedAt[replaced.id, default: 0] > startedAt {
            throw abandon(response, at: address, saying: String(localized: "Another sign-in to this server finished first. Sign in again to use this one."))
        }
        var account = known ?? Account(
            id: UUID().uuidString.lowercased(), serverURL: address.url, username: response.user.username,
            userID: response.user.id, deviceID: response.deviceId, serverVersion: response.serverVersion,
            thumbnailRenderKey: response.server.thumbnailRenderKey, capabilities: response.server.capabilities,
            signedOut: false
        )
        // An account saved before servers had an instance id adopts the
        // first one it sees; a server too old to send one changes nothing.
        if !instanceID.isEmpty {
            account.instanceID = instanceID
        }
        account.username = response.user.username
        account.deviceID = response.deviceId
        account.serverVersion = response.serverVersion
        account.thumbnailRenderKey = response.server.thumbnailRenderKey
        account.capabilities = response.server.capabilities
        account.signedOut = false
        // The server was set up again from scratch at this address, so the
        // account signed in to before is for a server that's gone: its
        // unsent changes would land on whatever has the same ids here. It's
        // removed once the new account opens, and the user told what was
        // lost. If it's still open, what its player and reader hold is
        // recorded, and its sync stopped, before its changes are counted.
        if let replaced, session?.account.id == replaced.id {
            player?.stop()
            closeReader()
            syncs.stop()
        }
        let removedChanges = if let replaced { await unsentChanges(replaced) } else { 0 }
        if canceled() {
            throw abandon(response, at: address, saying: String(localized: "Signing in was canceled."))
        }
        do {
            try Keychain.setCredential(response.credential, for: account.id)
        } catch {
            // The Device exists on the server, but this installation couldn't
            // keep its credential, so it signs that Device out again.
            throw abandon(response, at: address, saying: String(localized: "The app couldn't save your sign-in on this device. Try again."))
        }
        let opened: AccountSession
        do {
            opened = try openSession(account: account, credential: response.credential)
            // A copy kept from an earlier sign-in may be from a server since
            // reinstalled or restored at the same address, whose log has
            // reused its cursor. It downloads again from the start, and stays
            // on screen until that finishes.
            if known != nil {
                try opened.store.startOverIfNeeded(serverSaidSo: true)
            }
        } catch {
            // A new account that couldn't open leaves nothing behind, and an
            // account it would replace stays until a sign-in succeeds.
            if known == nil {
                AccountData(accountID: account.id).remove()
            }
            throw SignInError(message: String(localized: "The app couldn't open its storage on this device. \(error.localizedDescription)"))
        }
        if let replaced {
            // Unsaved, the account it replaces is still the one on disk, so
            // this sign-in leaves nothing behind.
            guard accounts.save(account, replacing: replaced) else {
                AccountData(accountID: account.id).remove()
                throw abandon(response, at: address, saying: String(localized: "The app couldn't save your sign-in on this device. Try again."))
            }
        } else {
            accounts.save(account)
        }
        accountsOpened += 1
        openedAt[account.id] = accountsOpened
        // A sync of the account showing until now must not land on this
        // one, nor sign it out.
        syncs.stop()
        syncStatus = .idle
        session = opened
        signedOutAccount = nil
        serverReplacedAccountID = nil
        addingAccount = false
        // Changes that couldn't be counted are said too. Without a
        // replacement, a notice from an earlier sign-in stays until it's
        // dismissed, shown only on its own account.
        if replaced != nil, removedChanges != 0 {
            replacedAccountNotice = ReplacedAccountNotice(accountID: account.id, removedChanges: removedChanges)
            // The notice is on the Library, and Add Server opens from any tab.
            tab = .library
        }
        if replaced != nil {
            await Diagnostics.shared.log(.app, "removed an account whose server was set up again, with \(removedChanges.map(String.init) ?? "an unknown number of") unsent changes")
        }
    }

    /// How many changes an account hasn't sent, or nil when its journal
    /// can't be read.
    private func unsentChanges(_ account: Account) async -> Int? {
        do {
            if let session, session.account.id == account.id {
                return try await session.store.pool.read(CatalogStore.pendingCount)
            }
            let path = AccountData(accountID: account.id).catalogPath
            guard FileManager.default.fileExists(atPath: path) else { return 0 }
            return try await CatalogStore(path: path).pool.read(CatalogStore.pendingCount)
        } catch {
            await Diagnostics.shared.log(.app, "couldn't count a replaced account's unsent changes: \(Diagnostics.describe(error))")
            return nil
        }
    }

    /// Said after signing in to a server set up again from scratch, when
    /// the account it replaced had changes it never sent.
    private(set) var replacedAccountNotice: ReplacedAccountNotice?

    func dismissReplacedAccountNotice() {
        replacedAccountNotice = nil
    }

    /// The accounts being signed out while their unsent changes go, during
    /// which they start no sync.
    private(set) var signingOut: Set<String> = []

    /// Signs this installation out of the current account: it first sends
    /// what the account hasn't sent, waiting at most signOutSendLimit, then
    /// the app forgets the credential and the server the Device. The
    /// account's copy of the catalog, and anything not sent, stay for signing
    /// in again. It takes its turn with sign-ins (beginAccountChange).
    func signOut() async {
        guard let requested = session, !signingOut.contains(requested.account.id) else { return }
        let accountID = requested.account.id
        signingOut.insert(accountID)
        // A sign-in still opening its account finishes first; one started
        // meanwhile waits, so it isn't undone below. A sign-in that opened the
        // same account again meanwhile isn't what was signed out of.
        await beginAccountChange()
        guard let session, session === requested else {
            signingOut.remove(accountID)
            endAccountChange()
            return
        }
        // What the player and reader hold is recorded first, so it's sent
        // too.
        player?.stop()
        closeReader()
        // Unsent changes go first, so signing in again after the server was
        // set up from scratch rarely has any to lose. Offline, or with a
        // server that doesn't answer, the account keeps them.
        await sendBeforeSigningOut(session)
        signingOut.remove(accountID)
        // The account signs out even if another was opened meanwhile.
        Keychain.removeCredential(for: accountID)
        // The account stays in the list, with its data, marked signed out.
        if var account = stored(accountID) {
            account.signedOut = true
            accounts.save(account, makeCurrent: false)
        }
        if self.session?.account.id == accountID {
            // All of this before the turn ends and the server is told, which
            // can take a while.
            syncs.stop()
            accounts.leaveCurrent()
            self.session = nil
            signedOutAccount = nil
            syncStatus = .idle
            // Another account still signed in takes over.
            if let next = accounts.accounts.first(where: { !$0.signedOut && $0.id != accountID && Keychain.credential(for: $0.id) != nil }) {
                switchAccount(to: next.id)
            }
        }
        endAccountChange()
        await session.client.signOut()
    }

    /// Whether a sign-in or sign-out is changing the accounts. Each awaits
    /// the server or the journal partway, and one that ran in between could
    /// undo the other's work, so they take turns.
    private var changingAccounts = false
    private var accountChangeWaiters: [CheckedContinuation<Void, Never>] = []

    private func beginAccountChange() async {
        guard changingAccounts else {
            changingAccounts = true
            return
        }
        // endAccountChange hands its turn straight to this one.
        await withCheckedContinuation { accountChangeWaiters.append($0) }
    }

    private func endAccountChange() {
        if accountChangeWaiters.isEmpty {
            changingAccounts = false
        } else {
            accountChangeWaiters.removeFirst().resume()
        }
    }

    /// How long signing out waits for unsent changes to reach the server. A
    /// debug build the harness launched waits longer, since a CI simulator
    /// can stall for about ten seconds and the flow checks the send.
    static var signOutSendLimit: Duration {
        #if DEBUG
        if HarnessSignIn.launched != nil {
            return .seconds(30)
        }
        #endif
        return .seconds(8)
    }

    /// Sends session's journal once, giving up after signOutSendLimit.
    private func sendBeforeSigningOut(_ session: AccountSession) async {
        guard networkAvailable else { return }
        let pending: Int
        do {
            pending = try await session.store.pool.read(CatalogStore.pendingCount)
        } catch {
            await Diagnostics.shared.log(.sync, "couldn't count changes before signing out: \(Diagnostics.describe(error))")
            return
        }
        guard pending > 0 else { return }
        // A sync in flight may be pulling a long feed; only the journal
        // matters now. Events sent twice are taken once, by their ids.
        if self.session?.account.id == session.account.id {
            syncs.stop()
        }
        let sync = session.sync
        let limit = AppModel.signOutSendLimit
        await withTaskGroup(of: Void.self) { group in
            group.addTask {
                do {
                    try await sync.sendJournal()
                } catch {
                    await Diagnostics.shared.log(.sync, "changes not sent before signing out: \(Diagnostics.describe(error))")
                }
            }
            group.addTask { try? await Task.sleep(for: limit) }
            await group.next()
            group.cancelAll()
        }
    }

    /// The server ended the sign-in of session's account. Its data stays,
    /// waiting for the same user to sign in again.
    private func handleSignedOut(_ ended: AccountSession, serverReplaced: Bool = false) {
        guard ended === session else { return }
        syncs.stop()
        guard var account = stored(ended.account.id) else { return }
        account.signedOut = true
        Keychain.removeCredential(for: account.id)
        accounts.save(account)
        signedOutAccount = account
        serverReplacedAccountID = serverReplaced ? account.id : nil
        session = nil
        syncStatus = .idle
    }

    // MARK: - Syncing

    /// Brings the catalog up to date. Failing to reach the server leaves the
    /// copy on the device as it is.
    func syncNow() async {
        guard let session, !signingOut.contains(session.account.id) else { return }
        guard networkAvailable else {
            syncStatus = .offline
            return
        }
        // A sync asked for as playback pauses, with the phone locked, gets
        // the time to finish.
        // Each call has its own, since calls that overlap wait on one run.
        let key = UUID()
        backgroundSyncs[key] = UIApplication.shared.beginBackgroundTask(withName: "Sync") { [weak self] in
            // Out of time: the sync stops, keeping what it hadn't sent.
            self?.endBackgroundSync(key, stopping: true)
        }
        await syncs.run { [weak self] isCurrent in
            await self?.sync(session, isCurrent: isCurrent)
        }
        endBackgroundSync(key, stopping: false)
    }

    @ObservationIgnored private var backgroundSyncs: [UUID: UIBackgroundTaskIdentifier] = [:]

    private func endBackgroundSync(_ key: UUID, stopping: Bool) {
        guard let task = backgroundSyncs.removeValue(forKey: key) else { return }
        if stopping { syncs.stop() }
        if task != .invalid {
            UIApplication.shared.endBackgroundTask(task)
        }
    }

    // MARK: - Background refresh

    /// The background refresh task's identifier, listed in Info.plist.
    nonisolated static var refreshTaskID: String {
        (Bundle.main.bundleIdentifier ?? "com.shishobooks.shisho") + ".refresh"
    }

    /// Asks iOS to wake the app now and then to sync, so listening done
    /// offline reaches the server without opening the app.
    func scheduleBackgroundRefresh() {
        let request = BGAppRefreshTaskRequest(identifier: AppModel.refreshTaskID)
        request.earliestBeginDate = Date(timeIntervalSinceNow: 30 * 60)
        do {
            try BGTaskScheduler.shared.submit(request)
        } catch {
            Task { await Diagnostics.shared.log(.app, "background refresh not scheduled: \(Diagnostics.describe(error))") }
        }
    }

    /// iOS launched the app in the background for Downloads' transfers:
    /// the session they belong to is made again to take their events.
    /// Another account's wait until it's showing.
    func backgroundDownloads(identifier: String) async {
        guard let session, DownloadManager.isSession(identifier, accountID: session.account.id) else { return }
        await session.downloads.backgroundEvents(identifier: identifier)
    }

    /// A background refresh: sync once, and ask for the next.
    func backgroundRefresh() async {
        scheduleBackgroundRefresh()
        await syncNow()
    }

    /// One sync of session's catalog. A result the runner has since
    /// dropped, after a sign-out or the network going, changes nothing.
    private func sync(_ session: AccountSession, isCurrent: @MainActor () -> Bool) async {
        guard networkAvailable else {
            syncStatus = .offline
            return
        }
        // Last heard not to answer: asked once first, since a sync from a
        // pause or a background refresh may be what finds it back, with
        // nothing else checking.
        if !session.client.reachability.allowsRequests {
            _ = await session.client.checkServer()
        }
        do {
            try await session.sync.run()
            guard isCurrent() else { return }
            syncStatus = .idle
            // Caught up, so a File missing from the copy has left the library,
            // and one with a new size was replaced.
            session.downloads.dropOrphans()
            session.downloads.refreshReplaced()
            prefetchCoversIfWanted(session)
        } catch {
            await Diagnostics.shared.log(.sync, "sync failed: \(Diagnostics.describe(error))")
            guard isCurrent() else { return }
            record(error, for: session)
        }
    }

    /// What a failed sync tells the user. Only the server's own refusal of
    /// the credential signs the device out.
    private func record(_ error: Error, for session: AccountSession) {
        guard let error = error as? APIError else {
            syncStatus = .failed(error.localizedDescription)
            return
        }
        switch error {
        case .signedOut:
            handleSignedOut(session)
        case .serverReplaced:
            // The journal holds ids from the server that was here before, so
            // nothing more goes to this one.
            handleSignedOut(session, serverReplaced: true)
        case .noAccess:
            // The user's role can't read Books anymore: nothing in the copy
            // is theirs to see. The journal stays, to send if that changes.
            try? session.store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [], listIds: []), force: true)
            session.downloads.dropOrphans()
            syncStatus = .failed(Messages.signIn(error))
        case .unreachable:
            syncStatus = .offline
        case .localNetworkDenied:
            syncStatus = .localNetworkDenied
        case .appTooOld, .serverTooOld, .serverPredatesApp:
            syncStatus = .versionGap(Messages.signIn(error, serverVersion: session.account.serverVersion),
                                     appTooOld: Messages.isAppTooOld(error))
        default:
            syncStatus = .failed(Messages.signIn(error))
        }
    }

    /// The account's session, whose responses keep its capabilities current,
    /// and tell an account from before instance ids its server's.
    private func openSession(account: Account, credential: String) throws -> AccountSession {
        let accountID = account.id
        return try AccountSession(account: account, credential: credential, onCapabilities: { [weak self] listed in
            Task { @MainActor in self?.recordCapabilities(listed, accountID: accountID) }
        }, onInstance: { [weak self] instanceID in
            Task { @MainActor in self?.recordInstance(instanceID, accountID: accountID) }
        })
    }

    // MARK: - Covers

    /// Whether to download every cover on Wi-Fi, for every account on this
    /// device. Off until the user turns it on.
    var prefetchesCovers = AppModel.storedPrefetchesCovers {
        didSet {
            guard prefetchesCovers != oldValue else { return }
            UserDefaults.standard.set(prefetchesCovers, forKey: AppModel.prefetchCoversKey)
            if prefetchesCovers, let session {
                prefetchCoversIfWanted(session)
            } else {
                stopPrefetching()
            }
        }
    }

    private static let prefetchCoversKey = "prefetchCovers"

    /// The setting, or, before it applied to every account, on when any
    /// account had it on.
    private static var storedPrefetchesCovers: Bool {
        let defaults = UserDefaults.standard
        if defaults.object(forKey: prefetchCoversKey) != nil {
            return defaults.bool(forKey: prefetchCoversKey)
        }
        let before = defaults.dictionaryRepresentation().contains { key, value in
            key.hasPrefix(prefetchCoversKey + ".") && (value as? Bool) == true
        }
        defaults.set(before, forKey: prefetchCoversKey)
        return before
    }

    @ObservationIgnored private var onWiFi = false
    /// The network as Downloads weigh it.
    @ObservationIgnored private var downloadNetwork: DownloadNetwork = .wifi
    @ObservationIgnored private var prefetching: Task<Void, Never>?
    /// Which prefetch is current, so one that ends late doesn't clear a newer
    /// one's handle.
    @ObservationIgnored private var prefetchRun = 0
    /// A sync finished while a prefetch ran, perhaps bringing Books its list
    /// of covers predates: it goes over them again when it ends.
    @ObservationIgnored private var prefetchAgain = false

    private func stopPrefetching() {
        prefetching?.cancel()
        prefetching = nil
        prefetchRun += 1
    }

    /// Downloads every cover that isn't on the device yet while on Wi-Fi, so
    /// browsing and listening offline show them all: each Book's at the
    /// grid's size, and each File's own, at the size Now Playing shows it.
    /// Other sizes of a cover kept show offline in their place.
    private func prefetchCoversIfWanted(_ session: AccountSession) {
        guard prefetchesCovers, onWiFi else { return }
        guard prefetching == nil else {
            prefetchAgain = true
            return
        }
        prefetchAgain = false
        let size = CoverCache.pixelSize(forWidth: 140, scale: UIScreen.main.scale)
        prefetchRun += 1
        let run = prefetchRun
        prefetching = Task { [weak self] in
            let versions = (try? session.store.coverVersions()) ?? []
            for cover in versions {
                guard !Task.isCancelled, self?.session === session, self?.onWiFi == true else { break }
                _ = await session.covers.image(cover.owner, version: cover.version,
                                               size: cover.owner.isFile ? AudioPlayer.artworkSize : size)
            }
            guard let self, prefetchRun == run else { return }
            prefetching = nil
            if prefetchAgain, !Task.isCancelled {
                prefetchCoversIfWanted(session)
            }
        }
    }

    /// Coming back online syncs; going offline says so without waiting for
    /// a request to fail.
    private func networkChanged(available: Bool, wifi: Bool = false) {
        downloadNetwork = !available ? .offline : wifi ? .wifi : .cellular
        session?.downloads.networkChanged(to: downloadNetwork)
        let wasOnWiFi = onWiFi
        onWiFi = wifi
        if !wifi {
            stopPrefetching()
        } else if !wasOnWiFi, let session {
            prefetchCoversIfWanted(session)
        }
        guard available != networkAvailable else {
            // Another network, as from Wi-Fi to cellular, may reach the
            // server where the last one didn't, or not.
            if available { checkServer() }
            return
        }
        networkAvailable = available
        session?.client.reachability.networkChanged(available: available)
        if session == nil {
            connection = available ? .online : .offline
        }
        // A quick check says at once whether this network reaches the
        // server, rather than the first request finding out the slow way.
        if available { checkServer() }
    }

    private func recordCapabilities(_ listed: [String], accountID: String) {
        guard session?.account.id == accountID, var account = stored(accountID) else { return }
        if capabilities != Set(listed) {
            capabilities = Set(listed)
        }
        player?.capabilitiesChanged(listed)
        guard account.capabilities != listed else { return }
        account.capabilities = listed
        accounts.save(account, makeCurrent: false)
    }

    /// An account saved before servers had an instance id adopts the first
    /// one its server's responses carry.
    private func recordInstance(_ instanceID: String, accountID: String) {
        guard var account = stored(accountID), account.instanceID?.isEmpty ?? true else { return }
        account.instanceID = instanceID
        accounts.save(account, makeCurrent: false)
    }

    /// The account as last saved. A session's account is as it was when the
    /// session opened, so saving it would undo what's been saved since.
    private func stored(_ accountID: String) -> Account? {
        accounts.accounts.first { $0.id == accountID }
    }
}

/// An account removed at sign-in because its server was set up again from
/// scratch, and how many of its changes were never sent.
struct ReplacedAccountNotice: Equatable {
    /// The new account, which the notice shows on.
    let accountID: String
    /// Nil when the replaced account's journal couldn't be read.
    let removedChanges: Int?
}

struct SignInError: LocalizedError {
    let message: String
    /// The fix is updating the app, so the screen links to the update.
    var offersUpdate = false
    /// The fix is a permission in Settings, so the screen links there.
    var offersSettings = false
    /// The request couldn't get through, perhaps because iOS was asking for
    /// the Local Network permission: try again when the app is back in
    /// front.
    var retriesWhenActive = false
    var errorDescription: String? { message }
}

/// The words for each problem, in one place.
enum Messages {
    static func address(_ problem: ServerAddress.Problem) -> String {
        switch problem {
        case .empty:
            return String(localized: "Enter your Shisho server's address.")
        case .invalid:
            return String(localized: "That doesn't look like a server address. Try something like https://shisho.example.com.")
        case .insecureHTTP:
            return String(localized: "Plain HTTP only works for servers on your local network. Use the server's https:// address.")
        }
    }

    static func signIn(_ error: APIError, serverVersion: String? = nil) -> String {
        let app = Installation.appVersion
        let needed = Installation.minimumServerVersion
        switch error {
        case .serverPredatesApp:
            return String(localized: "This server's Shisho is too old for this version of the app (\(app)). The app needs Shisho \(needed) or newer. Ask the server's owner to update Shisho, or, if an authentication proxy guards the server, to let the app through it.")
        case .serverTooOld(let version):
            let running = version ?? serverVersion ?? String(localized: "an older release")
            return String(localized: "This server runs Shisho \(running), which is too old for this version of the app (\(app)). Ask the server's owner to update Shisho.")
        case .appTooOld(let version):
            let running = version ?? serverVersion ?? String(localized: "a newer release")
            return String(localized: "This version of the app (\(app)) is too old for this server, which runs Shisho \(running). Update the Shisho app to keep syncing.")
        case .demoMode:
            return String(localized: "This is a Shisho demo server. The Shisho app doesn't connect to demo servers; sign in to your own Shisho server instead.")
        case .wrongPassword:
            return String(localized: "Wrong username or password.")
        case .passwordResetRequired:
            return String(localized: "Your password has to be changed before you can sign in. Sign in on the web to change it, then try again.")
        case .noAccess:
            return String(localized: "Your account can't read Books on this server. Ask the server's owner for access.")
        case .signedOut:
            return String(localized: "This device was signed out of your account. Sign in again.")
        case .serverReplaced:
            return String(localized: "The server at this address was set up again since you signed in, so it's a different server now. Sign in again.")
        case .unreachable(let detail):
            return String(localized: "Couldn't reach the server. Check the address and your connection. (\(detail))")
        case .localNetworkDenied:
            return localNetworkDenied
        case .blockedByProxy:
            return String(localized: "Something in front of your Shisho server, such as an authentication proxy, turned the app away. Its owner can let the app through; Shisho's documentation on the Shisho app explains how.")
        case .server(_, let message):
            return message
        }
    }

    static let localNetworkDenied = String(localized: "Shisho isn't allowed on your local network, where this server is. Turn on Local Network for Shisho in Settings.")

    static func isUnreachable(_ error: APIError) -> Bool {
        switch error {
        case .unreachable, .localNetworkDenied: true
        default: false
        }
    }

    static func isAppTooOld(_ error: APIError) -> Bool {
        if case .appTooOld = error { return true }
        return false
    }
}

/// The app's tabs.
enum AppTab: Hashable {
    case library, reading, lists, downloads
}

/// Each tab's pages, and which tabs' searches are active. Every change to a
/// tab's pages through it closes that tab's search in the same update: on
/// iOS 26 a stack with a page over an active search ignores being emptied,
/// as tapping the tab again does, and keeps showing its pages while its path
/// says it's back at the first screen. A screen with `.searchable` binds its
/// `isPresented` to `searchActive` and opens pages with `AppModel.push`,
/// never `NavigationLink(value:)`, which changes the path behind its back.
struct TabNavigation: Equatable {
    private var paths: [AppTab: [Route]] = [:]
    var searchActive: Set<AppTab> = []

    /// The pages pushed on tab.
    func path(_ tab: AppTab) -> [Route] {
        paths[tab] ?? []
    }

    /// Sets tab's pages, as its NavigationStack does when going back, which
    /// closes the search as popLast does.
    mutating func setPath(_ path: [Route], on tab: AppTab) {
        if path.count < self.path(tab).count {
            searchActive.remove(tab)
        }
        paths[tab] = path
    }

    mutating func push(_ route: Route, on tab: AppTab) {
        searchActive.remove(tab)
        paths[tab, default: []].append(route)
    }

    mutating func popLast(_ tab: AppTab) {
        searchActive.remove(tab)
        paths[tab]?.removeLast()
    }

    mutating func popToRoot(_ tab: AppTab) {
        searchActive.remove(tab)
        paths[tab] = []
    }
}

/// A page pushed on a tab's navigation.
enum Route: Hashable {
    /// Whether two routes open the same page: the same Book or resource by
    /// id, though the copy a route holds, such as a Book's progress or a
    /// Person's name, may have changed since.
    func isSamePage(as other: Route) -> Bool {
        switch (self, other) {
        case let (.book(a), .book(b)): a.id == b.id
        case let (.resource(kindA, idA, _), .resource(kindB, idB, _)): kindA == kindB && idA == idB
        case let (.list(idA, _), .list(idB, _)): idA == idB
        default: self == other
        }
    }

    case book(BookSummary)
    /// Every Series, Person, Genre, or Tag of the Library picked.
    case browse(ResourceKind)
    /// One Series, Person, Genre, or Tag, and its Books.
    case resource(ResourceKind, id: Int, name: String)
    case list(id: Int, name: String)
    /// One of a Book's Files, on its own page.
    case file(bookID: Int, fileID: Int)
    /// Search over the device's copy, opened from a tab's toolbar.
    case search
    /// The Download settings, from the Downloads tab's toolbar.
    case downloadSettings
    /// The Reading tab's pages: every Book underway, To Read, Reading
    /// History, and Reading Stats.
    case currentlyReading
    case toRead
    case history
    case stats
}

/// The File open in a reader, which records reading whatever the screen
/// does: a Reflowable File in foliate's reader, or a Page-based one in the
/// image pager.
@MainActor
enum OpenReader {
    case reflowable(ReaderModel)
    case pages(PageReaderModel)

    var id: Int {
        switch self {
        case .reflowable(let model): model.id
        case .pages(let model): model.id
        }
    }

    func background() {
        switch self {
        case .reflowable(let model): model.background()
        case .pages(let model): model.background()
        }
    }

    func foreground() {
        switch self {
        case .reflowable(let model): model.foreground()
        case .pages(let model): model.foreground()
        }
    }

    func close() {
        switch self {
        case .reflowable(let model): model.close()
        case .pages(let model): model.close()
        }
    }

    func connectionChanged(_ connection: Connection) {
        switch self {
        case .reflowable(let model): model.connectionChanged(connection)
        case .pages(let model): model.connectionChanged(connection)
        }
    }
}
