import AVFoundation
import GRDB
import MediaPlayer
import Observation
import ShishoAPI
import UIKit

/// Plays one account's audiobooks, and keeps the user's place and time.
///
/// Everything that tracks listening lives here, not in a view: it keeps
/// running with the phone locked and with no screen open. The place is
/// saved every five seconds of playback and on pause, sittings are timed
/// by `ListeningRules`, and both go into the account's journal for the
/// next sync (ADR 0024). The lock screen and Control Center show the
/// chapter's real length and the playback rate, so progress moves faster at
/// speed rather than the length shrinking. The sleep timer, smart rewind,
/// and Bookmarks live here too, so they work with no screen open.
@MainActor
@Observable
final class AudioPlayer {
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
    /// How far the skip buttons go, back and forward, in the app, on the
    /// lock screen, and from headphones and the car.
    private(set) var backInterval = PlayerSettings.skipBack
    private(set) var forwardInterval = PlayerSettings.skipForward
    /// Going back a little after a long pause.
    private(set) var smartRewind = PlayerSettings.smartRewind
    private(set) var sleepTimer: SleepTimer = .off

    var chapterIndex: Int? {
        file.flatMap { AudioChapters.index(at: position, in: $0.chapters) }
    }

    /// The chapter playing, as `AudioChapters.name` names it.
    var chapterTitle: String? {
        guard let file, let index = chapterIndex else { return nil }
        return AudioChapters.name(of: index, in: file.chapters)
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
    @ObservationIgnored private var stoppedPosition: Double = 0
    @ObservationIgnored private var retriedStop = false
    /// When the player played again after a failure, until it plays.
    @ObservationIgnored private var retriedAt: Date?
    /// When the listener paused, for smart rewind; nil once they chose a
    /// place since.
    @ObservationIgnored private var pausedAt: Date?
    /// When the last tick saw audio playing, so the sleep timer counts only
    /// time played.
    @ObservationIgnored private var lastPlayingTick: Date?
    /// Where an end of chapter timer stops. It's set where the timer is set
    /// and where the listener goes, but not by smart rewind, which can go
    /// back over a chapter start without changing which chapter is playing.
    @ObservationIgnored private var sleepTarget: Double?
    @ObservationIgnored private let shake = ShakeDetector()
    /// The item playing is the File's Download, not a stream.
    @ObservationIgnored private var playingDownload = false
    /// Playback stopped, or couldn't start, for want of a connection: the
    /// message goes when the connection comes back.
    @ObservationIgnored private var waitingForConnection = false
    /// The server was checked during the current wait for the stream.
    @ObservationIgnored private var checkedWhileWaiting = false

    init(session: AccountSession, requestSync: @escaping @MainActor () -> Void) {
        self.session = session
        self.requestSync = requestSync
        observeSystem()
        setUpRemoteCommands()
    }

    private var lastFileKey: String { AccountData(accountID: session.account.id).lastFileKey }

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
        waitingForConnection = false
        offer = choice.offer
        dismissedOffer = nil
        knownServerAt = places?.server?.occurredAt
        artwork = nil
        rate = storedRate(fileID: fileID)
        position = min(choice.start, max(0, loaded.duration))
        // Opening at this device's own place picks up after a pause as long
        // as the time since it was saved.
        pausedAt = places?.local.flatMap { abs($0.seconds - position) < 1 ? $0.updatedAt : nil }
        retargetSleep()
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

    /// Starts playing the open File, the first time it plays: from its
    /// Download when there is one, or else streamed.
    private func prepareItem() {
        guard let file, player == nil else { return }
        let asset: AVURLAsset
        if let local = session.downloads.localFile(fileID: file.fileID) {
            asset = AVURLAsset(url: local)
            playingDownload = true
        } else {
            // AVPlayer makes its own range requests rather than going through
            // APIClient, and takes cookies but no headers, so the credential
            // goes as a cookie the stream route reads.
            asset = AVURLAsset(url: session.client.streamURL(fileID: file.fileID),
                               options: [AVURLAssetHTTPCookiesKey: session.client.streamCookies])
            playingDownload = false
        }
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

    /// The connection changed: back, the message that it's needed goes; gone
    /// while a stream waits for more, playback stops and says why.
    func connectionChanged(_ connection: Connection) {
        if connection == .online {
            if waitingForConnection {
                waitingForConnection = false
                error = nil
            }
        } else if isPlaying, isBuffering, player != nil, !playingDownload {
            player?.pause()
            showNeedsConnection(connection)
        }
    }

    private func showNeedsConnection(_ connection: Connection) {
        isPlaying = false
        isBuffering = false
        waitingForConnection = true
        error = ConnectionRules.audiobook(connection)
        log("file \(file?.fileID ?? 0): no connection, \(connection == .offline ? "offline" : "the server can't be reached")")
        stopSitting(at: Date())
        updateNowPlaying()
    }

    /// Shows that the stream failed. Playing again builds a new one from
    /// where it was.
    private func couldNotPlay(_ failure: String) {
        isPlaying = false
        isBuffering = false
        error = String(localized: "This audiobook couldn't be played. Check your connection to the server and try again.")
        watchForShakes()
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
            waitingForConnection = false
            pendingSeek = position
            teardownItem()
        }
        // A stream loaded before its File finished downloading gives way to
        // the Download, from the same place.
        if player != nil, !isPlaying, !playingDownload, session.downloads.localFile(fileID: file.fileID) != nil {
            pendingSeek = currentPosition()
            teardownItem()
        }
        // Not on the device and no connection: say so at once rather than
        // wait for a stream that can't come. One already loaded plays what
        // it has.
        if player == nil, session.downloads.localFile(fileID: file.fileID) == nil, !session.client.reachability.allowsRequests {
            showNeedsConnection(session.client.reachability.connection)
            return
        }
        if player == nil {
            prepareItem()
        }
        // Playing again at the end starts over.
        if ListeningRules.isAtEnd(currentPosition(), duration: file.duration) {
            move(to: 0)
            retargetSleep()
        } else if smartRewind, let pausedAt {
            let back = SmartRewind.seconds(afterPause: Date().timeIntervalSince(pausedAt))
            if back > 0 {
                log("file \(file.fileID): back \(Int(back)) s after a pause of \(Int(Date().timeIntervalSince(pausedAt))) s")
                move(to: currentPosition() - back)
            }
        }
        pausedAt = nil
        let audio = AVAudioSession.sharedInstance()
        try? audio.setCategory(.playback, mode: .spokenAudio, policy: .longFormAudio)
        try? audio.setActive(true)
        isPlaying = true
        watchForShakes()
        player?.volume = SleepTimerRules.volume(sleepTimer)
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
        watchForShakes()
        if let player { noteWaiting(false, player: player, at: Date()) }
        refreshPosition()
        updateNowPlaying()
        guard wasPlaying else { return }
        pausedAt = Date()
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

    func skipBack() {
        skip(by: -backInterval)
    }

    func skipForward() {
        skip(by: forwardInterval)
    }

    func setBackInterval(_ seconds: Double) {
        PlayerSettings.skipBack = seconds
        backInterval = PlayerSettings.skipBack
        applySkipIntervals()
    }

    func setForwardInterval(_ seconds: Double) {
        PlayerSettings.skipForward = seconds
        forwardInterval = PlayerSettings.skipForward
        applySkipIntervals()
    }

    func setSmartRewind(_ on: Bool) {
        PlayerSettings.smartRewind = on
        smartRewind = on
    }

    func nextChapter() {
        guard let file, let target = AudioChapters.nextTarget(from: currentPosition(), in: file.chapters) else { return }
        seek(to: target)
    }

    func previousChapter() {
        guard let file else { return }
        seek(to: AudioChapters.previousTarget(from: currentPosition(), in: file.chapters))
    }

    /// Moves to seconds, and saves the place there. A place the listener
    /// chose plays from exactly there, with no smart rewind.
    func seek(to seconds: Double) {
        guard let file else { return }
        log("file \(file.fileID): seek from \(Int(currentPosition())) s to \(Int(seconds)) s, \(isPlaying ? "playing" : "paused")")
        move(to: seconds)
        pausedAt = nil
        retargetSleep()
        savePosition()
    }

    /// Steps the speed by steps of the web player's 0.1, as the speed
    /// buttons do.
    func stepRate(by steps: Int) {
        setRate(rate + Double(steps) * PlaybackSpeed.step)
    }

    func setRate(_ value: Double) {
        let clamped = PlaybackSpeed.clamp(value)
        rate = clamped
        if let file {
            AudioPlayer.storeRate(clamped, accountID: session.account.id, fileID: file.fileID)
        }
        if isPlaying, ready {
            player?.rate = Float(clamped)
        }
        updateNowPlaying()
    }

    // MARK: - Sleep timer

    /// Sets the sleep timer. A shake while it counts starts its length over.
    func setSleepTimer(_ timer: SleepTimer) {
        sleepTimer = timer
        retargetSleep()
        player?.volume = SleepTimerRules.volume(timer)
        watchForShakes()
        log("sleep timer \(AudioPlayer.describe(timer))")
    }

    /// Sets where an end of chapter timer stops, from where playback is.
    private func retargetSleep() {
        guard sleepTimer == .endOfChapter, let file else {
            sleepTarget = nil
            return
        }
        sleepTarget = SleepTimerRules.chapterEnd(after: currentPosition(), chapters: file.chapters, duration: file.duration)
    }

    /// A shake extends a counting timer while it plays; paused, nothing
    /// counts, so the accelerometer rests.
    private func watchForShakes() {
        if case .counting = sleepTimer, isPlaying {
            shake.start { [weak self] in self?.extendSleepTimer() }
        } else {
            shake.stop()
        }
    }

    /// A shake starts the timer's length over, as long as it's counting.
    func extendSleepTimer() {
        guard case .counting = sleepTimer else { return }
        sleepTimer = SleepTimerRules.extended(sleepTimer)
        player?.volume = SleepTimerRules.volume(sleepTimer)
        shake.confirm()
        log("sleep timer extended by a shake")
    }

    /// Counts the timer down by what played since the last tick, and stops
    /// playback when it runs out or the chapter it waits for ends.
    private func runSleepTimer(playing: Bool, at now: Date) {
        defer { lastPlayingTick = playing ? now : nil }
        guard playing, let file else { return }
        switch sleepTimer {
        case .off:
            return
        case .counting:
            // A tick that came late, as after the app was suspended, counts
            // at most a few seconds.
            let played = lastPlayingTick.map { min(3, now.timeIntervalSince($0)) } ?? 0
            let (next, ranOut) = SleepTimerRules.played(sleepTimer, for: played)
            sleepTimer = next
            player?.volume = SleepTimerRules.volume(next)
            if ranOut {
                sleepTimerStopped()
            }
        case .endOfChapter:
            guard let end = sleepTarget, currentPosition() >= end else { return }
            sleepTimerStopped()
            // Resuming plays the next chapter from its start.
            if end < file.duration - 0.5 {
                seek(to: end)
            }
        }
    }

    private func sleepTimerStopped() {
        log("file \(file?.fileID ?? 0): sleep timer stopped playback at \(Int(currentPosition())) s")
        sleepTimer = .off
        shake.stop()
        pause()
        player?.volume = 1
    }

    private static func describe(_ timer: SleepTimer) -> String {
        switch timer {
        case .off: "off"
        case let .counting(length, _): "set for \(Int(length / 60)) min"
        case .endOfChapter: "set for the end of the chapter"
        }
    }

    // MARK: - Bookmarks

    /// Bookmarks the open File at seconds, or where playback is, and queues
    /// it for the server. It returns the Bookmark, for a note added after.
    @discardableResult
    func addBookmark(note: String?, at seconds: Double? = nil) -> Bookmark? {
        guard let file else { return nil }
        let place = seconds ?? currentPosition()
        defer { requestSync() }
        do {
            let bookmark = try session.store.addBookmark(fileID: file.fileID, seconds: place, note: note, at: Date())
            log("file \(file.fileID): bookmark added at \(Int(place)) s")
            return bookmark
        } catch {
            log("file \(file.fileID): couldn't add a bookmark: \(Diagnostics.describe(error))")
            return nil
        }
    }

    func editBookmark(_ bookmark: Bookmark, note: String) {
        do {
            try session.store.editBookmarkNote(bookmark.id, note: note, at: Date())
        } catch {
            log("file \(bookmark.fileID): couldn't edit a bookmark: \(Diagnostics.describe(error))")
        }
        requestSync()
    }

    func deleteBookmark(_ bookmark: Bookmark) {
        do {
            try session.store.deleteBookmark(bookmark.id, at: Date())
        } catch {
            log("file \(bookmark.fileID): couldn't delete a bookmark: \(Diagnostics.describe(error))")
        }
        requestSync()
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
        setSleepTimer(.off)
        let center = MPRemoteCommandCenter.shared()
        for command in [center.playCommand, center.pauseCommand, center.togglePlayPauseCommand, center.skipForwardCommand,
                        center.skipBackwardCommand, center.nextTrackCommand, center.previousTrackCommand,
                        center.seekForwardCommand, center.seekBackwardCommand,
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
        runSleepTimer(playing: isPlaying && player.timeControlStatus == .playing, at: now)
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
            // A stream that keeps waiting may have lost the server, which
            // the system player doesn't say: a quick check finds out, and
            // if it's gone, playback stops and says so.
            if !checkedWhileWaiting, !playingDownload, now.timeIntervalSince(since) >= 5 {
                checkedWhileWaiting = true
                let client = session.client
                Task { await client.checkServer() }
            }
            if !waitingLogged, now.timeIntervalSince(since) >= 3 {
                waitingLogged = true
                log("file \(file.fileID): waiting to play for 3 s, \(AudioPlayer.describeWaiting(player, at: currentPosition()))")
            }
        } else if let since = waitingSince {
            waitingSince = nil
            checkedWhileWaiting = false
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
        watchForShakes()
        pausedAt = nil
        if sleepTimer != .off {
            setSleepTimer(.off)
        }
        position = file.duration
        if let current = sitting {
            sitting = ListeningRules.pause(current, at: Date(), percentage: 1)
        }
        savePosition(atEnd: true)
        stopSitting(at: Date())
        updateNowPlaying()
        requestSync()
        session.downloads.finished(fileID: file.fileID)
    }

    /// A File's Download arrived or went. The open File, while it isn't
    /// playing, lets go of what it loaded, so it plays from the Download, or
    /// streams once the Download is gone. Playing on, it keeps what it has:
    /// a stream already buffered, or a removed Download's file, which stays
    /// readable while it's open.
    func downloadChanged(fileID: Int) {
        guard file?.fileID == fileID, player != nil, !isPlaying else { return }
        let hasDownload = session.downloads.localFile(fileID: fileID) != nil
        guard hasDownload != playingDownload else { return }
        teardownItem()
        pendingSeek = position
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
            let coverChanged = updated.coverVersion != file?.coverVersion
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

    /// The user cleared the File's saved place: a paused File goes back to
    /// its start, which isn't saved as a place until it plays. One playing
    /// keeps going from where it is.
    func positionCleared(fileID: Int) {
        guard file?.fileID == fileID, !isPlaying else { return }
        move(to: 0)
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
        if let place {
            if place.isSameMove(as: dismissedOffer) { return }
            // A renamed Device sends its place again: the offer shown takes
            // the new name.
            if place.isSameMove(as: offer) {
                if place != offer { offer = place }
                return
            }
        }
        // Loaded but not being listened to, the File is as good as unopened,
        // including when the server's place was reset.
        guard sitting != nil || isPlaying else {
            applyNewestPlace()
            return
        }
        guard let place else { return }
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
        if let other = choice.offer, !other.isSameMove(as: dismissedOffer) {
            offer = other
        } else if abs(choice.start - currentPosition()) > 1 {
            move(to: choice.start)
            retargetSleep()
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
        AccountData(accountID: accountID).speedKey(fileID: fileID)
    }

    static func storeRate(_ rate: Double, accountID: String, fileID: Int) {
        UserDefaults.standard.set(PlaybackSpeed.clamp(rate), forKey: rateKey(accountID: accountID, fileID: fileID))
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
        let position = currentPosition()
        let new = StreamStopRules.isNew(at: now, position: position, last: stoppedAt.map { ($0, stoppedPosition) })
        if new {
            retriedStop = false
        }
        if new || stoppedSeek != seekCount {
            log("file \(file.fileID): stopped at \(Int(position)) s: \(failure)")
        }
        stoppedAt = now
        stoppedPosition = position
        stoppedSeek = seekCount
    }

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
            } else if now.timeIntervalSince(retriedAt) >= StreamStopRules.window {
                self.retriedAt = nil
                couldNotPlay("still not playing after playing again")
            }
            return
        }
        guard stopped, let stoppedAt, now.timeIntervalSince(stoppedAt) < StreamStopRules.window else { return }
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

    /// The lock screen's and Control Center's skip buttons show the lengths.
    private func applySkipIntervals() {
        let center = MPRemoteCommandCenter.shared()
        center.skipBackwardCommand.preferredIntervals = [NSNumber(value: backInterval)]
        center.skipForwardCommand.preferredIntervals = [NSNumber(value: forwardInterval)]
    }

    private func setUpRemoteCommands() {
        let center = MPRemoteCommandCenter.shared()
        applySkipIntervals()
        center.changePlaybackRateCommand.supportedPlaybackRates = [0.8, 1.0, 1.2, 1.5, 2.0].map { NSNumber(value: $0) }
        let commands: [(String, MPRemoteCommand, @MainActor (AudioPlayer, Double?) -> Void)] = [
            ("play", center.playCommand, { player, _ in player.play() }),
            ("pause", center.pauseCommand, { player, _ in player.pause() }),
            ("toggle play", center.togglePlayPauseCommand, { player, _ in player.togglePlay() }),
            ("skip back", center.skipBackwardCommand, { player, _ in player.skipBack() }),
            ("skip forward", center.skipForwardCommand, { player, _ in player.skipForward() }),
            // Headphones' double and triple click, and a car's track and
            // seek buttons, skip as the lock screen does rather than change
            // chapter; chapters change in the app and in CarPlay's Now Playing.
            ("next track", center.nextTrackCommand, { player, _ in player.skipForward() }),
            ("previous track", center.previousTrackCommand, { player, _ in player.skipBack() }),
            ("seek forward", center.seekForwardCommand, { player, _ in player.skipForward() }),
            ("seek back", center.seekBackwardCommand, { player, _ in player.skipBack() }),
            ("change position", center.changePlaybackPositionCommand, { player, time in
                if let time { player.seek(to: player.chapterBounds.start + time) }
            }),
            ("change rate", center.changePlaybackRateCommand, { player, rate in
                if let rate { player.setRate(rate) }
            }),
        ]
        for (name, command, action) in commands {
            AudioPlayer.onCommand(command) { [weak self] value in
                Task { @MainActor in
                    guard let self else { return }
                    // Which control sent what, as cars differ.
                    self.log("remote command: \(name)")
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
            case let event as MPSeekCommandEvent:
                if RemoteSkipRules.seekSkips(event.type == .beginSeeking ? .began : .ended) { handle(nil) }
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
            // No chapter number, chapter count, or media type: with the three,
            // a car's skip buttons never reached the remote commands (#917).
            // Which of them did it wasn't narrowed down.
        ]
        if let artwork {
            info[MPMediaItemPropertyArtwork] = AudioPlayer.nowPlayingArtwork(artwork)
        }
        MPNowPlayingInfoCenter.default().nowPlayingInfo = info
        MPNowPlayingInfoCenter.default().playbackState = isPlaying ? .playing : .paused
    }

    /// The cover's size in pixels for the lock screen and Now Playing, which
    /// downloading every cover fetches each File's own at.
    nonisolated static let artworkSize = 512

    /// The lock screen's and CarPlay's artwork: the File's own cover, the
    /// audiobook's, never its Book's, which can be an ebook's. A File with
    /// none shows the system's own.
    private func loadArtwork(_ loaded: ListeningFile) {
        artwork = nil
        guard !loaded.coverVersion.isEmpty else { return }
        let covers = session.covers
        Task { [weak self] in
            let image = await covers.image(.file(loaded.fileID), version: loaded.coverVersion, size: AudioPlayer.artworkSize)
            // Not for a File or a cover since replaced.
            guard let self, self.file?.fileID == loaded.fileID, self.file?.coverVersion == loaded.coverVersion else { return }
            self.artwork = image
            self.updateNowPlaying()
        }
    }

    private func log(_ message: String) {
        Task { await Diagnostics.shared.log(.player, message) }
    }
}
