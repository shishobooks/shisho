import UIKit
import WebKit

/// Serves the reader's pages over one origin: its own script and the web
/// reader's vendored foliate-js from the app bundle, and the book from the
/// device (ADR 0026).
@MainActor
final class ReaderSchemeHandler: NSObject, WKURLSchemeHandler {
    static let scheme = "shisho-reader"
    /// The page every WebView loads.
    static let pageURL = URL(string: "\(scheme)://app/ReaderWeb/reader.html")!
    /// Where the page fetches the book from.
    static let bookURL = "\(scheme)://app/book"

    /// The book's file on the device.
    var bookFile: URL? {
        didSet { book = nil }
    }
    /// The book, mapped rather than read: a page asks for a part at a time,
    /// and only the parts it asks for come into memory.
    private var book: Data?

    func webView(_ webView: WKWebView, start task: any WKURLSchemeTask) {
        guard let url = task.request.url else { return }
        if url.path == "/book" {
            serveBook(task, url: url)
            return
        }
        // Only the bundle's own resources, never a path out of it.
        let path = url.path.split(separator: "/").filter { $0 != ".." }.joined(separator: "/")
        guard let file = Bundle.main.resourceURL?.appendingPathComponent(path),
              let data = try? Data(contentsOf: file, options: .mappedIfSafe)
        else {
            task.didFailWithError(URLError(.fileDoesNotExist))
            return
        }
        let types = ["html": "text/html", "js": "text/javascript", "css": "text/css"]
        respond(task, url: url, status: 200, data: data, headers: ["Content-Type": types[file.pathExtension] ?? "application/octet-stream"])
    }

    /// The part of the book the page asked for with a Range header, as the
    /// web reader reads one from the server (app/libraries/rangeFile.ts), so
    /// no WebView holds a whole book: a photo cookbook runs to hundreds of
    /// megabytes, and three WebViews each read it.
    private func serveBook(_ task: any WKURLSchemeTask, url: URL) {
        if book == nil, let bookFile {
            book = try? Data(contentsOf: bookFile, options: .alwaysMapped)
        }
        guard let book else {
            task.didFailWithError(URLError(.fileDoesNotExist))
            return
        }
        guard let range = ByteRange.parse(task.request.value(forHTTPHeaderField: "Range"), size: book.count) else {
            respond(task, url: url, status: 416, data: Data(), headers: ["Content-Range": "bytes */\(book.count)"])
            return
        }
        respond(task, url: url, status: 206, data: book.subdata(in: range), headers: [
            "Content-Type": "application/octet-stream",
            "Content-Range": "bytes \(range.lowerBound)-\(range.upperBound - 1)/\(book.count)",
        ])
    }

    private func respond(_ task: any WKURLSchemeTask, url: URL, status: Int, data: Data, headers: [String: String]) {
        var fields = headers
        fields["Content-Length"] = String(data.count)
        let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: "HTTP/1.1", headerFields: fields)!
        task.didReceive(response)
        task.didReceive(data)
        task.didFinish()
    }

    func webView(_ webView: WKWebView, stop task: any WKURLSchemeTask) {}
}

/// A single range from an HTTP Range header, as the reader's page asks for
/// part of the book.
enum ByteRange {
    /// The bytes header asks for of a file of size bytes, or nil for one
    /// that can't be served: none, several, or one past the end.
    static func parse(_ header: String?, size: Int) -> Range<Int>? {
        guard let header, header.hasPrefix("bytes="), size > 0 else { return nil }
        let spec = header.dropFirst("bytes=".count)
        guard !spec.contains(","), let dash = spec.firstIndex(of: "-") else { return nil }
        let first = spec[..<dash], last = spec[spec.index(after: dash)...]
        if first.isEmpty {
            guard let count = Int(last), count > 0 else { return nil }
            return max(0, size - count)..<size
        }
        guard let start = Int(first), start < size else { return nil }
        if last.isEmpty { return start..<size }
        guard let end = Int(last), end >= start else { return nil }
        return start..<min(size, end + 1)
    }
}

/// Where a page is in the book, as its script reports it.
struct PageLocation: Decodable, Equatable, Sendable {
    var cfi: String?
    var fraction: Double?
    /// Where the page starts along the book; fraction is where it ends.
    var start: Double?
    /// Section index, page, and page count, as foliate's paginator counts
    /// them: one page from another means the same layout.
    var index: Int?
    var page: Int?
    var pages: Int?
    /// The reader is at the end of the book, which finishes the
    /// Read-through (app/utils/readerEnd.ts).
    var atEnd: Bool?
    /// Which screen shows, for timing reading per page.
    var screen: String?
    /// The links on the page, in the WebView's points: x, y, width,
    /// height, and the id the script knows the link by.
    var links: [[Double]]?

    var base: [Int]? {
        guard let index, let page, let pages else { return nil }
        return [index, page, pages]
    }
}

/// One message from a page's script.
struct PageMessage: Decodable, Sendable {
    struct TocItem: Decodable, Equatable, Sendable {
        let label: String
        let href: String
        let depth: Int
    }

    struct ContentsEntry: Decodable, Equatable, Sendable {
        /// Where the chapter is in this book, as foliate goes there.
        let target: String?
        /// Where it starts along the book, from 0 to 1.
        let position: Double?
    }

    let type: String
    var token: Int?
    var loc: PageLocation?
    /// The current page's [index, page, pages] a neighbor stepped from.
    var base: [Int]?
    var edge: Bool?
    var reason: String?
    /// The move came from opening or syncing the page, not the reader.
    var quiet: Bool?
    var toc: [TocItem]?
    var rtl: Bool?
    var fixedLayout: Bool?
    var contents: [ContentsEntry]?
    var active: Bool?
    var paragraphs: [String]?
    /// The note reference's box, in the WebView's points.
    var rect: [Double]?
    var href: String?
    var dir: Int?
    var message: String?
}

/// Forwards the page's messages without the content controller keeping
/// the page alive.
@MainActor
private final class MessageProxy: NSObject, WKScriptMessageHandler {
    weak var page: ReaderPage?

    init(_ page: ReaderPage) {
        self.page = page
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        // Only the reader's own script, never a book's frames.
        guard message.frameInfo.isMainFrame, let body = message.body as? String else { return }
        page?.receive(body)
    }
}

/// One WebView running foliate through ReaderWeb/reader.js.
@MainActor
final class ReaderPage: NSObject, WKNavigationDelegate {
    let webView: WKWebView
    var onMessage: ((PageMessage) -> Void)?
    /// WebKit ended the page's content process, as under memory pressure.
    var onTerminate: (() -> Void)?
    private var booted = false
    private var pending: [String] = []

    /// The last place the page reported.
    private(set) var location = PageLocation()
    private(set) var hasSelection = false

    init(handler: ReaderSchemeHandler, background: UIColor) {
        let configuration = WKWebViewConfiguration()
        configuration.setURLSchemeHandler(handler, forURLScheme: ReaderSchemeHandler.scheme)
        configuration.suppressesIncrementalRendering = false
        webView = WKWebView(frame: .zero, configuration: configuration)
        super.init()
        configuration.userContentController.add(MessageProxy(self), name: "bridge")
        webView.navigationDelegate = self
        webView.isOpaque = false
        webView.backgroundColor = background
        webView.scrollView.backgroundColor = background
        webView.scrollView.isScrollEnabled = false
        webView.scrollView.bounces = false
        webView.scrollView.contentInsetAdjustmentBehavior = .never
        // A tap near a link is the pager's to judge, and link previews
        // would take long presses meant for selection.
        webView.allowsLinkPreview = false
        #if DEBUG
        webView.isInspectable = true
        #endif
        load()
    }

    private func load() {
        booted = false
        webView.load(URLRequest(url: ReaderSchemeHandler.pageURL))
    }

    func setBackground(_ color: UIColor) {
        webView.backgroundColor = color
        webView.scrollView.backgroundColor = color
    }

    /// Runs js once the page's script is up.
    func call(_ js: String) {
        if booted {
            webView.evaluateJavaScript(js, completionHandler: nil)
        } else {
            pending.append(js)
        }
    }

    /// Runs js and returns what it gives back, or nil before the script is
    /// up.
    func evaluate(_ js: String) async -> Any? {
        guard booted else { return nil }
        return try? await webView.evaluateJavaScript(js)
    }

    /// Starts over after the content process ended: the next open runs in a
    /// fresh page.
    func reload() {
        pending = []
        location = PageLocation()
        hasSelection = false
        load()
    }

    fileprivate func receive(_ body: String) {
        guard let message = try? JSONDecoder().decode(PageMessage.self, from: Data(body.utf8)) else { return }
        switch message.type {
        case "boot":
            booted = true
            for js in pending {
                webView.evaluateJavaScript(js, completionHandler: nil)
            }
            pending = []
            return
        case "selection":
            hasSelection = message.active ?? false
        default:
            break
        }
        if let loc = message.loc {
            location = loc
        }
        onMessage?(message)
    }

    func clearSelection() {
        hasSelection = false
        call("R.clearSelection()")
    }

    /// The link nearest to point (in the WebView's points) within slop, from
    /// the boxes the page last reported, so a tap needs no round trip.
    func link(near point: CGPoint, slop: CGFloat) -> Int? {
        var best: (id: Int, distance: CGFloat)?
        for box in location.links ?? [] where box.count == 5 {
            let rect = CGRect(x: box[0], y: box[1], width: box[2], height: box[3])
            let dx = max(rect.minX - point.x, 0, point.x - rect.maxX)
            let dy = max(rect.minY - point.y, 0, point.y - rect.maxY)
            let distance = hypot(dx, dy)
            if distance <= slop, distance < (best?.distance ?? .infinity) {
                best = (Int(box[4]), distance)
            }
        }
        return best?.id
    }

    // MARK: - WKNavigationDelegate

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        onTerminate?()
    }
}

/// A string as a JavaScript literal.
func javaScriptLiteral(_ value: String) -> String {
    let data = (try? JSONSerialization.data(withJSONObject: [value])) ?? Data("[\"\"]".utf8)
    return String(String(decoding: data, as: UTF8.self).dropFirst().dropLast())
}
