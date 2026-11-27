import GRDB
import SwiftUI
import UIKit

/// What a Download is doing, in words, for its status line and VoiceOver.
enum DownloadWords {
    /// How far one has got, or why it waits, for its stop button's value.
    static func progress(_ status: DownloadStatus) -> String {
        switch status {
        case .running(let fraction?): fraction.formatted(.percent.precision(.fractionLength(0)))
        default: Self.status(status) ?? ""
        }
    }

    static func status(_ status: DownloadStatus) -> String? {
        switch status {
        case .notDownloaded: nil
        case .waiting(.wifi): String(localized: "Waiting for Wi-Fi")
        case .waiting(.connection): String(localized: "Waiting for a connection")
        case .waiting(.turn): String(localized: "Waiting to download")
        case .running(let fraction?): String(localized: "Downloading · \(fraction.formatted(.percent.precision(.fractionLength(0))))")
        case .running(nil): String(localized: "Downloading")
        case .downloaded: String(localized: "Downloaded")
        case .failed(let message): String(localized: "Download failed: \(message)")
        }
    }
}

extension DownloadWords {
    /// A Download's line under its Book's title: the File's own name when
    /// it isn't the Book's, as an edition's is, its format, and how far
    /// through it the user is, to tell which can go when space runs low.
    static func facts(_ item: StoredDownload) -> String {
        var parts: [String] = []
        if item.label.localizedCaseInsensitiveCompare(item.book.title) != .orderedSame {
            parts.append(item.label)
        }
        parts.append(FileTypes.label(item.fileType))
        parts.append(progress(item))
        return parts.joined(separator: " · ")
    }

    private static func progress(_ item: StoredDownload) -> String {
        if item.book.status == "read" || (item.progress ?? 0) >= 0.995 {
            return String(localized: "Finished")
        }
        guard let progress = item.progress, progress > 0 else {
            return String(localized: "Not started")
        }
        let percent = progress.formatted(.percent.precision(.fractionLength(0)))
        return item.fileType == "m4b" ? String(localized: "Listening \(percent)") : String(localized: "Reading \(percent)")
    }
}

/// A File's Download button, beside Listen and Read on its Book's page:
/// Download, a ring that fills while it downloads and stops it when
/// tapped, or a check with a menu to remove it. A File Type this version
/// doesn't know can't be downloaded.
struct DownloadControl: View {
    @Environment(AppModel.self) private var model
    let fileID: Int
    let label: String
    let supported: Bool
    let row: DownloadRow?

    private var status: DownloadStatus {
        model.session?.downloads.status(row) ?? .notDownloaded
    }

    var body: some View {
        Group {
            switch status {
            case .notDownloaded, .failed:
                Button { model.session?.downloads.download([fileID]) } label: {
                    Icon(.download, size: 18, relativeTo: nil)
                        .foregroundStyle(supported ? Theme.primary : Theme.mutedForeground)
                        .controlCircle(tinted: supported)
                }
                .disabled(!supported)
                .accessibilityLabel(supported ? String(localized: "Download \(label)") : String(localized: "Download \(label), not supported in this version of the app"))
                .accessibilityValue(DownloadWords.status(status) ?? "")
                // Downloading needs the server, as listening and reading do.
                .modifier(NeedsConnection(available: !supported || model.connection == .online))
            case .waiting, .running:
                let ring = DownloadRing(fraction: { if case .running(let fraction) = status { return fraction } else { return nil } }())
                    .controlCircle(tinted: true)
                if row?.replacing == true {
                    // A replaced File's new copy comes with its old one
                    // still here, which stopping it removes too, so it asks.
                    Menu {
                        Button(role: .destructive) { model.session?.downloads.remove([fileID]) } label: {
                            Label("Remove Download", menuIcon: .trash)
                        }
                    } label: {
                        ring
                    }
                    .accessibilityLabel(String(localized: "Downloading \(label) again"))
                    .accessibilityValue(DownloadWords.progress(status))
                } else {
                    Button { model.session?.downloads.remove([fileID]) } label: {
                        ring
                    }
                    .accessibilityLabel(String(localized: "Stop downloading \(label)"))
                    .accessibilityValue(DownloadWords.progress(status))
                }
            case .downloaded:
                Menu {
                    Button(role: .destructive) { model.session?.downloads.remove([fileID]) } label: {
                        Label("Remove Download", menuIcon: .trash)
                    }
                    .accessibilityIdentifier("file.removeDownload")
                } label: {
                    Icon(.check, size: 18, relativeTo: nil)
                        .foregroundStyle(Theme.primary)
                        .controlCircle(tinted: true)
                }
                .accessibilityLabel(String(localized: "\(label) is downloaded"))
                .accessibilityHint(String(localized: "Shows a menu to remove the Download"))
            }
        }
        .buttonStyle(PlayerControlStyle())
        // Editions can share a name, so the File's id tells them apart.
        .accessibilityIdentifier("file.download.\(fileID)")
    }
}

/// A line saying where a File's Download stands, under its facts. Nothing
/// without one.
struct DownloadStatusLine: View {
    @Environment(AppModel.self) private var model
    let fileID: Int
    let row: DownloadRow?

    var body: some View {
        let status = model.session?.downloads.status(row) ?? .notDownloaded
        if let words = DownloadWords.status(status) {
            Label(verbatim: words, icon: icon(status), size: 13, relativeTo: .footnote)
                .font(.geist(.footnote, weight: .medium))
                .foregroundStyle(isFailure(status) ? Theme.destructive : Theme.mutedForeground)
                .lineLimit(3)
                .monospacedDigit()
                .accessibilityIdentifier("file.downloadStatus.\(fileID)")
        }
    }

    private func icon(_ status: DownloadStatus) -> Lucide {
        switch status {
        case .downloaded: .check
        case .failed: .circleAlert
        case .waiting(.wifi): .wifi
        case .waiting(.connection): .wifiOff
        default: .download
        }
    }

    private func isFailure(_ status: DownloadStatus) -> Bool {
        if case .failed = status { return true }
        return false
    }
}

/// An action button's icon beside its title, or above it at accessibility
/// text sizes, where a title that wraps beside its icon reads as indented.
struct ActionLabelStyle: LabelStyle {
    func makeBody(configuration: LabelStyleConfiguration) -> some View {
        ActionLabel(configuration: configuration)
    }
}

private struct ActionLabel: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let configuration: LabelStyleConfiguration

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: 6) {
                configuration.icon
                configuration.title
                    .multilineTextAlignment(.center)
            }
            .padding(.vertical, 10)
        } else {
            HStack(spacing: 8) {
                configuration.icon
                configuration.title
            }
        }
    }
}

/// A ring that fills as a Download goes, around a stop square; it turns
/// while the size isn't known or the Download waits.
struct DownloadRing: View {
    let fraction: Double?
    @State private var turning = false

    var body: some View {
        ZStack {
            Circle()
                .stroke(Theme.primary.opacity(0.2), lineWidth: 2.5)
            Circle()
                .trim(from: 0, to: fraction.map { max(0.02, $0) } ?? 0.25)
                .stroke(Theme.primary, style: StrokeStyle(lineWidth: 2.5, lineCap: .round))
                .rotationEffect(.degrees(fraction == nil && turning ? 270 : -90))
                .animation(fraction == nil ? .linear(duration: 1).repeatForever(autoreverses: false) : .easeOut(duration: 0.2),
                           value: fraction == nil && turning)
                .animation(.easeOut(duration: 0.2), value: fraction)
            RoundedRectangle(cornerRadius: 1.5)
                .fill(Theme.primary)
                .frame(width: 7, height: 7)
        }
        .frame(width: 20, height: 20)
        .onAppear { turning = true }
        .accessibilityHidden(true)
    }
}

private extension View {
    /// The round-cornered square the Book page's File actions sit in.
    func controlCircle(tinted: Bool) -> some View {
        frame(width: 40, height: 40)
            .background((tinted ? Theme.primary : Theme.mutedForeground).opacity(0.12),
                        in: RoundedRectangle(cornerRadius: 10, style: .continuous))
            .frame(minWidth: 44, minHeight: 44)
            .contentShape(Rectangle())
    }
}

/// Download All for a Series or a List, in its toolbar: one tap downloads
/// the main Files of every Book in it. It says when they're all downloaded.
/// Its page observes `Files` and adds it once there's something to
/// download: a toolbar item that starts out empty never runs its own tasks.
struct DownloadAllButton: View {
    struct Files: Equatable, Sendable {
        let fileIDs: [Int]
        /// How many have no Download, or one that failed.
        let missing: Int

        static func fetch(_ db: Database, seriesID: Int? = nil, listID: Int? = nil) throws -> Files {
            let ids = try CatalogStore.downloadableFiles(db, seriesID: seriesID, listID: listID)
            return Files(fileIDs: ids, missing: try CatalogStore.notDownloaded(db, fileIDs: ids))
        }
    }

    @Environment(AppModel.self) private var model
    let files: Files

    var body: some View {
        Button {
            model.session?.downloads.download(files.fileIDs)
        } label: {
            Icon(files.missing == 0 ? .check : .download, size: 20, relativeTo: nil)
                .foregroundStyle(files.missing == 0 ? Theme.mutedForeground : Theme.foreground)
        }
        .disabled(files.missing == 0)
        // Queued counts too: what's on its way needs nothing more.
        .accessibilityLabel(files.missing == 0 ? String(localized: "All downloaded or on their way") : String(localized: "Download All"))
        .accessibilityValue(files.missing == 0 ? "" : String(AttributedString(localized: "^[\(files.missing) file](inflect: true)").characters))
        .accessibilityIdentifier("downloadAll")
        .modifier(NeedsConnection(available: files.missing == 0 || model.connection == .online))
    }
}

/// Turns off a control that needs the server while it can't be reached,
/// such as Listen for an audiobook that isn't downloaded, in the web app's
/// disabled style: half opacity, and no taps. The strip above the tabs
/// says why.
struct NeedsConnection: ViewModifier {
    let available: Bool

    func body(content: Content) -> some View {
        if available {
            content
        } else {
            content
                .disabled(true)
                .opacity(0.5)
                .accessibilityHint(String(localized: "Needs a connection to your server"))
        }
    }
}

// MARK: - Sharing

/// Shares a File out of the app through the system share sheet, as its
/// Generated File, so the copy carries Shisho's current metadata (ADR
/// 0022). The server builds it first, which can take a while for a large
/// File, so it says it's preparing and can be cancelled.
struct ShareFileButton: View {
    @Environment(AppModel.self) private var model
    let fileID: Int
    let label: String
    @State private var preparing: Task<Void, Never>?
    /// Which share is current, so a cancelled one that ends late doesn't
    /// clear the one after it.
    @State private var attempt = 0
    @State private var failure: String?

    var body: some View {
        Button {
            if let preparing {
                preparing.cancel()
                self.preparing = nil
            } else {
                share()
            }
        } label: {
            if preparing != nil {
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Preparing…")
                }
            } else {
                Label("Share", icon: .share)
            }
        }
        .buttonStyle(OutlineButtonStyle())
        .accessibilityLabel(preparing != nil ? String(localized: "Preparing \(label) to share. Cancel") : String(localized: "Share \(label)"))
        .accessibilityIdentifier("file.share")
        // A share still preparing when the page goes, or another account
        // takes over, is dropped rather than shown over another screen.
        .onDisappear {
            preparing?.cancel()
            preparing = nil
        }
        .alert(String(localized: "Couldn't share this File"), isPresented: Binding(get: { failure != nil }, set: { if !$0 { failure = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(failure ?? "")
        }
    }

    private func share() {
        guard let session = model.session else { return }
        failure = nil
        attempt += 1
        let current = attempt
        preparing = Task {
            defer { if attempt == current { preparing = nil } }
            do {
                let file = try await session.client.shareFile(fileID: fileID)
                guard !Task.isCancelled, model.session === session else {
                    try? FileManager.default.removeItem(at: file.deletingLastPathComponent())
                    return
                }
                SharePresenter.present(file)
            } catch is CancellationError {
            } catch {
                await Diagnostics.shared.log(.downloads, "file \(fileID): couldn't share it: \(Diagnostics.describe(error))")
                // Not for a share cancelled, or replaced, meanwhile.
                guard !Task.isCancelled, attempt == current, model.session === session else { return }
                failure = (error as? APIError).map { Messages.signIn($0) } ?? error.localizedDescription
            }
        }
    }
}

/// Presents the system share sheet for a file from wherever the app is,
/// and deletes the file's folder once the sheet closes.
@MainActor
enum SharePresenter {
    static func present(_ file: URL) {
        let sheet = UIActivityViewController(activityItems: [file], applicationActivities: nil)
        sheet.completionWithItemsHandler = { _, _, _, _ in
            try? FileManager.default.removeItem(at: file.deletingLastPathComponent())
        }
        guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first,
              let window = scene.windows.first(where: \.isKeyWindow) ?? scene.windows.first,
              var top = window.rootViewController
        else {
            try? FileManager.default.removeItem(at: file.deletingLastPathComponent())
            return
        }
        while let presented = top.presentedViewController { top = presented }
        // On iPad the sheet is a popover, which needs somewhere to point.
        if let popover = sheet.popoverPresentationController {
            popover.sourceView = window
            popover.sourceRect = CGRect(x: window.bounds.midX, y: window.bounds.midY, width: 0, height: 0)
            popover.permittedArrowDirections = []
        }
        top.present(sheet, animated: true)
    }
}

// MARK: - The Downloads tab

/// Everything downloaded and on its way, the space it takes, and the
/// Download settings: the Storage screen, as the prototype's Downloads tab
/// (variant E). Removing a Download frees its space at once.
struct DownloadsView: View {
    @Environment(AppModel.self) private var model
    @State private var downloads: [StoredDownload]?
    @State private var confirmingRemoveAll = false
    @State private var scrolledBy: CGFloat = 0

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.downloadsPath) {
            ScrollViewReader { proxy in
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        ScrollTopMarker(space: "downloads")
                            .id(ScrollTop.anchor)
                        // With the heading, so they add no gap when there's
                        // nothing to say.
                        VStack(alignment: .leading, spacing: 0) {
                            TabHeading(title: String(localized: "Downloads"))
                                .padding(.top, 4)
                            AppNotices()
                        }
                        .padding(.bottom, -8)
                        if let downloads {
                            summary(downloads)
                            if downloads.isEmpty {
                                EmptyNotice(title: String(localized: "No Downloads"),
                                            message: String(localized: "Download audiobooks and ebooks from a Book's page to listen and read offline."),
                                            icon: .download)
                                    .accessibilityIdentifier("downloads.empty")
                            } else {
                                section(String(localized: "Downloading"), downloads.filter { $0.state != .done })
                                section(String(localized: "Downloaded"), downloads.filter { $0.state == .done })
                                Button(role: .destructive) { confirmingRemoveAll = true } label: {
                                    Label("Remove All Downloads", icon: .trash)
                                }
                                .buttonStyle(OutlineButtonStyle(role: .destructive))
                                .labelStyle(ActionLabelStyle())
                                .accessibilityIdentifier("downloads.removeAll")
                                // On the button, so where iOS shows it as a
                                // popover, it points at the button.
                                .confirmationDialog(String(localized: "Remove every Download from this device?"),
                                                    isPresented: $confirmingRemoveAll, titleVisibility: .visible) {
                                    Button("Remove All Downloads", role: .destructive) { model.session?.downloads.removeAll() }
                                        .accessibilityIdentifier("downloads.removeAll.confirm")
                                } message: {
                                    Text("They stay in your library, and you can redownload them later.")
                                }
                            }
                        }
                    }
                    .padding(.horizontal, GridLayout.margin)
                    .padding(.bottom, 32)
                    .frame(maxWidth: 680)
                    .frame(maxWidth: .infinity)
                }
                .onScrolledDistanceChange(space: "downloads") { distance in
                    // Only as far as the bar's logo or title changes, so the
                    // page isn't redrawn on every frame of a long scroll.
                    let settled = min(LogoToolbarItem.settledAfter, distance)
                    if settled != scrolledBy { scrolledBy = settled }
                }
                .miniPlayerClearance()
                .onChange(of: model.scrollToTop[.downloads]) { _, _ in
                    withAnimation { proxy.scrollTo(ScrollTop.anchor, anchor: .top) }
                }
            }
            .background(Theme.background)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                LogoToolbarItem(scrolledBy: scrolledBy, title: String(localized: "Downloads"))
                RootToolbar(downloadSettings: true)
            }
            .routeDestinations()
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $downloads) { db in
            try CatalogStore.storedDownloads(db)
        }
    }

    /// The space Downloads take, and what's left on the device.
    private func summary(_ downloads: [StoredDownload]) -> some View {
        let used = DownloadManager.totalSize(downloads)
        let free = (try? URL(fileURLWithPath: NSHomeDirectory())
            .resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey]).volumeAvailableCapacityForImportantUsage) ?? nil
        let count = downloads.filter { $0.state == .done }.count
        let coming = downloads.filter { $0.state == .queued }.count
        // The size is what's on the device; ones on their way say so beside
        // the count, rather than leaving "0 downloads" while one runs.
        var parts = [Counts.downloads(count)]
        if coming > 0 { parts.append(String(localized: "\(coming) on the way")) }
        if let free { parts.append(String(localized: "\(FileFacts.size(Int(free))) available")) }
        return VStack(alignment: .leading, spacing: 4) {
            Text(FileFacts.size(used))
                .font(.geist(size: 30, relativeTo: .title, weight: .bold))
                .monospacedDigit()
                .accessibilityIdentifier("downloads.used")
            Text(parts.joined(separator: " · "))
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private func section(_ title: String, _ items: [StoredDownload]) -> some View {
        if !items.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                Text(title)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityAddTraits(.isHeader)
                VStack(spacing: 0) {
                    ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                        if index > 0 { Divider().overlay(Theme.border) }
                        DownloadRowView(item: item)
                    }
                }
                .cardStyle()
            }
        }
    }
}

/// The Download settings, on a page of their own behind the Downloads
/// tab's gear, since they're set once and rarely changed.
struct DownloadSettingsView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        ScrollView {
            if let downloads = model.session?.downloads {
                DownloadSettingsCard(downloads: downloads)
                    .padding(.horizontal, GridLayout.margin)
                    .padding(.top, 8)
                    .padding(.bottom, 32)
                    .frame(maxWidth: 680)
                    .frame(maxWidth: .infinity)
            }
        }
        .miniPlayerClearance()
        .background(Theme.background)
        .navigationTitle("Download Settings")
        .navigationBarTitleDisplayMode(.inline)
    }
}

/// Wi-Fi only, Remove when finished, and covers, for every account on the
/// device.
private struct DownloadSettingsCard: View {
    @Environment(AppModel.self) private var model
    @Bindable var downloads: DownloadManager

    var body: some View {
        @Bindable var model = model
        SettingsCard {
            SettingToggle(title: "Download on Wi-Fi only",
                          description: downloads.wifiOnly && downloads.wait == .wifi
                              ? "Downloads wait until you're on Wi-Fi."
                              : "Keeps Downloads off cellular data and Low Data Mode.",
                          isOn: $downloads.wifiOnly, identifier: "downloads.wifiOnly",
                          emphasized: downloads.wifiOnly && downloads.wait == .wifi)
            SettingsDivider()
            SettingToggle(title: "Remove when finished",
                          description: "A Download is removed once you listen to or read it to the end.",
                          isOn: $downloads.removeWhenFinished, identifier: "downloads.removeWhenFinished")
            SettingsDivider()
            SettingToggle(title: "Download the top of To Read",
                          description: "The first Books on To Read download by themselves, with all their Files, so what's next is ready offline.",
                          isOn: Binding(get: { downloads.toReadCount > 0 },
                                        set: { downloads.toReadCount = $0 ? ToReadCountStepper.defaultCount : 0 }),
                          identifier: "downloads.toRead")
            if downloads.toReadCount > 0 {
                ToReadCountStepper(count: $downloads.toReadCount)
            }
            SettingsDivider()
            SettingToggle(title: "Download all covers on Wi-Fi",
                          description: "Every cover in your Libraries shows offline. Otherwise covers download as you browse.",
                          isOn: $model.prefetchesCovers, identifier: "downloads.prefetchCovers")
        }
    }
}

/// How many Books from the top of To Read download, under the setting that
/// turns it on.
private struct ToReadCountStepper: View {
    static let defaultCount = 3
    @Binding var count: Int

    var body: some View {
        Stepper(value: $count, in: DownloadSettings.toReadCountRange) {
            Group {
                if count == 1 { Text("1 Book") } else { Text("\(count) Books") }
            }
            .font(.geist(.subheadline))
            .foregroundStyle(Theme.foreground)
            .monospacedDigit()
        }
        .frame(minHeight: 44)
        .padding(.top, 6)
        .accessibilityLabel(Text("Books to download from To Read"))
        .accessibilityValue(Text("\(count)"))
        .accessibilityIdentifier("downloads.toReadCount")
    }
}

/// One Download: its cover, Book, File, and size or progress, opening its
/// Book, with a button to remove it.
private struct DownloadRowView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let item: StoredDownload

    var body: some View {
        let row = DownloadRow(fileID: item.fileID, fileType: item.fileType, state: item.state, sizeBytes: item.sizeBytes,
                              error: item.error)
        let status = model.session?.downloads.status(row) ?? .downloaded
        HStack(spacing: 12) {
            Button { model.push(.book(item.book)) } label: {
                HStack(spacing: 12) {
                    // The File's own cover, so the row shows the edition it is.
                    CoverView(book: item.book, file: CoverFile(id: item.fileID, fileType: item.fileType, coverVersion: item.coverVersion),
                              width: item.fileType == "m4b" ? 44 : 34)
                        .frame(width: 44)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(item.book.title)
                            .font(.geist(.subheadline, weight: .semibold))
                            .lineLimit(2)
                        Text(verbatim: DownloadWords.facts(item))
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            // Room to wrap at large text, so the format isn't cut off.
                            .lineLimit(dynamicTypeSize.isAccessibilitySize ? 4 : 1)
                        Text(item.state == .done ? FileFacts.size(item.sizeBytes) : DownloadWords.status(status) ?? "")
                            .font(.geist(.footnote))
                            .foregroundStyle(item.state == .failed ? Theme.destructive : Theme.mutedForeground)
                            .monospacedDigit()
                            .lineLimit(3)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityHint(String(localized: "Opens the Book"))
            .accessibilityIdentifier("downloads.row")

            Button { model.session?.downloads.remove([item.fileID]) } label: {
                Icon(item.state == .done || item.replacing ? .trash : .x, size: 18, relativeTo: nil)
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(width: 44, height: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(item.state == .done || item.replacing
                                ? String(localized: "Remove the Download of \(item.book.title), \(item.label)")
                                : String(localized: "Stop downloading \(item.book.title), \(item.label)"))
            .accessibilityIdentifier("downloads.remove")
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .contextMenu {
            Button(role: .destructive) { model.session?.downloads.remove([item.fileID]) } label: {
                Label(item.state == .done || item.replacing ? "Remove Download" : "Stop Download", menuIcon: .trash)
            }
        }
    }
}
