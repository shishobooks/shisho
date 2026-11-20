import Observation

/// Runs one sync at a time. A sync asked for while one runs isn't dropped:
/// it runs once more after the one in flight, and every caller waits for
/// that. Stopping, on sign-out or when the network goes, cancels the sync
/// in flight and marks its result stale, so a late answer never lands on
/// another account or hides that the device is offline.
@MainActor
@Observable
final class SyncRunner {
    private(set) var isRunning = false

    @ObservationIgnored private var task: Task<Void, Never>?
    @ObservationIgnored private var again = false
    /// Which run is current; stopping moves it on.
    @ObservationIgnored private var generation = 0

    /// Runs work, or joins the run in flight and has it run once more.
    /// work gets isCurrent, false once the run was stopped: check it before
    /// applying a result.
    func run(_ work: @escaping @MainActor (_ isCurrent: @escaping @MainActor () -> Bool) async -> Void) async {
        if let task {
            again = true
            await task.value
            return
        }
        generation += 1
        let mine = generation
        let isCurrent: @MainActor @Sendable () -> Bool = { [weak self] in self?.generation == mine }
        isRunning = true
        let task = Task {
            repeat {
                again = false
                await work(isCurrent)
            } while again && isCurrent() && !Task.isCancelled
        }
        self.task = task
        await task.value
        if isCurrent() {
            self.task = nil
            isRunning = false
        }
    }

    /// Cancels the run in flight and drops its result.
    func stop() {
        task?.cancel()
        task = nil
        again = false
        generation += 1
        isRunning = false
    }
}
