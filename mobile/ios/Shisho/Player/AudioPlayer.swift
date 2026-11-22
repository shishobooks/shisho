import AVFoundation
import GRDB
import MediaPlayer
import Observation
import UIKit

/// Plays one account's audiobooks, and keeps the user's place and time.
///
/// Everything that tracks listening lives here, not in a view: it keeps
/// running with the phone locked and with no screen open. The place is
/// saved every five seconds of playback and on pause, sittings are timed
/// by `ListeningRules`, and both go into the account's journal for the
/// next sync (ADR 0024). The lock screen and Control Center show the
/// chapter's real length and the playback rate, so progress moves faster at
/// speed rather than the length shrinking.
@MainActor
@Observable
final class AudioPlayer {
    /// How far the skip buttons go.
    static let skipInterval: Double = 30

    private(set) var file: ListeningFile?
    /// Playback is wanted: playing, or about to once loaded.
    private(set) var isPlaying = false
    /// Waiting for the stream before it can play.
    private(set) var isBuffering = false
    private(set) var position: Double = 0
    private(set) var rate: Double = 1
    private(set) var error: String?
    /// Another device's place, offered without jumping (#912).
    private(set) var offer: ServerPlace?
    /// The cover, for the lock screen and the tint.
    private(set) var artwork: UIImage?

    var chapterIndex: Int? {
        file.flatMap { AudioChapters.index(at: position, in: $0.chapters) }
    }

    var chapterTitle: String? {
        guard let file, let index = chapterIndex else { return nil }
        return file.chapters[index].title
    }

    /// Where the current chapter starts and ends, or the whole File without
    /// chapters.
    var chapterBounds: (start: Double, end: Double) {
        guard let file else { return (0, 0) }
        guard let index = chapterIndex else { return (0, file.duration) }
        return (file.chapters[index].start, AudioChapters.end(of: index, in: file.chapters, duration: file.duration))
    }

    @ObservationIgnored private let session: AccountSession
    @ObservationIgnored private let requestSync: @MainActor () -> Void
    @ObservationIgnored private var player: AVPlayer?
    @ObservationIgnored private var statusObservation: NSKeyValueObservation?
    @ObservationIgnored private var ready = false
    @ObservationIgnored private var pendingSeek: Double?
    /// Where a seek under way is going: the position until it lands, so the
    /// bar doesn't flick back to where it was.
    @ObservationIgnored private var seekingTo: Double?
    /// Counts seeks, so a seek's completion clears seekingTo only when no
    /// later one replaced it, even one to the same place.
    @ObservationIgnored private var seekCount = 0
    /// The seek the last failure came after, so each seek's is logged.
    @ObservationIgnored private var stoppedSeek = 0
    @ObservationIgnored private var sitting: ListeningSitting?
    @ObservationIgnored private var lastSave = Date.distantPast
    /// When the server's place was from that the player last knew of, at
    /// opening or applying it: a later one from another device is offered.
    @ObservationIgnored private var knownServerAt: Date?
    @ObservationIgnored private var ticker: Task<Void, Never>?
    @ObservationIgnored private var placeWatch: Task<Void, Never>?
    @ObservationIgnored private var fileWatch: Task<Void, Never>?
    @ObservationIgnored private var observers: [NSObjectProtocol] = []
    @ObservationIgnored private var resumeAfterInterruption = false
    @ObservationIgnored private var nowPlayingChapter: Int?
    @ObservationIgnored private var dismissedOffer: ServerPlace?
    /// Which item AVFoundation's callbacks are about: a callback queued
    /// before another File opened is dropped.
    @ObservationIgnored private var itemGeneration = 0
    @ObservationIgnored private var endObserver: NSObjectProtocol?
    @ObservationIgnored private var errorLogObserver: NSObjectProtocol?
    @ObservationIgnored private var failureObserver: NSObjectProtocol?
    /// When playback that's wanted began waiting for the stream, and whether
    /// the wait was long enough to log.
    @ObservationIgnored private var waitingSince: Date?
    @ObservationIgnored private var waitingLogged = false
    /// When the stream last stopped playing with an error while the item
    /// stayed ready, and whether play was tried again since.
    @ObservationIgnored private var stoppedAt: Date?
    @ObservationIgnored private var retriedStop = false
    /// When the player played again after a failure, until it plays.
    @ObservationIgnored private var retriedAt: Date?

    init(session: AccountSession, requestSync: @escaping @MainActor () -> Void) {
        self.session = session
        self.requestSync = requestSync
        observeSystem()
        setUpRemoteCommands()
    }

    private var lastFileKey: String { "player.lastFile.\(session.account.id)" }

    /// Picks up where the last launch left off: a sitting it was killed
    /// during becomes a Reading Session, and the last File is loaded,
    /// paused, at its place.
    func restore() {
        do {
            if let left = try session.store.openSitting() {
                let finished = ListeningRules.finish(left)
                try session.store.saveSitting(nil, recording: finished)
                if let finished {
                    Task { await Diagnostics.shared.log(.player, "file \(left.fileID): sitting from an earlier launch recorded, \(finished.duration) s") }
                }
            }
        } catch {
            log("couldn't recover the last sitting: \(Diagnostics.describe(error))")
        }
        let last = UserDefaults.standard.integer(forKey: lastFileKey)
        if last != 0 {
            open(fileID: last, autoplay: false)
        }
    }

    // MARK: - Loading

    /// Opens an audiobook File at its place (`PlaceRules.open`), playing when
    /// autoplay. Opening the File already loaded only plays it. Nothing is
    /// streamed until it plays.
    func open(fileID: Int, autoplay: Bool) {
        if file?.fileID == fileID {
            if sitting == nil, !isPlaying {
                applyNewestPlace()
            }
            if autoplay { play() }
            return
        }
        let store = session.store
        let ownDevice = session.account.deviceID
        // The File is found before the one playing is touched, so a File
        // that's gone, such as one a sync just removed, leaves it playing.
        guard let loaded = try? store.pool.read({ try CatalogStore.listeningFile($0, fileID: fileID) }) else {
            if file == nil {
                UserDefaults.standard.removeObject(forKey: lastFileKey)
            }
            return
        }
        if file != nil {
            pause()
            teardownItem()
        }
        let places = try? store.pool.read { db in
            (local: try CatalogStore.localPlace(db, fileID: fileID), server: try CatalogStore.serverPlace(db, fileID: fileID))
        }
        let choice = PlaceRules.open(local: places?.local, server: places?.server, ownDeviceID: ownDevice)

        file = loaded
        error = nil
        offer = choice.offer
        dismissedOffer = nil
        knownServerAt = places?.server?.occurredAt
        artwork = nil
        rate = storedRate(fileID: fileID)
        position = min(choice.start, max(0, loaded.duration))
        pendingSeek = position
        ready = false
        nowPlayingChapter = nil
        UserDefaults.standard.set(fileID, forKey: lastFileKey)

        watchServerPlace(fileID: fileID)
        loadArtwork(loaded)
        startTicker()
        updateNowPlaying()
        if autoplay { play() }
    }

    /// How far ahead the stream buffers, so listening goes on through a lost
    /// connection. It keeps filling while paused, so a pause before going
    /// out of range still leaves this much to play.
    private static let forwardBuffer: TimeInterval = 10 * 60

    /// Starts streaming the open File, the first time it plays.
    private func prepareItem() {
        guard let file, player == nil else { return }
        // AVPlayer makes its own range requests rather than going through
        // APIClient, and takes cookies but no headers, so the credential goes
        // as a cookie the stream route reads.
        let asset = AVURLAsset(url: session.client.streamURL(fileID: file.fileID),
                               options: [AVURLAssetHTTPCookiesKey: session.client.streamCookies])
        let item = AVPlayerItem(asset: asset)
        item.audioTimePitchAlgorithm = .timeDomain
        item.preferredForwardBufferDuration = AudioPlayer.forwardBuffer
        let player = AVPlayer(playerItem: item)
        player.automaticallyWaitsToMinimizeStalling = true
        self.player = player
        ready = false
        pendingSeek = pendingSeek ?? position
        itemGeneration += 1
        let generation = itemGeneration
        statusObservation = AudioPlayer.observeStatus(of: item) { [weak self] status, failure in
            Task { @MainActor in
                guard self?.itemGeneration == generation else { return }
                self?.itemStatusChanged(status, failure: failure)
            }
        }
        observeEnd(of: item, generation: generation)
        observeErrorLog(of: item, generation: generation)
        observeFailures(of: item, generation: generation)
    }

    private func itemStatusChanged(_ status: AVPlayerItem.Status, failure: String?) {
        switch status {
        case .readyToPlay:
            guard !ready, let player else { return }
            ready = true
            let target = pendingSeek ?? 0
            pendingSeek = nil
            isBuffering = isPlaying
            let generation = itemGeneration
            AudioPlayer.seek(player, to: target) { [weak self] in
                Task { @MainActor in
                    guard let self, self.itemGeneration == generation else { return }
                    if self.isPlaying { self.player?.playImmediately(atRate: Float(self.rate)) }
                    self.refreshPosition()
                    self.updateNowPlaying()
                }
            }
        case .failed:
            couldNotPlay(failure ?? "unknown error")
        default:
            break
        }
    }

    /// Shows that the stream failed. Playing again builds a new one from
    /// where it was.
    private func couldNotPlay(_ failure: String) {
        isPlaying = false
        isBuffering = false
        error = String(localized: "This audiobook couldn't be played. Check your connection to the server and try again.")
        log("file \(file?.fileID ?? 0): couldn't play: \(failure)")
        stopSitting(at: Date())
        updateNowPlaying()
    }

    // MARK: - Controls

    func togglePlay() {
        isPlaying ? pause() : play()
    }

    func play() {
        guard let file else { return }
        if error != nil {
            // A failed stream is tried again from where it was.
            error = nil
            pendingSeek = position
            teardownItem()
        }
        if player == nil {
            prepareItem()
        }
        // Playing again at the end starts over.
        if file.duration > 0, currentPosition() >= file.duration - 0.5 {
            move(to: 0)
        }
        let audio = AVAudioSession.sharedInstance()
        try? audio.setCategory(.playback, mode: .spokenAudio, policy: .longFormAudio)
        try? audio.setActive(true)
        isPlaying = true
        if ready && pendingSeek == nil {
            player?.playImmediately(atRate: Float(rate))
        } else {
            isBuffering = true
        }
        let now = Date()
        let (next, ended) = ListeningRules.play(sitting, fileID: file.fileID, at: now, percentage: percentage)
        sitting = next
        persistSitting(recording: ended)
        updateNowPlaying()
    }

    func pause() {
        guard file != nil else { return }
        player?.pause()
        retriedAt = nil
        let wasPlaying = isPlaying
        isPlaying = false
        isBuffering = false
        if let player { noteWaiting(false, player: player, at: Date()) }
        refreshPosition()
        updateNowPlaying()
        guard wasPlaying else { return }
        // A sitting a stall already paused keeps when the stall began.
        if let current = sitting, current.isPlaying {
            sitting = ListeningRules.pause(current, at: Date(), percentage: percentage)
        }
        savePosition()
        // The session so far goes out on every pause, so it shows on the web
        // at once; resuming within five minutes extends the same session.
        persistSitting(recording: sitting.flatMap { ListeningRules.finish($0) })
        requestSync()
    }

    func skip(by seconds: Double) {
        seek(to: currentPosition() + seconds)
    }

    func nextChapter() {
        guard let file, let target = AudioChapters.nextTarget(from: currentPosition(), in: file.chapters) else { return }
        seek(to: target)
    }

    func previousChapter() {
        guard let file else { return }
        seek(to: AudioChapters.previousTarget(from: currentPosition(), in: file.chapters))
    }

    /// Moves to seconds, and saves the place there.
    func seek(to seconds: Double) {
        guard let file else { return }
        log("file \(file.fileID): seek from \(Int(currentPosition())) s to \(Int(seconds)) s, \(isPlaying ? "playing" : "paused")")
        move(to: seconds)
        savePosition()
    }

    func setRate(_ value: Double) {
        let clamped = PlaybackSpeed.clamp(value)
        rate = clamped
        if let file {
            UserDefaults.standard.set(clamped, forKey: AudioPlayer.rateKey(accountID: session.account.id, fileID: file.fileID))
        }
        if isPlaying, ready {
            player?.rate = Float(clamped)
        }
        updateNowPlaying()
    }

    /// Goes to the place offered from another device.
    func acceptOffer() {
        guard let offer else { return }
        self.offer = nil
        resolveOffer()
        seek(to: offer.seconds)
    }

    func dismissOffer() {
        dismissedOffer = offer
        offer = nil
        resolveOffer()
    }

    private func resolveOffer() {
        guard let file else { return }
        try? session.store.resolveOffer(fileID: file.fileID)
    }

    /// Stops playing and forgets the File, recording what was listened.
    func stop() {
        unload()
        let center = MPRemoteCommandCenter.shared()
        for command in [center.playCommand, center.pauseCommand, center.togglePlayPauseCommand, center.skipForwardCommand,
                        center.skipBackwardCommand, center.nextTrackCommand, center.previousTrackCommand,
                        center.changePlaybackPositionCommand, center.changePlaybackRateCommand] {
            command.removeTarget(nil)
        }
        for observer in observers {
            NotificationCenter.default.removeObserver(observer)
        }
        observers = []
        ticker?.cancel()
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }

    // MARK: - Tracking

    private var percentage: Double {
        guard let file, file.duration > 0 else { return 0 }
        return min(1, max(0, position / file.duration))
    }

    private func currentPosition() -> Double {
        if let pendingSeek { return pendingSeek }
        if let seekingTo { return seekingTo }
        guard ready, let seconds = player?.currentTime().seconds, seconds.isFinite else { return position }
        return seconds
    }

    private func refreshPosition() {
        position = currentPosition()
    }

    private func startTicker() {
        ticker?.cancel()
        ticker = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .milliseconds(500))
                self?.tick()
            }
        }
    }

    private func tick() {
        guard let player else { return }
        refreshPosition()
        let now = Date()
        // Before the rest, which a failure it reports must not see as
        // playing.
        retryAfterStop(player, at: now)
        if isPlaying {
            let buffering = player.timeControlStatus != .playing
            noteWaiting(buffering, player: player, at: now)
            if buffering != isBuffering {
                isBuffering = buffering
                updateNowPlaying()
            }
            // Only time playing counts: a stall waiting for the stream
            // pauses the sitting's clock.
            if let current = sitting {
                if buffering, current.isPlaying {
                    sitting = ListeningRules.pause(current, at: now, percentage: percentage)
                    persistSitting()
                } else if buffering, ListeningRules.isOver(current, at: now) {
                    // Stalled as long as a pause that ends a sitting.
                    stopSitting(at: now)
                } else if !buffering, !current.isPlaying {
                    let (next, ended) = ListeningRules.play(current, fileID: current.fileID, at: now, percentage: percentage)
                    sitting = next
                    persistSitting(recording: ended)
                } else if !buffering {
                    sitting = ListeningRules.tick(current, at: now, percentage: percentage)
                }
            } else if !buffering, let file {
                // Playing again after a stall ended the last sitting.
                sitting = ListeningRules.play(nil, fileID: file.fileID, at: now, percentage: percentage).0
                persistSitting()
            }
            if now.timeIntervalSince(lastSave) >= ListeningRules.saveInterval {
                savePosition()
                persistSitting()
            }
        } else if let current = sitting, ListeningRules.isOver(current, at: now) {
            // Paused too long: the sitting ends where the pause began.
            stopSitting(at: now)
            requestSync()
        }
        if chapterIndex != nowPlayingChapter {
            updateNowPlaying()
        }
    }

    /// Logs a wait for the stream long enough to notice, with what the
    /// player says it's waiting for, and how it ended. A stall leaves no
    /// error, so without this it wouldn't show in an export.
    private func noteWaiting(_ waiting: Bool, player: AVPlayer, at now: Date) {
        guard let file else { return }
        if waiting {
            let since = waitingSince ?? now
            waitingSince = since
            if !waitingLogged, now.timeIntervalSince(since) >= 3 {
                waitingLogged = true
                log("file \(file.fileID): waiting to play for 3 s, \(AudioPlayer.describeWaiting(player, at: currentPosition()))")
            }
        } else if let since = waitingSince {
            waitingSince = nil
            if waitingLogged {
                waitingLogged = false
                let how = isPlaying ? "playing" : "paused"
                log("file \(file.fileID): \(how) after waiting \(Int(now.timeIntervalSince(since))) s")
            }
        }
    }

    /// What the player and its item say about a wait, as the log names it.
    private static func describeWaiting(_ player: AVPlayer, at seconds: Double) -> String {
        let item = player.currentItem
        let buffered = (item?.loadedTimeRanges ?? []).map { value in
            let range = value.timeRangeValue
            return "\(Int(range.start.seconds))-\(Int(range.end.seconds))"
        }
        let reason = player.reasonForWaitingToPlay?.rawValue ?? "none"
        return "at \(Int(seconds)) s, reason \(reason), buffered \(buffered.isEmpty ? "nothing" : buffered.joined(separator: " ")) s, "
            + "likely to keep up \(item?.isPlaybackLikelyToKeepUp ?? false), buffer empty \(item?.isPlaybackBufferEmpty ?? true), "
            + "item status \(item?.status.rawValue ?? -1)"
    }

    /// Saves the place locally and queues it for the server.
    private func savePosition(atEnd: Bool = false) {
        guard let file else { return }
        lastSave = Date()
        do {
            try session.store.recordPosition(fileID: file.fileID, seconds: position, percentage: percentage, at: lastSave,
                                             atEnd: atEnd, sitting: sitting)
        } catch {
            log("file \(file.fileID): couldn't save the place: \(Diagnostics.describe(error))")
        }
    }

    /// Keeps the sitting, and queues the session of one that ended, which
    /// asks for a sync.
    private func persistSitting(recording ended: FinishedSession? = nil) {
        do {
            try session.store.saveSitting(sitting, recording: ended)
        } catch {
            log("couldn't keep the sitting: \(Diagnostics.describe(error))")
        }
        if ended != nil {
            requestSync()
        }
    }

    /// Ends the sitting, if any, recording its session.
    private func stopSitting(at: Date) {
        guard let current = sitting else { return }
        sitting = nil
        persistSitting(recording: ListeningRules.finish(current, at: at))
    }

    /// Playback reached the end of the File, which finishes the Read-through
    /// underway, as in the web player.
    private func reachedEnd() {
        guard let file else { return }
        isPlaying = false
        isBuffering = false
        position = file.duration
        if let current = sitting {
            sitting = ListeningRules.pause(current, at: Date(), percentage: 1)
        }
        savePosition(atEnd: true)
        stopSitting(at: Date())
        updateNowPlaying()
        requestSync()
    }

    /// Follows the open File in the catalog: an edit, such as renamed
    /// chapters, shows at once, and a File removed from the user's
    /// Libraries closes the player. It also offers a place another device
    /// synced while the File is open, but never jumps to it.
    private func watchServerPlace(fileID: Int) {
        placeWatch?.cancel()
        let pool = session.store.pool
        fileWatch?.cancel()
        fileWatch = Task { [weak self] in
            let files = ValueObservation.tracking { db in try CatalogStore.listeningFile(db, fileID: fileID) }.removeDuplicates()
            do {
                for try await file in files.values(in: pool) {
                    self?.catalogChanged(file, fileID: fileID)
                }
            } catch {}
        }
        placeWatch = Task { [weak self] in
            let places = ValueObservation.tracking { db in try CatalogStore.serverPlace(db, fileID: fileID) }.removeDuplicates()
            do {
                for try await place in places.values(in: pool) {
                    self?.serverPlaceChanged(place)
                }
            } catch {}
        }
    }

    private func catalogChanged(_ updated: ListeningFile?, fileID: Int) {
        guard file?.fileID == fileID else { return }
        guard let updated else {
            close()
            return
        }
        if updated != file {
            let coverChanged = updated.coverVersion != file?.coverVersion || updated.book.coverVersion != file?.book.coverVersion
            file = updated
            if coverChanged { loadArtwork(updated) }
            updateNowPlaying()
        }
    }

    /// Puts a paused File away, as swiping the mini player down does: its
    /// place is kept, and it isn't opened again at the next launch.
    func dismiss() {
        guard !isPlaying, file != nil else { return }
        close()
    }

    /// Closes the File, keeping what was listened, as when it's no longer
    /// in the user's Libraries.
    private func close() {
        unload()
        UserDefaults.standard.removeObject(forKey: lastFileKey)
    }

    /// Lets go of the File: records what was listened, stops the stream and
    /// following the catalog, and clears the lock screen.
    private func unload() {
        if isPlaying { pause() }
        stopSitting(at: Date())
        teardownItem()
        placeWatch?.cancel()
        fileWatch?.cancel()
        file = nil
        offer = nil
        MPNowPlayingInfoCenter.default().nowPlayingInfo = nil
        MPNowPlayingInfoCenter.default().playbackState = .stopped
    }

    private func serverPlaceChanged(_ place: ServerPlace?) {
        // Loaded but not being listened to, the File is as good as unopened,
        // including when the server's place was reset.
        guard sitting != nil || isPlaying else {
            if place == nil || place != dismissedOffer && place != offer {
                applyNewestPlace()
            }
            return
        }
        guard let place, place != dismissedOffer, place != offer else { return }
        if let offered = PlaceRules.offerDuringListening(server: place, at: currentPosition(), known: knownServerAt,
                                                         ownDeviceID: session.account.deviceID) {
            offer = offered
        }
    }

    /// Goes to where the File opens now (`PlaceRules.open`), or offers the
    /// other device's place. It isn't a move of the user's, so it saves
    /// nothing.
    private func applyNewestPlace() {
        guard let file else { return }
        let fileID = file.fileID
        let ownDevice = session.account.deviceID
        guard let places = try? session.store.pool.read({ db in
            (local: try CatalogStore.localPlace(db, fileID: fileID), server: try CatalogStore.serverPlace(db, fileID: fileID))
        }) else { return }
        let choice = PlaceRules.open(local: places.local, server: places.server, ownDeviceID: ownDevice)
        knownServerAt = places.server?.occurredAt
        if let other = choice.offer, other != dismissedOffer {
            offer = other
        } else if abs(choice.start - currentPosition()) > 1 {
            move(to: choice.start)
        }
    }

    /// Moves the playhead without saving a place.
    private func move(to seconds: Double) {
        guard let file else { return }
        let target = max(0, min(seconds, max(0, file.duration - 0.5)))
        position = target
        if ready, let player {
            seekingTo = target
            seekCount += 1
            // A seek is a new chance to decode, so a failure after it gets its
            // own retry (retryAfterStop).
            retriedStop = false
            retriedAt = nil
            let generation = itemGeneration
            let count = seekCount
            AudioPlayer.seek(player, to: target) { [weak self] in
                Task { @MainActor in
                    guard let self, self.itemGeneration == generation else { return }
                    // A later seek supersedes this one, and clears its own.
                    if self.seekCount == count {
                        self.seekingTo = nil
                        // Seeking while the stream had stopped plays from
                        // the new place, as pausing and playing would.
                        if self.isPlaying, self.player?.timeControlStatus == .paused {
                            self.player?.playImmediately(atRate: Float(self.rate))
                        }
                    }
                    self.refreshPosition()
                    self.updateNowPlaying()
                }
            }
        } else {
            pendingSeek = target
        }
        updateNowPlaying()
    }

    // MARK: - Speed

    // Speed is the File's, since it suits the narrator: a Book's two
    // audiobooks are often read by different people.
    private static func rateKey(accountID: String, fileID: Int) -> String {
        "player.speed.\(accountID).file.\(fileID)"
    }

    /// The File's own speed on this installation; a new File starts at 1x.
    static func storedRate(accountID: String, fileID: Int) -> Double {
        let stored = UserDefaults.standard.double(forKey: rateKey(accountID: accountID, fileID: fileID))
        return stored == 0 ? 1 : PlaybackSpeed.clamp(stored)
    }

    private func storedRate(fileID: Int) -> Double {
        AudioPlayer.storedRate(accountID: session.account.id, fileID: fileID)
    }

    // MARK: - System

    // AVFoundation calls these back on its own queues, so the closures are
    // made outside the main actor and hop back to it.

    nonisolated private static func observeStatus(of item: AVPlayerItem,
                                                  _ handler: @escaping @Sendable (AVPlayerItem.Status, String?) -> Void) -> NSKeyValueObservation {
        item.observe(\.status, options: [.new]) { item, _ in
            handler(item.status, item.error.map(Diagnostics.describe))
        }
    }

    /// MediaPlayer asks for the artwork on its own queue.
    nonisolated private static func nowPlayingArtwork(_ image: UIImage) -> MPMediaItemArtwork {
        MPMediaItemArtwork(boundsSize: image.size) { _ in image }
    }

    nonisolated private static func seek(_ player: AVPlayer, to seconds: Double, then done: @escaping @Sendable () -> Void) {
        player.seek(to: CMTime(seconds: seconds, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero) { _ in
            done()
        }
    }

    private func teardownItem() {
        itemGeneration += 1
        seekingTo = nil
        for observer in [endObserver, errorLogObserver, failureObserver].compactMap({ $0 }) {
            NotificationCenter.default.removeObserver(observer)
        }
        endObserver = nil
        errorLogObserver = nil
        failureObserver = nil
        waitingSince = nil
        waitingLogged = false
        stoppedAt = nil
        retriedStop = false
        retriedAt = nil
        statusObservation?.invalidate()
        statusObservation = nil
        // The catalog and place watches belong to the File, not the stream:
        // a stream tried again keeps them, and open() replaces them.
        player?.pause()
        player?.replaceCurrentItem(with: nil)
        player = nil
        ready = false
        isPlaying = false
        isBuffering = false
    }

    private func observeEnd(of item: AVPlayerItem, generation: Int) {
        endObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.didPlayToEndTimeNotification, object: item, queue: .main) { [weak self] _ in
            Task { @MainActor in
                guard self?.itemGeneration == generation else { return }
                self?.reachedEnd()
            }
        }
    }

    /// Logs what the stream reports going wrong without failing, such as a
    /// request the server refused, which can leave playback waiting.
    private func observeErrorLog(of item: AVPlayerItem, generation: Int) {
        errorLogObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.newErrorLogEntryNotification, object: item,
                                                                  queue: .main) { [weak self] note in
            let entry = (note.object as? AVPlayerItem)?.errorLog()?.events.last
            let text = entry.map { "\($0.errorDomain) \($0.errorStatusCode): \($0.errorComment ?? "no comment")" } ?? "no entry"
            Task { @MainActor in
                guard let self, self.itemGeneration == generation, let file = self.file else { return }
                self.log("file \(file.fileID): stream reported \(text)")
            }
        }
    }

    /// Notes when playback stops partway with an error, so the ticker can
    /// play again (`retryAfterStop`).
    private func observeFailures(of item: AVPlayerItem, generation: Int) {
        failureObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.failedToPlayToEndTimeNotification, object: item,
                                                                 queue: .main) { [weak self] note in
            let failure = (note.userInfo?[AVPlayerItemFailedToPlayToEndTimeErrorKey] as? Error).map(Diagnostics.describe)
            Task { @MainActor in
                guard let self, self.itemGeneration == generation else { return }
                self.stoppedWithError(failure ?? "unknown error")
            }
        }
    }

    private func stoppedWithError(_ failure: String) {
        guard let file else { return }
        let now = Date()
        // One failure is reported several times in a row. A new one starts
        // after a quiet spell, or after a seek, which also gives it its own
        // retry (move(to:)).
        let quiet = stoppedAt.map { now.timeIntervalSince($0) > AudioPlayer.stopWindow } ?? true
        if quiet {
            retriedStop = false
        }
        if quiet || stoppedSeek != seekCount {
            log("file \(file.fileID): stopped at \(Int(currentPosition())) s: \(failure)")
        }
        stoppedAt = now
        stoppedSeek = seekCount
    }

    /// How long after a failure the player may play again on its own.
    private static let stopWindow: TimeInterval = 5

    /// Plays again, once per failure, when the stream stopped with an error
    /// although playback is wanted. Some xHE-AAC audiobooks mark every frame
    /// as a place decoding can start, but only some are; a seek that lands
    /// on another fails with "Cannot Decode" and AVPlayer pauses itself.
    /// Playing again from there decodes, as pausing and playing does by hand.
    /// If it's still stopped a while later, or stops again before another
    /// seek, the failure shows, rather than loading forever.
    private func retryAfterStop(_ player: AVPlayer, at now: Date) {
        guard isPlaying, ready, pendingSeek == nil, seekingTo == nil, let file else { return }
        let stopped = player.timeControlStatus == .paused
        if let retriedAt {
            // Waiting for audio after playing again counts as not playing
            // yet, so a retry that never gets going still shows the error.
            if player.timeControlStatus == .playing {
                self.retriedAt = nil
            } else if now.timeIntervalSince(retriedAt) >= AudioPlayer.stopWindow {
                self.retriedAt = nil
                couldNotPlay("still not playing after playing again")
            }
            return
        }
        guard stopped, let stoppedAt, now.timeIntervalSince(stoppedAt) < AudioPlayer.stopWindow else { return }
        if retriedStop {
            couldNotPlay("stopped again after playing again")
            return
        }
        retriedStop = true
        retriedAt = now
        log("file \(file.fileID): playing again at \(Int(currentPosition())) s after the stream stopped")
        player.playImmediately(atRate: Float(rate))
    }

    /// Interruptions, such as a call, pause without counting, and resume
    /// when the system says to. Unplugging headphones pauses.
    private func observeSystem() {
        let center = NotificationCenter.default
        observers.append(center.addObserver(forName: AVAudioSession.interruptionNotification, object: nil, queue: .main) { [weak self] note in
            let type = (note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt).flatMap(AVAudioSession.InterruptionType.init)
            let options = (note.userInfo?[AVAudioSessionInterruptionOptionKey] as? UInt).map(AVAudioSession.InterruptionOptions.init)
            Task { @MainActor in self?.interrupted(type, options: options) }
        })
        observers.append(center.addObserver(forName: AVAudioSession.routeChangeNotification, object: nil, queue: .main) { [weak self] note in
            let reason = (note.userInfo?[AVAudioSessionRouteChangeReasonKey] as? UInt).flatMap(AVAudioSession.RouteChangeReason.init)
            guard reason == .oldDeviceUnavailable else { return }
            Task { @MainActor in
                if self?.isPlaying == true { self?.pause() }
            }
        })
    }

    private func interrupted(_ type: AVAudioSession.InterruptionType?, options: AVAudioSession.InterruptionOptions?) {
        switch type {
        case .began:
            resumeAfterInterruption = isPlaying
            if isPlaying { pause() }
        case .ended:
            if resumeAfterInterruption, options?.contains(.shouldResume) == true { play() }
            resumeAfterInterruption = false
        default:
            break
        }
    }

    private func setUpRemoteCommands() {
        let center = MPRemoteCommandCenter.shared()
        center.skipBackwardCommand.preferredIntervals = [NSNumber(value: AudioPlayer.skipInterval)]
        center.skipForwardCommand.preferredIntervals = [NSNumber(value: AudioPlayer.skipInterval)]
        center.changePlaybackRateCommand.supportedPlaybackRates = [0.8, 1.0, 1.2, 1.5, 2.0].map { NSNumber(value: $0) }
        let commands: [(MPRemoteCommand, @MainActor (AudioPlayer, Double?) -> Void)] = [
            (center.playCommand, { player, _ in player.play() }),
            (center.pauseCommand, { player, _ in player.pause() }),
            (center.togglePlayPauseCommand, { player, _ in player.togglePlay() }),
            (center.skipBackwardCommand, { player, _ in player.skip(by: -AudioPlayer.skipInterval) }),
            (center.skipForwardCommand, { player, _ in player.skip(by: AudioPlayer.skipInterval) }),
            // Headphones' double and triple click, and a car's track
            // buttons, skip as the lock screen does rather than change
            // chapter; chapters change in the app.
            (center.nextTrackCommand, { player, _ in player.skip(by: AudioPlayer.skipInterval) }),
            (center.previousTrackCommand, { player, _ in player.skip(by: -AudioPlayer.skipInterval) }),
            (center.changePlaybackPositionCommand, { player, time in
                if let time { player.seek(to: player.chapterBounds.start + time) }
            }),
            (center.changePlaybackRateCommand, { player, rate in
                if let rate { player.setRate(rate) }
            }),
        ]
        for (command, action) in commands {
            AudioPlayer.onCommand(command) { [weak self] value in
                Task { @MainActor in
                    guard let self else { return }
                    action(self, value)
                }
            }
        }
    }

    /// Calls handle when command comes, from whichever queue MediaPlayer
    /// uses, with the time or rate the command carries.
    nonisolated private static func onCommand(_ command: MPRemoteCommand, _ handle: @escaping @Sendable (Double?) -> Void) {
        command.addTarget { event in
            switch event {
            case let event as MPChangePlaybackPositionCommandEvent:
                handle(event.positionTime)
            case let event as MPChangePlaybackRateCommandEvent:
                handle(Double(event.playbackRate))
            default:
                handle(nil)
            }
            return .success
        }
    }

    /// Tells the lock screen, Control Center, and CarPlay what's playing:
    /// the chapter, its real length, and the rate, from which they move the
    /// progress themselves.
    private func updateNowPlaying() {
        guard let file else { return }
        let bounds = chapterBounds
        nowPlayingChapter = chapterIndex
        var info: [String: Any] = [
            MPMediaItemPropertyTitle: chapterTitle ?? file.book.title,
            MPMediaItemPropertyAlbumTitle: file.book.title,
            MPMediaItemPropertyArtist: file.book.authors,
            MPMediaItemPropertyPlaybackDuration: max(0, bounds.end - bounds.start),
            MPNowPlayingInfoPropertyElapsedPlaybackTime: max(0, position - bounds.start),
            MPNowPlayingInfoPropertyPlaybackRate: isPlaying && !isBuffering ? rate : 0,
            MPNowPlayingInfoPropertyDefaultPlaybackRate: rate,
            MPNowPlayingInfoPropertyMediaType: MPNowPlayingInfoMediaType.audio.rawValue,
        ]
        if let index = chapterIndex {
            info[MPNowPlayingInfoPropertyChapterNumber] = index
            info[MPNowPlayingInfoPropertyChapterCount] = file.chapters.count
        }
        if let artwork {
            info[MPMediaItemPropertyArtwork] = AudioPlayer.nowPlayingArtwork(artwork)
        }
        MPNowPlayingInfoCenter.default().nowPlayingInfo = info
        MPNowPlayingInfoCenter.default().playbackState = isPlaying ? .playing : .paused
    }

    /// The lock screen's artwork: the File's own cover, or its Book's.
    private func loadArtwork(_ loaded: ListeningFile) {
        let covers = session.covers
        let (owner, version): (CoverCache.Owner, String) = loaded.coverVersion.isEmpty
            ? (.book(loaded.book.id), loaded.book.coverVersion) : (.file(loaded.fileID), loaded.coverVersion)
        Task { [weak self] in
            let image = await covers.image(owner, version: version, size: 512)
            guard let self, self.file?.fileID == loaded.fileID else { return }
            self.artwork = image
            self.updateNowPlaying()
        }
    }

    private func log(_ message: String) {
        Task { await Diagnostics.shared.log(.player, message) }
    }
}
