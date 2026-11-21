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
    /// autoplay. Opening the File already loaded only plays it.
    func open(fileID: Int, autoplay: Bool) {
        if file?.fileID == fileID, player != nil {
            if sitting == nil, !isPlaying {
                applyNewestPlace()
            }
            if autoplay { play() }
            return
        }
        if player != nil {
            pause()
            teardownItem()
        }
        let store = session.store
        let ownDevice = session.account.deviceID
        guard let loaded = try? store.pool.read({ try CatalogStore.listeningFile($0, fileID: fileID) }) else {
            UserDefaults.standard.removeObject(forKey: lastFileKey)
            return
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
        rate = storedRate(bookID: loaded.book.id)
        position = min(choice.start, max(0, loaded.duration))
        pendingSeek = position
        ready = false
        nowPlayingChapter = nil
        UserDefaults.standard.set(fileID, forKey: lastFileKey)

        // AVPlayer makes its own range requests, so they carry the
        // account's headers here rather than going through APIClient.
        let asset = AVURLAsset(url: session.client.streamURL(fileID: fileID),
                               options: ["AVURLAssetHTTPHeaderFieldsKey": session.client.headers])
        let item = AVPlayerItem(asset: asset)
        item.audioTimePitchAlgorithm = .timeDomain
        // Buffer far ahead, so listening goes on through a lost connection.
        item.preferredForwardBufferDuration = 15 * 60
        let player = AVPlayer(playerItem: item)
        player.automaticallyWaitsToMinimizeStalling = true
        self.player = player
        itemGeneration += 1
        let generation = itemGeneration
        statusObservation = AudioPlayer.observeStatus(of: item) { [weak self] status, failure in
            Task { @MainActor in
                guard self?.itemGeneration == generation else { return }
                self?.itemStatusChanged(status, failure: failure)
            }
        }
        observeEnd(of: item, generation: generation)
        watchServerPlace(fileID: fileID)
        loadArtwork(loaded.book)
        startTicker()
        updateNowPlaying()
        if autoplay { play() }
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
            isPlaying = false
            isBuffering = false
            error = String(localized: "This audiobook couldn't be played. Check your connection to the server and try again.")
            log("file \(file?.fileID ?? 0): couldn't play: \(failure ?? "unknown error")")
            stopSitting(at: Date())
            updateNowPlaying()
        default:
            break
        }
    }

    // MARK: - Controls

    func togglePlay() {
        isPlaying ? pause() : play()
    }

    func play() {
        guard let file, player != nil else { return }
        if error != nil {
            // A failed stream is tried again from where it was.
            let fileID = file.fileID
            teardownItem()
            self.file = nil
            open(fileID: fileID, autoplay: true)
            return
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
        guard player != nil else { return }
        player?.pause()
        let wasPlaying = isPlaying
        isPlaying = false
        isBuffering = false
        refreshPosition()
        updateNowPlaying()
        guard wasPlaying else { return }
        // A sitting a stall already paused keeps when the stall began.
        if let current = sitting, current.isPlaying {
            sitting = ListeningRules.pause(current, at: Date(), percentage: percentage)
        }
        savePosition()
        persistSitting()
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
        guard file != nil else { return }
        move(to: seconds)
        savePosition()
    }

    func setRate(_ value: Double) {
        let clamped = PlaybackSpeed.clamp(value)
        rate = clamped
        if let file {
            UserDefaults.standard.set(clamped, forKey: rateKey(bookID: file.book.id))
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
        if isPlaying { pause() }
        stopSitting(at: Date())
        teardownItem()
        file = nil
        MPNowPlayingInfoCenter.default().nowPlayingInfo = nil
        MPNowPlayingInfoCenter.default().playbackState = .stopped
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
        if isPlaying {
            let buffering = player.timeControlStatus != .playing
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
            file = updated
            updateNowPlaying()
        }
    }

    /// Closes the File, keeping what was listened, as when it's no longer
    /// in the user's Libraries.
    private func close() {
        if isPlaying { pause() }
        stopSitting(at: Date())
        teardownItem()
        file = nil
        offer = nil
        UserDefaults.standard.removeObject(forKey: lastFileKey)
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
            let generation = itemGeneration
            AudioPlayer.seek(player, to: target) { [weak self] in
                Task { @MainActor in
                    guard self?.itemGeneration == generation else { return }
                    self?.refreshPosition()
                    self?.updateNowPlaying()
                }
            }
        } else {
            pendingSeek = target
        }
        updateNowPlaying()
    }

    // MARK: - Speed

    private static func rateKey(accountID: String, bookID: Int) -> String {
        "player.speed.\(accountID).\(bookID)"
    }

    private func rateKey(bookID: Int) -> String {
        AudioPlayer.rateKey(accountID: session.account.id, bookID: bookID)
    }

    /// The Book's own speed on this installation; a new Book starts at 1x.
    static func storedRate(accountID: String, bookID: Int) -> Double {
        let stored = UserDefaults.standard.double(forKey: rateKey(accountID: accountID, bookID: bookID))
        return stored == 0 ? 1 : PlaybackSpeed.clamp(stored)
    }

    private func storedRate(bookID: Int) -> Double {
        AudioPlayer.storedRate(accountID: session.account.id, bookID: bookID)
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
        if let endObserver {
            NotificationCenter.default.removeObserver(endObserver)
            self.endObserver = nil
        }
        statusObservation?.invalidate()
        statusObservation = nil
        placeWatch?.cancel()
        fileWatch?.cancel()
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

    private func loadArtwork(_ book: BookSummary) {
        let covers = session.covers
        Task { [weak self] in
            let image = await covers.image(bookID: book.id, version: book.coverVersion, size: 512)
            guard let self, self.file?.book.id == book.id else { return }
            self.artwork = image
            self.updateNowPlaying()
        }
    }

    private func log(_ message: String) {
        Task { await Diagnostics.shared.log(.player, message) }
    }
}
