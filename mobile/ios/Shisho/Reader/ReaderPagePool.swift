import Darwin
import UIKit

/// Reader pages booted before a book opens (ADR 0026). A page's web content
/// process takes seconds to start and to load what WebKit needs for a
/// book's XHTML; done while the user looks at a Book's page, the reader
/// opens on pages that are already up, and only lays out the book.
@MainActor
final class ReaderPagePool {
    static let shared = ReaderPagePool()

    /// One reader's pages and the handler that serves them its book.
    struct Pages {
        let handler: ReaderSchemeHandler
        let back: ReaderPage
        let current: ReaderPage
        let forward: ReaderPage
    }

    private var ready: Pages?
    private var watches: [Task<Void, Never>] = []
    /// Lets waiting pages go once no Book with an ebook has shown for a
    /// while, so browsing on doesn't keep three web content processes up.
    private var idle: Task<Void, Never>?
    private static let idleDrain: Duration = .seconds(90)

    private init() {
        for name in [UIApplication.didReceiveMemoryWarningNotification, UIApplication.didEnterBackgroundNotification] {
            // Made here, so it listens from now rather than once the task
            // starts.
            let notifications = NotificationCenter.default.notifications(named: name)
            watches.append(Task { [weak self] in
                for await _ in notifications {
                    self?.drain()
                }
            })
        }
    }

    /// WebKit asks QuickLook whether it previews a document that isn't HTML
    /// or plain text, as each of a book's XHTML sections is, in the app's
    /// own process too, on the main thread. The first ask loads QuickLook,
    /// which holds the main thread for seconds the first time a book opens
    /// after launch. Loading it on a background thread first leaves
    /// WebKit's load nothing to do. (WebCore's PreviewConverter::supportsMIMEType.)
    nonisolated static func loadPreviewSupport() {
        DispatchQueue.global(qos: .utility).async {
            _ = dlopen("/System/Library/Frameworks/QuickLook.framework/QuickLook", RTLD_LAZY)
        }
    }
    /// Where waiting pages sit: in the app's window, invisible and behind
    /// everything, since WebKit holds back a page that isn't in a window.
    private let parking: UIView = {
        let view = UIView()
        view.alpha = 0
        view.isUserInteractionEnabled = false
        view.accessibilityElementsHidden = true
        return view
    }()

    /// Boots a reader's pages if none are waiting, as when a Book with an
    /// ebook shows.
    func prewarm() {
        idle?.cancel()
        idle = Task { [weak self] in
            try? await Task.sleep(for: Self.idleDrain)
            guard !Task.isCancelled else { return }
            self?.drain()
        }
        guard ready == nil else { return }
        let pages = Self.makePages()
        for page in [pages.back, pages.current, pages.forward] {
            // A waiting page whose process ended is no use; boot again next
            // time.
            page.onTerminate = { [weak self] in self?.drain() }
        }
        ready = pages
        park(pages)
    }

    private func park(_ pages: Pages) {
        let window = UIApplication.shared.connectedScenes
            .compactMap { ($0 as? UIWindowScene)?.keyWindow }
            .first
        guard let window else { return }
        parking.frame = window.bounds
        window.insertSubview(parking, at: 0)
        for page in [pages.back, pages.current, pages.forward] {
            page.webView.frame = parking.bounds
            parking.addSubview(page.webView)
        }
    }

    /// The waiting pages, or new ones when there are none.
    func take() -> Pages {
        idle?.cancel()
        if let ready {
            self.ready = nil
            parking.removeFromSuperview()
            return ready
        }
        return Self.makePages()
    }

    /// Pages are booted and waiting.
    var isWarm: Bool { ready != nil }

    /// Lets the waiting pages go, as under memory pressure or in the
    /// background.
    func drain() {
        ready = nil
        parking.subviews.forEach { $0.removeFromSuperview() }
        parking.removeFromSuperview()
    }

    private static func makePages() -> Pages {
        let handler = ReaderSchemeHandler()
        return Pages(handler: handler, back: ReaderPage(handler: handler), current: ReaderPage(handler: handler),
                     forward: ReaderPage(handler: handler))
    }
}
