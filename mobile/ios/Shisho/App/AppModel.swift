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

    /// The capabilities the server listed in its last response, which any
    /// thread may read.
    let capabilities: OSAllocatedUnfairLock<[String]>

    init(account: Account, credential: String, onCapabilities: @escaping @Sendable ([String]) -> Void) throws {
        self.account = account
        let capabilities = OSAllocatedUnfairLock(initialState: account.capabilities)
        self.capabilities = capabilities
        client = APIClient(baseURL: account.serverURL, credential: credential) { listed in
            capabilities.withLock { $0 = listed }
            onCapabilities(listed)
        }
        try FileManager.default.createDirectory(at: account.folder, withIntermediateDirectories: true)
        store = try CatalogStore(path: account.folder.appendingPathComponent("catalog.sqlite").path)
        sync = SyncEngine(client: client, store: store, deviceID: account.deviceID) { capabilities.withLock { $0 } }
        covers = CoverCache(client: client, accountID: account.id, renderKey: account.thumbnailRenderKey)
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
    private let accounts = AccountStore()

    private(set) var session: AccountSession? {
        didSet { sessionChanged(from: oldValue) }
    }
    private(set) var syncStatus: SyncStatus = .idle
    /// The signed-in account's audiobook player.
    private(set) var player: AudioPlayer?
    /// Now Playing is open.
    var showingPlayer = false
    /// The tab showing, and each tab's navigation: the pages pushed on it.
    var tab: AppTab = .library
    var libraryPath: [Route] = []
    var listsPath: [Route] = []
    var searchPath: [Route] = []
    /// The sign-in sheet for adding another account is open.
    var addingAccount = false
    /// The Library picked in the Library, kept for each account, or 0 for
    /// every Library.
    var libraryID = 0 {
        didSet {
            guard let session, libraryID != oldValue else { return }
            UserDefaults.standard.set(libraryID, forKey: "libraryID.\(session.account.id)")
        }
    }
    private let syncs = SyncRunner()
    var isSyncing: Bool { syncs.isRunning }
    /// False while the device has no network at all, as in airplane mode
    /// without Wi-Fi, so a sync says so at once instead of waiting on a
    /// request that can't go anywhere.
    private var networkAvailable = true
    /// Set when the server ended this installation's sign-in, for the
    /// sign-in screen to explain.
    private(set) var signedOutAccount: Account?

    init() {
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
                                     wifi: path.usesInterfaceType(.wifi) && !path.isExpensive && !path.isConstrained)
            }
        }
        // Observers don't run for what init sets.
        sessionChanged(from: nil)
        let device = Installation.deviceName
        Task {
            await Diagnostics.shared.log(.app, "launched \(Installation.appVersion) on \(device), iOS \(UIDevice.current.systemVersion)")
        }
    }

    /// A new session gets its own player, which picks up where the last
    /// launch left off; the old one stops, recording what was listened.
    private func sessionChanged(from old: AccountSession?) {
        guard old !== session else { return }
        player?.stop()
        player = nil
        showingPlayer = false
        libraryPath = []
        listsPath = []
        searchPath = []
        stopPrefetching()
        libraryID = session.map { UserDefaults.standard.integer(forKey: "libraryID.\($0.account.id)") } ?? 0
        if let session {
            let player = AudioPlayer(session: session) { [weak self] in
                Task { await self?.syncNow() }
            }
            self.player = player
            player.restore()
        }
    }

    /// Audio was playing when the app went to the background.
    @ObservationIgnored private var playingInBackground = false

    func didEnterBackground() {
        playingInBackground = player?.isPlaying == true
    }

    /// Coming back while the audio played on, as by tapping it on the lock
    /// screen or in Control Center, opens Now Playing, where its controls
    /// are. iOS doesn't say which of those brought the app back.
    func didBecomeActive() {
        if playingInBackground, player?.isPlaying == true, player?.file != nil {
            showingPlayer = true
        }
        playingInBackground = false
    }

    /// Closes Now Playing onto book's page in the tab showing, pushing it
    /// unless it's showing.
    func showBook(_ book: BookSummary) {
        showingPlayer = false
        let route = Route.book(book)
        switch tab {
        case .library where libraryPath.last != route: libraryPath.append(route)
        case .lists where listsPath.last != route: listsPath.append(route)
        case .search where searchPath.last != route: searchPath.append(route)
        default: break
        }
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
        let response: Shishoapp.SignInResponse
        do {
            response = try await client.signIn(username: username, password: password, deviceName: Installation.deviceName)
        } catch let error as APIError {
            throw SignInError(
                message: Messages.signIn(error), offersUpdate: Messages.isAppTooOld(error),
                offersSettings: error == .localNetworkDenied, retriesWhenActive: Messages.isUnreachable(error)
            )
        }

        let known = accounts.account(serverURL: address.url, userID: response.user.id)
        var account = known ?? Account(
            id: UUID().uuidString.lowercased(), serverURL: address.url, username: response.user.username,
            userID: response.user.id, deviceID: response.deviceId, serverVersion: response.serverVersion,
            thumbnailRenderKey: response.server.thumbnailRenderKey, capabilities: response.server.capabilities,
            signedOut: false
        )
        account.username = response.user.username
        account.deviceID = response.deviceId
        account.serverVersion = response.serverVersion
        account.thumbnailRenderKey = response.server.thumbnailRenderKey
        account.capabilities = response.server.capabilities
        account.signedOut = false
        do {
            try Keychain.setCredential(response.credential, for: account.id)
        } catch {
            // The Device exists on the server, but this installation couldn't
            // keep its credential, so it signs that Device out again.
            await APIClient(baseURL: address.url, credential: response.credential).signOut()
            throw SignInError(message: String(localized: "The app couldn't save your sign-in on this device. Try again."))
        }
        accounts.save(account)
        do {
            let opened = try openSession(account: account, credential: response.credential)
            // A copy kept from an earlier sign-in may be from a server since
            // reinstalled or restored at the same address, whose log has
            // reused its cursor. It downloads again from the start, and stays
            // on screen until that finishes.
            if known != nil {
                try opened.store.startOverIfNeeded(serverSaidSo: true)
            }
            // A sync of the account showing until now must not land on this
            // one, nor sign it out.
            syncs.stop()
            syncStatus = .idle
            session = opened
        } catch {
            throw SignInError(message: String(localized: "The app couldn't open its storage on this device. \(error.localizedDescription)"))
        }
        signedOutAccount = nil
        addingAccount = false
        await syncNow()
    }

    /// Signs this installation out of the current account: the server
    /// forgets the Device, and the app its credential. The account's copy of
    /// the catalog stays for signing in again.
    func signOut() async {
        guard let session else { return }
        // Everything here first: the server's answer can take a while, and a
        // sign-in meanwhile must not be undone when it comes.
        syncs.stop()
        Keychain.removeCredential(for: session.account.id)
        accounts.leaveCurrent()
        self.session = nil
        signedOutAccount = nil
        syncStatus = .idle
        // Another account still signed in takes over.
        if let next = accounts.accounts.first(where: { !$0.signedOut && $0.id != session.account.id && Keychain.credential(for: $0.id) != nil }) {
            switchAccount(to: next.id)
        }
        await session.client.signOut()
    }

    /// The server ended the sign-in of session's account. Its data stays,
    /// waiting for the same user to sign in again.
    private func handleSignedOut(_ ended: AccountSession) {
        guard ended === session else { return }
        syncs.stop()
        guard var account = session?.account else { return }
        account.signedOut = true
        Keychain.removeCredential(for: account.id)
        accounts.save(account)
        signedOutAccount = account
        session = nil
        syncStatus = .idle
    }

    // MARK: - Syncing

    /// Brings the catalog up to date. Failing to reach the server leaves the
    /// copy on the device as it is.
    func syncNow() async {
        guard let session else { return }
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
        do {
            try await session.sync.run()
            guard isCurrent() else { return }
            syncStatus = .idle
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
        case .noAccess:
            // The user's role can't read Books anymore: nothing in the copy
            // is theirs to see. The journal stays, to send if that changes.
            try? session.store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [], listIds: []), force: true)
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

    /// The account's session, whose responses keep its capabilities current.
    private func openSession(account: Account, credential: String) throws -> AccountSession {
        let accountID = account.id
        return try AccountSession(account: account, credential: credential) { [weak self] listed in
            Task { @MainActor in self?.recordCapabilities(listed, accountID: accountID) }
        }
    }

    // MARK: - Covers

    /// Whether to download every cover on Wi-Fi, kept for each account. Off
    /// until the user turns it on.
    func prefetchesCovers(_ account: Account) -> Bool {
        UserDefaults.standard.bool(forKey: "prefetchCovers.\(account.id)")
    }

    func setPrefetchesCovers(_ on: Bool) {
        guard let session else { return }
        UserDefaults.standard.set(on, forKey: "prefetchCovers.\(session.account.id)")
        if on {
            prefetchCoversIfWanted(session)
        } else {
            stopPrefetching()
        }
    }

    @ObservationIgnored private var onWiFi = false
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

    /// Downloads every cover the account's Library shows that isn't on the
    /// device yet, at the grid's size, while on Wi-Fi, so browsing offline
    /// shows them all.
    private func prefetchCoversIfWanted(_ session: AccountSession) {
        guard prefetchesCovers(session.account), onWiFi else { return }
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
                _ = await session.covers.image(bookID: cover.bookID, version: cover.version, size: size)
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
        let wasOnWiFi = onWiFi
        onWiFi = wifi
        if !wifi {
            stopPrefetching()
        } else if !wasOnWiFi, let session {
            prefetchCoversIfWanted(session)
        }
        guard available != networkAvailable else { return }
        networkAvailable = available
        if available {
            Task { await syncNow() }
        } else if session != nil {
            // The sync in flight can't finish; it would only time out.
            syncs.stop()
            syncStatus = .offline
        }
    }

    private func recordCapabilities(_ listed: [String], accountID: String) {
        guard var account = session?.account, account.id == accountID, account.capabilities != listed else { return }
        account.capabilities = listed
        accounts.save(account)
    }
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
    case library, lists, search
}

/// A page pushed on a tab's navigation.
enum Route: Hashable {
    case book(BookSummary)
    /// Every Series, Person, Genre, or Tag of the Library picked.
    case browse(ResourceKind)
    /// One Series, Person, Genre, or Tag, and its Books.
    case resource(ResourceKind, id: Int, name: String)
    case list(id: Int, name: String)
}
