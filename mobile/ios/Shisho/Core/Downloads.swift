import Foundation
import GRDB
import Observation
import os

/// The network a Download would use, as `DownloadRules` weighs it.
enum DownloadNetwork: Equatable, Sendable {
    case offline
    case wifi
    /// Cellular, or any network iOS calls expensive or constrained, such as
    /// a personal hotspot or Low Data Mode.
    case cellular
}

/// What a File's Download is doing, as its button and the Downloads tab say.
enum DownloadStatus: Equatable, Sendable {
    case notDownloaded
    /// Queued, and waiting for why.
    case waiting(DownloadRules.Wait)
    /// Transferring, with how far it's got from 0 to 1, or nil before the
    /// size is known.
    case running(Double?)
    case downloaded
    case failed(String)
}

/// When Downloads may transfer, as pure rules.
enum DownloadRules {
    enum Wait: Equatable, Sendable {
        /// Wi-Fi only is on and the device isn't on Wi-Fi.
        case wifi
        /// No network at all.
        case connection
        /// About to start.
        case turn
    }

    /// Why queued Downloads can't transfer now, or nil when they can.
    static func wait(wifiOnly: Bool, network: DownloadNetwork) -> Wait? {
        switch network {
        case .offline: .connection
        case .cellular where wifiOnly: .wifi
        default: nil
        }
    }

    /// A row's status, given what's running and how far each has got. A
    /// transfer iOS holds until there's a network it may use is waiting,
    /// not running.
    static func status(_ row: DownloadRow?, running: Bool, progress: Double?, wait: Wait?) -> DownloadStatus {
        guard let row else { return .notDownloaded }
        switch row.state {
        case .done: return .downloaded
        case .failed: return .failed(row.error ?? String(localized: "Try again."))
        case .queued:
            if let wait { return .waiting(wait) }
            return running ? .running(progress) : .waiting(.turn)
        }
    }

    /// Whether a server's answer is worth trying again later, as while a
    /// reverse proxy waits for Shisho to restart, rather than a refusal.
    static func retries(_ error: APIError) -> Bool {
        switch error {
        case .unreachable, .localNetworkDenied: true
        case .server(let status, _): status >= 500 || status == 408 || status == 429
        default: false
        }
    }
}

/// Which transfer is each File's own. An event from a transfer that was
/// replaced, as by removing a Download and downloading it again before the
/// first one's end arrives, never changes the one that replaced it.
struct Transfers<ID: Hashable> {
    /// How a transfer's end relates to its File.
    enum Ending: Equatable {
        /// The File's running transfer.
        case current
        /// One stopped on purpose, to carry on later.
        case paused
        /// One with nothing running for its File, as when iOS ended it
        /// because the app was force-quit and says so at the next launch.
        case orphan
        /// One replaced by the File's running transfer.
        case stale
    }

    private var running: [Int: ID] = [:]
    private var pausing: [ID: Int] = [:]

    var runningFiles: [Int] { Array(running.keys) }

    mutating func started(fileID: Int, id: ID) {
        running[fileID] = id
    }

    func isCurrent(fileID: Int, id: ID) -> Bool {
        running[fileID] == id
    }

    func id(fileID: Int) -> ID? {
        running[fileID]
    }

    /// Running, or stopped on purpose and not ended yet: either way, not to
    /// be started again.
    func busy(fileID: Int) -> Bool {
        running[fileID] != nil || pausing.values.contains(fileID)
    }

    /// Forgets the File's transfer, to cancel it for good.
    mutating func drop(fileID: Int) -> ID? {
        running.removeValue(forKey: fileID)
    }

    /// Stops the File's transfer, to carry on once its end arrives.
    mutating func pause(fileID: Int) -> ID? {
        guard let id = running.removeValue(forKey: fileID) else { return nil }
        pausing[id] = fileID
        return id
    }

    mutating func ended(fileID: Int, id: ID) -> Ending {
        if pausing.removeValue(forKey: id) != nil { return .paused }
        if running[fileID] == id {
            running[fileID] = nil
            return .current
        }
        return running[fileID] == nil ? .orphan : .stale
    }
}

/// One transfer, by its session and its task's identifier there, which
/// stay the same for the task's life, across launches too. A task object's
/// address can be reused once it's freed, while its last event is still on
/// its way.
struct TransferID: Hashable, Sendable {
    let session: String
    let task: Int

    init(_ task: URLSessionTask, in session: URLSession) {
        self.session = session.configuration.identifier ?? ""
        self.task = task.taskIdentifier
    }
}

/// The Download settings, for every account on this installation. Both are
/// off until the user turns them on.
enum DownloadSettings {
    private static let wifiOnlyKey = "downloads.wifiOnly"
    private static let removeWhenFinishedKey = "downloads.removeWhenFinished"

    static var wifiOnly: Bool {
        get { UserDefaults.standard.bool(forKey: wifiOnlyKey) }
        set { UserDefaults.standard.set(newValue, forKey: wifiOnlyKey) }
    }

    static var removeWhenFinished: Bool {
        get { UserDefaults.standard.bool(forKey: removeWhenFinishedKey) }
        set { UserDefaults.standard.set(newValue, forKey: removeWhenFinishedKey) }
    }
}

/// One account's Downloads (ADR 0022): Original Files kept on the device
/// until removed, so audiobooks play and ebooks open offline.
///
/// Transfers run in background URLSessions, so they go on outside the app,
/// carry on from where they stopped after an interruption, and are found
/// again after a relaunch. There are two: one that may use any network, and
/// one iOS keeps off cellular, so Wi-Fi only holds while the app is
/// suspended. The account's database says what's wanted and what's done;
/// how far a running transfer has got is kept here. Files, and what an
/// interrupted transfer carries on from, live in the account's folder in
/// Application Support, which iOS never clears, excluded from iCloud
/// backup, since each can be downloaded again.
@MainActor
@Observable
final class DownloadManager {
    /// How far each running transfer has got, from 0 to 1, by File.
    private(set) var progress: [Int: Double] = [:]
    /// The Files transferring now.
    private(set) var running: Set<Int> = []
    var wifiOnly = DownloadSettings.wifiOnly {
        didSet {
            guard wifiOnly != oldValue else { return }
            DownloadSettings.wifiOnly = wifiOnly
            moveToPolicySession()
        }
    }
    var removeWhenFinished = DownloadSettings.removeWhenFinished {
        didSet { DownloadSettings.removeWhenFinished = removeWhenFinished }
    }
    private(set) var network: DownloadNetwork = .wifi

    /// Why queued Downloads aren't transferring, or nil when they may.
    var wait: DownloadRules.Wait? { DownloadRules.wait(wifiOnly: wifiOnly, network: network) }

    nonisolated let folder: URL
    /// Each File's copy in the folder, put there and taken away one at a
    /// time with the transfers' delegate.
    nonisolated let copies: DownloadCopies
    @ObservationIgnored nonisolated private let store: CatalogStore
    @ObservationIgnored private let client: APIClient
    @ObservationIgnored private let accountID: String
    /// The session for any network, under false, and the one kept off
    /// cellular, under true.
    @ObservationIgnored private var sessions: [Bool: URLSession] = [:]
    /// Both sessions' transfers from before this launch have been found,
    /// so a File isn't started twice.
    @ObservationIgnored private var attached = false
    @ObservationIgnored private var foundTasks: [Bool: [URLSessionDownloadTask]] = [:]
    /// Events that arrived before then, handled once it's done.
    @ObservationIgnored private var early: [DownloadSessionDelegate.Event] = []
    @ObservationIgnored private var transfers = Transfers<TransferID>()
    @ObservationIgnored private var tasks: [TransferID: URLSessionDownloadTask] = [:]
    /// Which session each transfer is in: true for the one kept off
    /// cellular.
    @ObservationIgnored private var keptOffCellular: [TransferID: Bool] = [:]
    /// Transfers carried on from what an earlier one left, which start over
    /// if the server turns them down.
    @ObservationIgnored private var resumed: Set<TransferID> = []
    /// When each transfer this launch started began, so a late stop's resume
    /// data replaces only one that has barely started.
    @ObservationIgnored private var startedAt: [TransferID: Date] = [:]
    private static let freshTransfer: TimeInterval = 10
    /// Transfers that failed in a row without finishing, by File, so one
    /// that keeps failing stops being tried.
    @ObservationIgnored private var failures: [Int: Int] = [:]
    @ObservationIgnored private var retry: Task<Void, Never>?
    /// When a File that stopped may be tried again, so nothing that asks
    /// the queue to go on cuts its wait short.
    @ObservationIgnored private var notBefore: [Int: Date] = [:]
    /// Files paused on purpose whose resume data hasn't been kept yet, so
    /// they aren't started over meanwhile.
    @ObservationIgnored private var awaitingResume: Set<Int> = []
    /// Background launches waiting for their session's events, by session.
    @ObservationIgnored private var backgroundEventsDone: [String: [CheckedContinuation<Void, Never>]] = [:]
    /// Sessions whose background events were all handed over before
    /// anything waited for them.
    @ObservationIgnored private var backgroundEventsArrived: Set<String> = []
    @ObservationIgnored private var stopped = false
    /// Events from before the transfers were found are being handled.
    @ObservationIgnored private var replaying = false
    /// Called when a File's Download arrives or goes, so the player can
    /// switch to it or back to streaming.
    @ObservationIgnored var onChange: ((Int) -> Void)?

    /// The background session identifiers of an account: for any network,
    /// and kept off cellular.
    nonisolated static func sessionIdentifier(accountID: String, wifiOnly: Bool) -> String {
        (Bundle.main.bundleIdentifier ?? "com.shishobooks.shisho") + ".downloads." + accountID + (wifiOnly ? ".wifi" : "")
    }

    /// Whether identifier is one of an account's sessions.
    nonisolated static func isSession(_ identifier: String, accountID: String) -> Bool {
        identifier == sessionIdentifier(accountID: accountID, wifiOnly: false)
            || identifier == sessionIdentifier(accountID: accountID, wifiOnly: true)
    }

    init(account: Account, client: APIClient, store: CatalogStore) {
        self.client = client
        self.store = store
        accountID = account.id
        folder = account.folder.appendingPathComponent("Downloads", isDirectory: true)
        copies = DownloadCopies(store: store, folder: folder)
        // A book finished while the reader held it goes now.
        if let pending = try? store.pool.read(CatalogStore.removePending), !pending.isEmpty {
            remove(pending)
        }
        // One whose file is gone, as after restoring a backup, which leaves
        // Downloads out, downloads again. A read that fails leaves
        // everything as it is: an empty list would sweep every file away.
        guard let rows = try? store.pool.read({ try CatalogStore.downloads($0) }) else { return }
        let missing = rows.filter {
            $0.state == .done && !FileManager.default.fileExists(atPath: location(fileID: $0.fileID, fileType: $0.fileType).path)
        }
        try? store.requeueDownloads(missing.map(\.fileID))
        sweep(keeping: rows)
        // Transfers from before this launch are found again, and any still
        // wanted go on.
        if rows.contains(where: { $0.state == .queued }) || !missing.isEmpty {
            ensureSessions()
        }
    }

    /// Deletes files in the Downloads folder no row accounts for, as one
    /// whose removal was cut short, so their space isn't lost for good.
    private func sweep(keeping rows: [DownloadRow]) {
        let fileManager = FileManager.default
        let kept = Set(rows.map { "\($0.fileID).\($0.fileType)" })
        for name in (try? fileManager.contentsOfDirectory(atPath: folder.path)) ?? [] where name != "Resume" && !kept.contains(name) {
            try? fileManager.removeItem(at: folder.appendingPathComponent(name))
        }
        let queued = Set(rows.filter { $0.state == .queued }.map { "\($0.fileID).resume" })
        let resume = folder.appendingPathComponent("Resume", isDirectory: true)
        for name in (try? fileManager.contentsOfDirectory(atPath: resume.path)) ?? [] where !queued.contains(name) {
            try? fileManager.removeItem(at: resume.appendingPathComponent(name))
        }
    }

    // MARK: - Where Files are

    /// Where a File's Download is kept, whether or not it's there.
    nonisolated func location(fileID: Int, fileType: String) -> URL {
        copies.location(fileID: fileID, fileType: fileType)
    }

    /// The File's Download on the device, or nil without a finished one.
    /// A File the server replaced is queued again with its old copy still
    /// here, which is used until the new one arrives.
    nonisolated func localFile(fileID: Int) -> URL? {
        guard let row = try? store.pool.read({ try CatalogStore.download($0, fileID: fileID) }), row.state == .done || row.replacing else {
            return nil
        }
        let url = location(fileID: fileID, fileType: row.fileType)
        return FileManager.default.fileExists(atPath: url.path) ? url : nil
    }

    func status(_ row: DownloadRow?) -> DownloadStatus {
        DownloadRules.status(row, running: row.map { running.contains($0.fileID) } ?? false,
                             progress: row.flatMap { progress[$0.fileID] }, wait: wait)
    }

    // MARK: - Asking

    /// Downloads fileIDs, skipping any already downloaded or on their way.
    func download(_ fileIDs: [Int]) {
        do {
            let queued = try store.queueDownloads(fileIDs)
            for fileID in queued { failures[fileID] = nil }
            if !queued.isEmpty { log("queued \(queued.map { "file \($0)" }.joined(separator: ", "))") }
        } catch {
            log("couldn't queue \(fileIDs): \(Diagnostics.describe(error))")
        }
        pump()
    }

    /// Downloads a File now, in front of the user, as opening an ebook that
    /// isn't on the device does: the reader needs all of it before it can
    /// show a page. It becomes a Download like any other, listed, removed by
    /// the user, and by Remove when finished. A background transfer of it
    /// already under way isn't needed after this. fetched is the copy this
    /// call put there, which only it may discard (`discard`), or nil for one
    /// it found on the device.
    func fetchNow(fileID: Int) async throws -> (url: URL, fetched: UUID?) {
        if let local = localFile(fileID: fileID) { return (local, nil) }
        let fetched = try await client.download(fileID: fileID)
        defer { try? FileManager.default.removeItem(at: fetched) }
        // A background transfer of it may have finished meanwhile.
        if let local = localFile(fileID: fileID) { return (local, nil) }
        try store.queueDownloads([fileID])
        // One finished whose file is gone counts as wanted again.
        try store.requeueDownloads([fileID])
        guard let row = try await store.pool.read({ try CatalogStore.download($0, fileID: fileID) }) else {
            throw APIError.server(status: 404, message: String(localized: "The server doesn't have this File anymore."))
        }
        if let id = transfers.drop(fileID: fileID) {
            tasks.removeValue(forKey: id)?.cancel()
        }
        running.remove(fileID)
        progress[fileID] = nil
        DownloadResume.drop(fileID: fileID, in: folder)
        guard let copy = try copies.publish(fileID: fileID, from: fetched) else {
            // A background transfer finishing at the same moment put the
            // same File there first, or it was removed meanwhile.
            if let local = localFile(fileID: fileID) { return (local, nil) }
            throw CancellationError()
        }
        log("file \(fileID): downloaded to open it")
        onChange?(fileID)
        return (location(fileID: fileID, fileType: row.fileType), copy)
    }

    /// Stops a transfer and removes Downloads, freeing their space.
    func remove(_ fileIDs: [Int]) {
        guard !fileIDs.isEmpty else { return }
        let rows = (try? store.pool.read { try CatalogStore.downloads($0) }) ?? []
        do {
            try store.deleteDownloads(fileIDs)
        } catch {
            log("couldn't remove \(fileIDs): \(Diagnostics.describe(error))")
            return
        }
        for fileID in fileIDs {
            if let row = rows.first(where: { $0.fileID == fileID }) {
                try? FileManager.default.removeItem(at: location(fileID: fileID, fileType: row.fileType))
            }
            forget(fileID)
        }
        log("removed \(fileIDs.map { "file \($0)" }.joined(separator: ", "))")
    }

    /// Stops a removed Download's transfer and forgets what it had done.
    private func forget(_ fileID: Int) {
        if let id = transfers.drop(fileID: fileID) {
            tasks.removeValue(forKey: id)?.cancel()
        }
        running.remove(fileID)
        progress[fileID] = nil
        failures[fileID] = nil
        DownloadResume.drop(fileID: fileID, in: folder)
        onChange?(fileID)
    }

    /// Removes a copy an open downloaded (`fetchNow`) that won't open, so
    /// Try Again downloads it again. Only that copy: a newer one a transfer
    /// put in its place meanwhile stays, and a replacement on its way keeps
    /// going.
    func discard(fileID: Int, copy: UUID) {
        let discarded: DownloadCopies.Discarded
        do {
            discarded = try copies.discard(fileID: fileID, copy: copy)
        } catch {
            log("couldn't remove file \(fileID)'s copy: \(Diagnostics.describe(error))")
            return
        }
        switch discarded {
        case .download:
            log("file \(fileID): its copy won't open, so its Download is removed")
            forget(fileID)
        case .copy:
            log("file \(fileID): its copy won't open, so it waits for the new one")
            onChange?(fileID)
        case .nothing:
            break
        }
    }

    func removeAll() {
        let all = (try? store.pool.read { try CatalogStore.downloads($0) })?.map(\.fileID) ?? []
        remove(all)
    }

    /// A File was listened to or read to its end, which finishes its
    /// Read-through: its Download goes when the user asked for that. One
    /// still open, as in the reader, which reads it as pages open, is
    /// marked to go once it's let go (`released`), or at the next launch.
    func finished(fileID: Int, holding: Bool = false) {
        guard removeWhenFinished, localFile(fileID: fileID) != nil,
              (try? store.pool.read({ try CatalogStore.isSupplement($0, fileID: fileID) })) == false
        else { return }
        if holding {
            try? store.markRemovePending(fileID: fileID)
            return
        }
        log("file \(fileID): finished, so its Download is removed")
        remove([fileID])
    }

    /// The reader went back from the end, which undoes an automatic finish,
    /// so a removal waiting for it to let go is called off.
    func unfinished(fileID: Int) {
        try? store.clearRemovePending(fileID: fileID)
    }

    /// The reader let go of a File: a Download finished meanwhile goes now.
    func released(fileID: Int) {
        guard let pending = try? store.pool.read(CatalogStore.removePending), pending.contains(fileID) else { return }
        log("file \(fileID): finished, so its Download is removed")
        remove([fileID])
    }

    /// Removes Downloads whose File left the catalog copy: deleted on the
    /// server, or in a Library the user can't access anymore. It runs once
    /// a sync has caught up, never between its pages, where a File moving
    /// between Books can be out of both for a moment.
    func dropOrphans() {
        // A Library gained and not fetched yet may hold a File that moved.
        guard let pending = try? store.pendingAccess(), pending.libraries.isEmpty, pending.lists.isEmpty,
              let gone = try? store.pool.read(CatalogStore.orphanedDownloads), !gone.isEmpty
        else { return }
        log("\(gone.map { "file \($0)" }.joined(separator: ", ")) left the library")
        remove(gone)
    }

    /// Downloads a File again once the server's copy was replaced, which the
    /// catalog shows as a new size, so the device doesn't keep a stale one.
    /// Until the new one arrives, the File streams or opens from the
    /// server, as one not downloaded does.
    func refreshReplaced() {
        guard let replaced = try? store.requeueReplaced(), !replaced.isEmpty else { return }
        log("\(replaced.map { "file \($0)" }.joined(separator: ", ")) changed on the server, so it downloads again")
        for fileID in replaced {
            failures[fileID] = nil
            onChange?(fileID)
        }
        pump()
    }

    // MARK: - Network

    /// The device's network changed. Transfers are left to iOS, which holds
    /// one kept off cellular until there's Wi-Fi, and any until there's a
    /// network, then carries on from where it stopped; queued ones start
    /// once the network allows.
    func networkChanged(to network: DownloadNetwork) {
        guard network != self.network else { return }
        self.network = network
        // A new network is a fresh start for transfers that kept stopping
        // on the last one.
        failures = [:]
        notBefore = [:]
        pump()
    }

    /// Tries the queue again, as when the app comes back to the front.
    func resume() {
        backgroundEventsArrived = []
        pump()
    }

    // MARK: - Transfers

    private func ensureSessions() {
        guard sessions.isEmpty, !stopped else { return }
        let sink = DownloadSessionDelegate.Sink(owner: ObjectIdentifier(self), copies: copies) { [weak self] event in
            Task { @MainActor in self?.receive(event) }
        }
        for wifiOnly in [false, true] {
            let session = DownloadSessions.shared.session(
                identifier: DownloadManager.sessionIdentifier(accountID: accountID, wifiOnly: wifiOnly), sink: sink
            ) { [client] identifier, delegate in
                client.backgroundSession(identifier: identifier, wifiOnly: wifiOnly, delegate: delegate)
            }
            sessions[wifiOnly] = session
            session.getAllTasks { [weak self] tasks in
                let downloads = tasks.compactMap { $0 as? URLSessionDownloadTask }
                Task { @MainActor in self?.found(downloads, keptOffCellular: wifiOnly) }
            }
        }
    }

    /// One session's transfers from before this launch; both are taken in
    /// once both are known.
    private func found(_ downloads: [URLSessionDownloadTask], keptOffCellular: Bool) {
        foundTasks[keptOffCellular] = downloads
        if foundTasks.count == 2 {
            attach(foundTasks)
            foundTasks = [:]
        }
    }

    /// Takes in the transfers iOS kept running from before this launch, and
    /// handles what arrived meanwhile.
    private func attach(_ found: [Bool: [URLSessionDownloadTask]]) {
        // A read that fails keeps every transfer, rather than cancelling
        // them all as unwanted.
        let rows = try? store.pool.read { try CatalogStore.downloads($0) }
        let queued = rows.map { Set($0.filter { $0.state == .queued }.map(\.fileID)) }
        for (offCellular, task) in found.flatMap({ kind, tasks in tasks.map { (kind, $0) } }) {
            guard let fileID = task.taskDescription.flatMap(Int.init), queued?.contains(fileID) ?? true,
                  !transfers.busy(fileID: fileID), let session = sessions[offCellular] else {
                task.cancel()
                continue
            }
            let id = TransferID(task, in: session)
            transfers.started(fileID: fileID, id: id)
            tasks[id] = task
            keptOffCellular[id] = offCellular
            running.insert(fileID)
            if task.countOfBytesExpectedToReceive > 0 {
                progress[fileID] = Double(task.countOfBytesReceived) / Double(task.countOfBytesExpectedToReceive)
            }
        }
        let count = found.values.reduce(0) { $0 + $1.count }
        if count > 0 { log("found \(count) transfers from before") }
        attached = true
        let waiting = early
        early = []
        // A force-quit's transfers end at this launch, each with what to
        // carry on from. They're all kept before any starts again: one
        // started early starts over, its resume data arriving stale.
        replaying = true
        for event in waiting { handle(event) }
        replaying = false
        // One in the session the setting doesn't ask for moves.
        moveToPolicySession()
    }

    /// Starts every queued Download that isn't running, when the network
    /// allows, from what an interrupted transfer left when there is some.
    private func pump() {
        guard !stopped, !replaying else { return }
        let rows = ((try? store.pool.read { try CatalogStore.downloads($0) }) ?? []).filter { $0.state == .queued }
        guard !rows.isEmpty else { return }
        // A Download goes to iOS even while it waits for Wi-Fi or a
        // connection, so it starts once there's one, with the app suspended.
        ensureSessions()
        guard attached, let session = sessions[wifiOnly] else { return }
        let now = Date()
        // The queue goes on when the soonest File waiting to try again may.
        let waits = rows.compactMap { notBefore[$0.fileID] }.filter { $0 > now }
        if let soonest = waits.min() {
            scheduleRetry(after: soonest.timeIntervalSince(now))
        }
        for row in rows where !transfers.busy(fileID: row.fileID) && !awaitingResume.contains(row.fileID)
            && (notBefore[row.fileID].map { $0 <= now } ?? true) {
            let task: URLSessionDownloadTask
            if let resumeData = DownloadResume.take(fileID: row.fileID, in: folder) {
                task = session.downloadTask(withResumeData: resumeData)
                resumed.insert(TransferID(task, in: session))
            } else {
                task = session.downloadTask(with: client.downloadRequest(fileID: row.fileID))
            }
            task.taskDescription = String(row.fileID)
            task.countOfBytesClientExpectsToReceive = Int64(row.sizeBytes)
            let id = TransferID(task, in: session)
            startedAt[id] = Date()
            transfers.started(fileID: row.fileID, id: id)
            tasks[id] = task
            keptOffCellular[id] = wifiOnly
            running.insert(row.fileID)
            task.resume()
        }
    }

    /// Stops transfers in the session the setting doesn't ask for, to carry
    /// on from where they are in the one it does: one that may use cellular
    /// would otherwise go on doing so after Wi-Fi only is turned on.
    private func moveToPolicySession() {
        if attached {
            for fileID in transfers.runningFiles {
                guard let id = transfers.id(fileID: fileID), keptOffCellular[id] != wifiOnly else { continue }
                pause(fileID: fileID)
            }
        }
        pump()
    }

    /// Stops a File's transfer where it is, keeping what to carry on from.
    private func pause(fileID: Int) {
        guard let id = transfers.pause(fileID: fileID), let task = tasks[id] else { return }
        running.remove(fileID)
        progress[fileID] = nil
        awaitingResume.insert(fileID)
        let folder = folder
        task.cancel(byProducingResumeData: { [weak self] data in
            if let data { DownloadResume.save(data, fileID: fileID, in: folder) }
            Task { @MainActor in
                self?.awaitingResume.remove(fileID)
                self?.pump()
            }
        })
    }

    /// Events wait until the transfers from before this launch are known,
    /// so the end of a background launch is acknowledged after them.
    private func receive(_ event: DownloadSessionDelegate.Event) {
        if attached {
            handle(event)
        } else {
            early.append(event)
        }
    }

    private func handle(_ event: DownloadSessionDelegate.Event) {
        switch event {
        case .progress(let fileID, let id, let fraction):
            guard transfers.isCurrent(fileID: fileID, id: id) else { return }
            progress[fileID] = fraction
        case .finished(let fileID, let id, let kept):
            let ending = transfers.ended(fileID: fileID, id: id)
            settle(fileID: fileID, id: id, ending: ending)
            failures[fileID] = nil
            notBefore[fileID] = nil
            if kept {
                log("file \(fileID): downloaded")
                // A replaced transfer finished first: the File is here, so
                // its replacement isn't needed.
                if let current = transfers.drop(fileID: fileID) {
                    tasks.removeValue(forKey: current)?.cancel()
                    running.remove(fileID)
                    progress[fileID] = nil
                }
                DownloadResume.drop(fileID: fileID, in: folder)
                onChange?(fileID)
            }
            pump()
        case .refused(let fileID, let id, let error):
            let ending = transfers.ended(fileID: fileID, id: id)
            let wasResumed = resumed.contains(id)
            settle(fileID: fileID, id: id, ending: ending)
            guard ending == .current || ending == .orphan else { return }
            if wasResumed {
                // What it carried on from holds the sign-in it started with,
                // which may have ended since; a fresh start uses today's.
                log("file \(fileID): the server refused carrying on, so it starts over: \(Diagnostics.describe(error))")
                pump()
                return
            }
            log("file \(fileID): the server refused the download: \(Diagnostics.describe(error))")
            try? store.markDownloadFailed(fileID: fileID, message: DownloadManager.refusal(error))
        case .stopped(let fileID, let id, let error, let resumeData):
            let ending = transfers.ended(fileID: fileID, id: id)
            settle(fileID: fileID, id: id, ending: ending)
            if ending == .stale {
                // A transfer from before this launch, ended late, after
                // one started over from nothing moments ago: it carries on
                // from here instead. One that has had longer may be further
                // along, and goes on.
                if let resumeData, let current = transfers.id(fileID: fileID), !resumed.contains(current),
                   startedAt[current].map({ Date().timeIntervalSince($0) < DownloadManager.freshTransfer }) == true {
                    _ = transfers.drop(fileID: fileID)
                    tasks.removeValue(forKey: current)?.cancel()
                    settle(fileID: fileID, id: current, ending: .current)
                    DownloadResume.save(resumeData, fileID: fileID, in: folder)
                    pump()
                }
                return
            }
            if let resumeData {
                DownloadResume.save(resumeData, fileID: fileID, in: folder)
            }
            guard ending == .current else {
                // Paused on purpose, or ended by iOS before this launch.
                pump()
                return
            }
            let count = (failures[fileID] ?? 0) + 1
            failures[fileID] = count
            log("file \(fileID): stopped after \(count) tries, \(resumeData == nil ? "from the start" : "resumable"): \(Diagnostics.describe(error))")
            if count >= 5 {
                try? store.markDownloadFailed(fileID: fileID, message: String(localized: "It kept stopping. Check your connection and try again."))
            } else {
                let delay = Double(count * count) * 5
                notBefore[fileID] = Date().addingTimeInterval(delay)
                pump()
            }
        case .backgroundEventsFinished(let identifier):
            let waiting = backgroundEventsDone.removeValue(forKey: identifier) ?? []
            if waiting.isEmpty { backgroundEventsArrived.insert(identifier) }
            for continuation in waiting { continuation.resume() }
        }
    }

    /// Forgets a transfer that ended.
    private func settle(fileID: Int, id: TransferID, ending: Transfers<TransferID>.Ending) {
        tasks[id] = nil
        startedAt[id] = nil
        keptOffCellular[id] = nil
        resumed.remove(id)
        if ending == .current {
            running.remove(fileID)
            progress[fileID] = nil
        }
    }

    /// Why the server turned a Download down, as its File says.
    private static func refusal(_ error: APIError) -> String {
        switch error {
        case .server(let status, _) where status == 404:
            String(localized: "The server doesn't have this File anymore.")
        case .server(let status, _) where (300..<400).contains(status):
            String(localized: "The server sent it to another address, which the app doesn't follow with your sign-in.")
        default:
            Messages.signIn(error)
        }
    }

    private func scheduleRetry(after seconds: Double) {
        retry?.cancel()
        retry = Task { [weak self] in
            try? await Task.sleep(for: .seconds(seconds))
            guard !Task.isCancelled else { return }
            self?.pump()
        }
    }

    // MARK: - Relaunches and switching

    /// Waits until iOS has handed over every event of a background launch
    /// for this account's sessions.
    func backgroundEvents(identifier: String) async {
        ensureSessions()
        if backgroundEventsArrived.remove(identifier) != nil || stopped {
            return
        }
        await withCheckedContinuation { backgroundEventsDone[identifier, default: []].append($0) }
    }

    /// Another account takes over: every transfer stops where it is, to
    /// carry on when this account is back. The sessions stay open for the
    /// process (`DownloadSessions`), so coming back never makes a second one
    /// with the same identifier.
    func stop() {
        stopped = true
        retry?.cancel()
        for continuation in backgroundEventsDone.values.joined() { continuation.resume() }
        backgroundEventsDone = [:]
        // Signed in to the same account again, whose new manager has taken
        // the sessions over: its transfers are its own.
        let owner = ObjectIdentifier(self)
        guard sessions.keys.allSatisfy({
            DownloadSessions.shared.owner(of: DownloadManager.sessionIdentifier(accountID: accountID, wifiOnly: $0)) == owner
        }) else { return }
        for fileID in transfers.runningFiles {
            pause(fileID: fileID)
        }
        let folder = folder
        // Transfers not taken in yet, and any iOS started meanwhile, unless
        // a new manager has taken the session over by the time they're known.
        for session in sessions.values {
            let identifier = session.configuration.identifier ?? ""
            session.getAllTasks { tasks in
                guard DownloadSessions.shared.owner(of: identifier) == owner else { return }
                for task in tasks {
                    guard let download = task as? URLSessionDownloadTask, let fileID = task.taskDescription.flatMap(Int.init) else {
                        task.cancel()
                        continue
                    }
                    download.cancel(byProducingResumeData: { data in
                        if let data { DownloadResume.save(data, fileID: fileID, in: folder) }
                    })
                }
            }
        }
    }

    // MARK: - Space

    /// The bytes every finished Download takes.
    nonisolated static func totalSize(_ downloads: [StoredDownload]) -> Int {
        downloads.filter { $0.state == .done }.reduce(0) { $0 + $1.sizeBytes }
    }

    private func log(_ message: String) {
        Task { await Diagnostics.shared.log(.downloads, message) }
    }
}

/// Each File's copy in an account's Downloads folder, at one place per
/// File. A copy is put there and taken away inside a write to the account's
/// database, and its writes run one at a time, so the row and the file
/// change together: an open discarding the copy it downloaded can't take a
/// newer one a transfer puts there between its check and its delete. Used
/// from the main actor and from the transfers' delegate.
struct DownloadCopies: Sendable {
    let store: CatalogStore
    let folder: URL

    /// What discarding a copy did.
    enum Discarded: Equatable, Sendable {
        /// The Download went with it.
        case download
        /// Only the copy went: the Download is on its way, as for a File
        /// the server replaced, and its transfer goes on.
        case copy
        /// Another copy is there now, or none, and it stays.
        case nothing
    }

    /// Where a File's copy is kept, whether or not it's there.
    func location(fileID: Int, fileType: String) -> URL {
        folder.appendingPathComponent("\(fileID).\(fileType)")
    }

    /// Moves a downloaded file into the File's place as its new copy, and
    /// returns which copy it is. Nil, leaving the file where it is, when the
    /// Download isn't on its way: removed meanwhile, or finished by another
    /// transfer, whose copy of the same File stays.
    func publish(fileID: Int, from file: URL) throws -> UUID? {
        try store.pool.write { db in
            guard let row = try CatalogStore.download(db, fileID: fileID), row.state == .queued else { return nil }
            try DownloadSessionDelegate.prepare(folder)
            let destination = location(fileID: fileID, fileType: row.fileType)
            try? FileManager.default.removeItem(at: destination)
            try FileManager.default.moveItem(at: file, to: destination)
            let size = (try? FileManager.default.attributesOfItem(atPath: destination.path)[.size] as? Int) ?? row.sizeBytes
            return try CatalogStore.markDownloaded(db, fileID: fileID, sizeBytes: size)
        }
    }

    /// Deletes copy if it's still the one in the File's place. A finished
    /// Download goes with it; one on its way, as a File's replacement, stays
    /// on its way without the copy.
    func discard(fileID: Int, copy: UUID) throws -> Discarded {
        try store.pool.write { db in
            guard let row = try CatalogStore.download(db, fileID: fileID), row.copy == copy else { return .nothing }
            try CatalogStore.discardCopy(db, fileID: fileID, finished: row.state == .done)
            try? FileManager.default.removeItem(at: location(fileID: fileID, fileType: row.fileType))
            return row.state == .done ? .download : .copy
        }
    }
}

/// What an interrupted transfer carries on from, one file per File in the
/// Downloads folder. It holds the request, sign-in included, so it stays
/// out of the database and out of backups.
enum DownloadResume {
    private static func url(fileID: Int, in folder: URL) -> URL {
        folder.appendingPathComponent("Resume", isDirectory: true).appendingPathComponent("\(fileID).resume")
    }

    static func save(_ data: Data, fileID: Int, in folder: URL) {
        let url = url(fileID: fileID, in: folder)
        try? DownloadSessionDelegate.prepare(folder)
        try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: false)
        try? data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }

    /// The File's, used once: if it's no good, the next try starts over.
    static func take(fileID: Int, in folder: URL) -> Data? {
        let url = url(fileID: fileID, in: folder)
        defer { try? FileManager.default.removeItem(at: url) }
        return try? Data(contentsOf: url)
    }

    static func drop(fileID: Int, in folder: URL) {
        try? FileManager.default.removeItem(at: url(fileID: fileID, in: folder))
    }
}

/// Every background session this process opened, by identifier, kept open
/// for its life. iOS allows one session per identifier, so an account
/// switched away from and back to, or signed in to again, reuses its
/// sessions, pointed at its current manager.
final class DownloadSessions: Sendable {
    static let shared = DownloadSessions()

    private struct Entry: Sendable {
        let session: URLSession
        let delegate: DownloadSessionDelegate
    }

    private let open = OSAllocatedUnfairLock<[String: Entry]>(initialState: [:])

    func session(identifier: String, sink: DownloadSessionDelegate.Sink,
                 make: @escaping @Sendable (String, DownloadSessionDelegate) -> URLSession) -> URLSession {
        open.withLock { open in
            if let entry = open[identifier] {
                entry.delegate.point(at: sink)
                return entry.session
            }
            let delegate = DownloadSessionDelegate(sink: sink)
            let session = make(identifier, delegate)
            open[identifier] = Entry(session: session, delegate: delegate)
            return session
        }
    }

    /// The manager a session's events go to now.
    func owner(of identifier: String) -> ObjectIdentifier? {
        open.withLock { $0[identifier]?.delegate.owner }
    }

}

/// Receives a background session's events, off the main actor. A finished
/// transfer has to be moved before this returns, since iOS deletes it
/// after, so that part happens here; the rest goes to the manager.
final class DownloadSessionDelegate: NSObject, URLSessionDownloadDelegate, Sendable {
    enum Event: Sendable {
        case progress(Int, TransferID, Double)
        /// kept is false when the Download was removed while it ran.
        case finished(Int, TransferID, kept: Bool)
        /// The server answered with an error that trying again won't fix.
        case refused(Int, TransferID, APIError)
        /// The transfer stopped before it finished, with what to carry on
        /// from.
        case stopped(Int, TransferID, Error, Data?)
        /// iOS handed over every event of a background launch for the
        /// session named.
        case backgroundEventsFinished(String)
    }

    /// Where events go: the account's Downloads, and its manager.
    struct Sink: Sendable {
        let owner: ObjectIdentifier
        let copies: DownloadCopies
        let send: @Sendable (Event) -> Void
    }

    private let sink: OSAllocatedUnfairLock<Sink>

    init(sink: Sink) {
        self.sink = OSAllocatedUnfairLock(initialState: sink)
    }

    func point(at sink: Sink) {
        self.sink.withLock { $0 = sink }
    }

    var owner: ObjectIdentifier { current.owner }

    private var current: Sink { sink.withLock { $0 } }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard let fileID = downloadTask.taskDescription.flatMap(Int.init) else { return }
        let id = TransferID(downloadTask, in: session)
        let sink = current
        do {
            try DownloadSessionDelegate.checkOrigin(downloadTask)
            try APIClient.checkDownloaded(location, response: downloadTask.response)
        } catch let error as APIError {
            sink.send(DownloadRules.retries(error) ? .stopped(fileID, id, error, nil) : .refused(fileID, id, error))
            return
        } catch {
            sink.send(.stopped(fileID, id, error, nil))
            return
        }
        do {
            // Not kept when it was removed meanwhile, or finished another
            // way, as by opening it in the reader.
            let kept = try sink.copies.publish(fileID: fileID, from: location) != nil
            sink.send(.finished(fileID, id, kept: kept))
        } catch {
            sink.send(.stopped(fileID, id, error, nil))
        }
    }

    /// A background session follows every redirect itself, without asking
    /// its delegate, so a download that ended up anywhere `RedirectPolicy`
    /// wouldn't follow isn't the File.
    static func checkOrigin(_ task: URLSessionTask) throws {
        guard let asked = task.originalRequest?.url, let answered = task.response?.url,
              asked.host() != answered.host() || asked.scheme != answered.scheme || asked.port != answered.port,
              !RedirectPolicy.allows(from: asked, to: answered)
        else { return }
        throw APIError.server(status: 307, message: "redirected to \(answered.host() ?? "another server")")
    }

    /// Makes the Downloads folder, kept out of iCloud backup: its Files can
    /// all be downloaded again. Only inside the account's folder: an account
    /// removed while a transfer ended, as for a server set up again, has its
    /// folder deleted, and this mustn't make it again.
    static func prepare(_ folder: URL) throws {
        guard !FileManager.default.fileExists(atPath: folder.path) else { return }
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: false)
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        var folder = folder
        try folder.setResourceValues(values)
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64,
                    totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        guard let fileID = downloadTask.taskDescription.flatMap(Int.init), totalBytesExpectedToWrite > 0 else { return }
        // Only each whole percent, so a fast transfer doesn't flood the
        // main actor.
        let before = (totalBytesWritten - bytesWritten) * 100 / totalBytesExpectedToWrite
        let now = totalBytesWritten * 100 / totalBytesExpectedToWrite
        guard now != before else { return }
        current.send(.progress(fileID, TransferID(downloadTask, in: session), Double(totalBytesWritten) / Double(totalBytesExpectedToWrite)))
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        guard let error, let fileID = task.taskDescription.flatMap(Int.init) else { return }
        let resumeData = (error as NSError).userInfo[NSURLSessionDownloadTaskResumeData] as? Data
        current.send(.stopped(fileID, TransferID(task, in: session), error, resumeData))
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        current.send(.backgroundEventsFinished(session.configuration.identifier ?? ""))
    }
}
