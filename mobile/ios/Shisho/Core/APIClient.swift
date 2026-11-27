import Foundation
import Network
import os
import ShishoAPI

/// Why a request to a Shisho server didn't succeed, in the terms the app
/// explains to the user.
enum APIError: Error, Equatable {
    /// The server's Shisho predates the Shisho app.
    case serverPredatesApp
    /// The server needs a newer app. serverVersion is known once signed in.
    case appTooOld(serverVersion: String?)
    /// The app needs a newer server.
    case serverTooOld(serverVersion: String?)
    /// A Demo Mode server, which the app doesn't connect to.
    case demoMode
    /// The username or password is wrong.
    case wrongPassword
    /// The account's password must be changed on the web first.
    case passwordResetRequired
    /// The account can't read Books.
    case noAccess
    /// The Device was revoked, or its user deactivated: sign in again.
    case signedOut
    /// The server at the account's address isn't the one the account was
    /// made on, such as one set up again from scratch: sign in again.
    case serverReplaced
    /// Something in front of the server, such as an authentication proxy,
    /// refused the request. The credential is still good.
    case blockedByProxy
    /// The server couldn't be reached.
    case unreachable(String)
    /// iOS keeps the app off the local network, where the server is: its
    /// Local Network permission is off, or still being asked for.
    case localNetworkDenied
    /// Anything else the server said.
    case server(status: Int, message: String)
}

/// The one HTTP layer every request of one account goes through. Requests
/// skip the URL cache, so every response the app reads capabilities from
/// came from the server (ADR 0020). The system's trust store, including
/// root certificates the user installed and trusted, checks HTTPS.
final class APIClient: Sendable {
    let baseURL: URL
    private let credential: String?
    private let session: URLSession
    /// Called with each capability list a server response carries. It's
    /// fixed at creation, since requests on any thread read it.
    private let onCapabilities: (@Sendable ([String]) -> Void)?
    /// The server instance the account was made on, which every request
    /// names so a server set up again refuses to sync it. Empty for an
    /// account from before servers had one, until a response says.
    private let instance: OSAllocatedUnfairLock<String>
    /// Called once with the instance an account without one learns.
    private let onInstance: (@Sendable (String) -> Void)?
    /// Whether the server can be reached, which every request reads and
    /// updates.
    let reachability = ServerReachability()

    /// protocolClasses answer requests before the network, for tests.
    init(baseURL: URL, credential: String?, instanceID: String = "",
         onCapabilities: (@Sendable ([String]) -> Void)? = nil, onInstance: (@Sendable (String) -> Void)? = nil,
         protocolClasses: [AnyClass] = []) {
        self.baseURL = baseURL
        self.credential = credential
        self.onCapabilities = onCapabilities
        instance = OSAllocatedUnfairLock(initialState: instanceID)
        self.onInstance = onInstance
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        configuration.httpCookieStorage = nil
        configuration.timeoutIntervalForRequest = 30
        configuration.waitsForConnectivity = false
        configuration.protocolClasses = protocolClasses + (configuration.protocolClasses ?? [])
        session = URLSession(configuration: configuration, delegate: RedirectPolicy(), delegateQueue: nil)
    }

    deinit {
        // The session holds its delegate until it's invalidated.
        session.finishTasksAndInvalidate()
    }

    // MARK: - Requests

    func signIn(username: String, password: String, deviceName: String) async throws -> Shishoapp.SignInResponse {
        let payload = Shishoapp.SignInPayload(
            username: username, password: password, installationId: Installation.id,
            deviceName: deviceName, appVersion: Installation.appVersion
        )
        do {
            return try APIClient.signInResponse(await data("mobile/sign-in", method: "POST", body: try JSONEncoder().encode(payload)))
        } catch APIError.server(let status, _) where status == 404 || status == 405 {
            throw APIError.serverPredatesApp
        } catch APIError.signedOut {
            throw APIError.wrongPassword
        }
    }

    /// Whether the server answers now, asked quickly: a few seconds at most,
    /// so finding the server again never keeps a screen waiting. It's how a
    /// server marked unreachable is found again.
    @discardableResult
    func checkServer() async -> Bool {
        do {
            _ = try await data("mobile/server", readsCapabilities: false, timeout: 5, check: true)
            return true
        } catch {
            return reachability.allowsRequests
        }
    }

    /// Signs the Device out on the server, so its credential stops working.
    /// It's best effort: a sign-out offline still forgets the credential.
    /// It's tried whatever was last heard of the server, which may be out of
    /// date, since a credential left valid on a server that's back is worse
    /// than a request that fails.
    func signOut() async {
        _ = try? await data("mobile/device", method: "DELETE", check: true)
    }

    /// One page of the feed, and what the user can access now, when the
    /// server says (ADR 0023).
    struct FeedPage: Sendable {
        let response: Shishoapp.FeedResponse
        let access: Shishoapp.FeedAccess?
    }

    /// The changes after cursor. expiredThrough is the last page's, so the
    /// server can tell a cursor that missed expired deletions from one that
    /// was downloading from the start when they expired.
    func changes(cursor: Int, expiredThrough: Int, limit: Int = 500) async throws -> FeedPage {
        let body = try await data("mobile/changes", query: [
            URLQueryItem(name: "cursor", value: String(cursor)),
            URLQueryItem(name: "expired_through", value: String(expiredThrough)),
            URLQueryItem(name: "limit", value: String(limit)),
        ])
        return FeedPage(response: try APIClient.feedPage(body), access: APIClient.feedAccess(body))
    }

    /// One page of a Library's or List's whole fetch, for one the user just
    /// gained access to.
    func scopedChanges(libraryID: Int? = nil, listID: Int? = nil, cursor: Int, limit: Int = 500) async throws -> FeedPage {
        var query = [URLQueryItem(name: "cursor", value: String(cursor)), URLQueryItem(name: "limit", value: String(limit))]
        if let libraryID {
            query.append(URLQueryItem(name: "library_id", value: String(libraryID)))
        }
        if let listID {
            query.append(URLQueryItem(name: "list_id", value: String(listID)))
        }
        let body = try await data("mobile/changes", query: query)
        return FeedPage(response: try APIClient.feedPage(body), access: APIClient.feedAccess(body))
    }

    // The API types decode leniently (ADR 0020), so any JSON decodes as
    // one, with every field at its zero value. A response that changes what's
    // on the device must have the fields only Shisho sends: an empty page
    // from something else would end a download from the start and drop
    // every Book it hadn't reached.

    /// A feed page, which always has its cursor, entries, and paging flags.
    /// A server from
    /// before the app answers with the web app's page, which isn't JSON.
    static func feedPage(_ data: Data) throws -> Shishoapp.FeedResponse {
        struct Shape: Decodable {
            struct Anything: Decodable {
                init(from decoder: Decoder) throws {}
            }

            let cursor: Int64
            let entries: [Anything]
            let hasMore: Bool
            let startOver: Bool

            enum CodingKeys: String, CodingKey {
                case cursor, entries
                case hasMore = "has_more"
                case startOver = "start_over"
            }
        }
        guard (try? JSONDecoder().decode(Shape.self, from: data)) != nil,
              let page = try? makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: data)
        else { throw APIError.serverPredatesApp }
        return page
    }

    /// What a feed page says the user can access, or nil when it doesn't
    /// say. A page decodes leniently, so without this a server from before
    /// access was sent would read as access to nothing, and the app would
    /// drop every Library.
    static func feedAccess(_ data: Data) -> Shishoapp.FeedAccess? {
        struct Shape: Decodable {
            struct Access: Decodable {
                let libraryIds: [Int]
                let listIds: [Int]

                enum CodingKeys: String, CodingKey {
                    case libraryIds = "library_ids"
                    case listIds = "list_ids"
                }
            }

            let access: Access
        }
        guard (try? JSONDecoder().decode(Shape.self, from: data)) != nil else { return nil }
        return try? makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: data).access
    }

    /// A sign-in, which always names its credential, Device, and user.
    static func signInResponse(_ data: Data) throws -> Shishoapp.SignInResponse {
        guard let response = try? makeShishoDecoder().decode(Shishoapp.SignInResponse.self, from: data),
              !response.credential.isEmpty, !response.deviceId.isEmpty, response.user.id > 0
        else { throw APIError.serverPredatesApp }
        return response
    }

    /// Sends a batch of the journal (ADR 0024), answered with one result per
    /// event. A response without them, from something other than Shisho,
    /// is an error, so the app keeps every event.
    func journal(_ rows: [JournalRow], clock: Date = Date()) async throws -> Shishoapp.JournalResponse {
        let decoder = JSONDecoder()
        let events = try rows.map { row in
            Shishoapp.JournalEvent(
                id: row.eventID, type: Shishoapp.JournalEventType(rawValue: row.type), at: JournalTime.string(row.at),
                payload: try decoder.decode(JSONValue.self, from: row.payload)
            )
        }
        let payload = Shishoapp.JournalPayload(clock: JournalTime.string(clock), events: try events.map {
            try decoder.decode(JSONValue.self, from: JSONEncoder().encode($0))
        })
        let body = try JSONEncoder().encode(payload)
        return try APIClient.journalResponse(await data("mobile/journal", method: "POST", body: body), sent: rows.count)
    }

    static func journalResponse(_ data: Data, sent: Int) throws -> Shishoapp.JournalResponse {
        struct Shape: Decodable {
            struct Result: Decodable {
                let id: String
                let result: String
            }

            let results: [Result]
        }
        guard let shape = try? JSONDecoder().decode(Shape.self, from: data), shape.results.count == sent,
              let response = try? makeShishoDecoder().decode(Shishoapp.JournalResponse.self, from: data)
        else { throw APIError.serverPredatesApp }
        return response
    }

    /// The user's Reading Stats for year, or all time. A response without
    /// its time zone, from something other than Shisho, is an error, so the
    /// copy on the device stays.
    func stats(year: Int?) async throws -> Shishoapp.StatsResponse {
        let query = year.map { [URLQueryItem(name: "year", value: String($0))] } ?? []
        let body = try await data("mobile/stats", query: query)
        struct Shape: Decodable {
            let time_zone: String
            let totals: [String: Double?]
        }
        guard (try? JSONDecoder().decode(Shape.self, from: body)) != nil,
              let response = try? makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: body)
        else { throw APIError.serverPredatesApp }
        return response
    }

    /// The user's reading a day at a time for year, or the last 12 months,
    /// for Reading Stats' Calendar (CapabilityReadingCalendar). A response
    /// without its days, from something other than Shisho, is an error, so
    /// the copy on the device stays.
    func statsHeatmap(year: Int?) async throws -> Shishoapp.StatsHeatmapResponse {
        let query = year.map { [URLQueryItem(name: "year", value: String($0))] } ?? []
        let body = try await data("mobile/stats/heatmap", query: query)
        struct Shape: Decodable {
            let today: String
            let from: String
            let to: String
            let days: [Shishoapp.StatsDay]
        }
        guard (try? JSONDecoder().decode(Shape.self, from: body)) != nil,
              let response = try? makeShishoDecoder().decode(Shishoapp.StatsHeatmapResponse.self, from: body)
        else { throw APIError.serverPredatesApp }
        return response
    }

    /// One month, 1 to 12, of the user's Reading Stats calendar.
    func statsCalendar(year: Int, month: Int) async throws -> Shishoapp.StatsCalendarResponse {
        let body = try await data("mobile/stats/calendar", query: [
            URLQueryItem(name: "year", value: String(year)),
            URLQueryItem(name: "month", value: String(month)),
        ])
        struct Shape: Decodable {
            let year: Int
            let month: Int
            let books: [Shishoapp.StatsBookRef]
        }
        guard (try? JSONDecoder().decode(Shape.self, from: body)) != nil,
              let response = try? makeShishoDecoder().decode(Shishoapp.StatsCalendarResponse.self, from: body)
        else { throw APIError.serverPredatesApp }
        return response
    }

    /// One day's Reading Sessions and finishes, day as YYYY-MM-DD.
    func statsDay(day: String) async throws -> Shishoapp.StatsDayResponse {
        let body = try await data("mobile/stats/day", query: [URLQueryItem(name: "day", value: day)])
        struct Shape: Decodable {
            let day: String
            let time_zone: String
            let sessions: [Shishoapp.StatsDaySession]
        }
        guard (try? JSONDecoder().decode(Shape.self, from: body)) != nil,
              let response = try? makeShishoDecoder().decode(Shishoapp.StatsDayResponse.self, from: body)
        else { throw APIError.serverPredatesApp }
        return response
    }

    /// A File's own cover at size pixels, keyed by its cover version.
    func fileCover(fileID: Int, version: String, size: Int, renderKey: String) async throws -> Data {
        try await data("mobile/files/\(fileID)/cover", query: [
            URLQueryItem(name: "v", value: version),
            URLQueryItem(name: "size", value: String(size)),
            URLQueryItem(name: "r", value: renderKey),
        ], readsCapabilities: false)
    }

    /// A CBZ's or PDF's page, 0-based, as a thumbnail at size pixels, for
    /// the page grid of a File that isn't downloaded (CapabilityPageImages).
    func pageThumbnail(fileID: Int, page: Int, size: Int) async throws -> Data {
        try await data("mobile/files/\(fileID)/pages/\(page)", query: [
            URLQueryItem(name: "size", value: String(size)),
        ], readsCapabilities: false)
    }

    /// Where an audiobook File streams from, with Range support.
    func streamURL(fileID: Int) -> URL {
        baseURL.appendingPathComponent("mobile/files/\(fileID)/stream")
    }

    /// The credential, protocol, and version as cookies, for the system
    /// player, which makes its own requests and takes cookies but no
    /// headers. The server takes the credential and protocol from cookies
    /// on the stream route only, and they go only with requests for Files,
    /// so a redirect elsewhere on the host doesn't carry the credential.
    var streamCookies: [HTTPCookie] {
        // As the stream's URL spells it, so a path with spaces still matches.
        let files = baseURL.appendingPathComponent("mobile/files")
        let path = (URLComponents(url: files, resolvingAgainstBaseURL: false)?.percentEncodedPath ?? files.path) + "/"
        let values = [
            ("shisho_app_credential", credential),
            ("shisho_app_protocol", String(Installation.appProtocol)),
            // For the server's access log, as the header names it elsewhere.
            ("shisho_app_version", Installation.appVersion),
        ]
        return values.compactMap { name, value in
            guard let value else { return nil }
            var properties: [HTTPCookiePropertyKey: Any] = [
                .name: name, .value: value, .path: path, .originURL: baseURL,
            ]
            if baseURL.scheme == "https" {
                properties[.secure] = "TRUE"
            }
            return HTTPCookie(properties: properties)
        }
    }

    /// A Book's cover at size pixels, keyed by its cover version so the
    /// server sends it as immutable.
    func cover(bookID: Int, version: String, size: Int, renderKey: String) async throws -> Data {
        try await data("mobile/books/\(bookID)/cover", query: [
            URLQueryItem(name: "v", value: version),
            URLQueryItem(name: "size", value: String(size)),
            URLQueryItem(name: "r", value: renderKey),
        ], readsCapabilities: false)
    }

    // MARK: - Plumbing

    /// A File's Original File (ADR 0022), downloaded to a temporary file
    /// the caller moves where it keeps it.
    func download(fileID: Int) async throws -> URL {
        try await fetchFile(request("mobile/files/\(fileID)/download", accept: "*/*"), session: session).0
    }

    /// A File's Generated File, which carries Shisho's current metadata, for
    /// sharing out of the app (ADR 0022), downloaded to a temporary file.
    /// The server builds it before it answers, which can take minutes for a
    /// large audiobook, so the wait for the first byte is long.
    /// It's named as the server names it, in a folder of its own the
    /// caller deletes when done.
    func shareFile(fileID: Int) async throws -> URL {
        var request = request("mobile/files/\(fileID)/share", accept: "*/*")
        request.timeoutInterval = 15 * 60
        let (file, response) = try await fetchFile(request, session: session)
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("Share-\(UUID().uuidString)", isDirectory: true)
        do {
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let name = response.suggestedFilename.flatMap { $0.isEmpty ? nil : $0 } ?? "File \(fileID)"
            let named = folder.appendingPathComponent(name.replacingOccurrences(of: "/", with: "-"))
            try FileManager.default.moveItem(at: file, to: named)
            return named
        } catch {
            try? FileManager.default.removeItem(at: file)
            try? FileManager.default.removeItem(at: folder)
            throw error
        }
    }

    private func fetchFile(_ request: URLRequest, session: URLSession) async throws -> (URL, URLResponse) {
        guard reachability.allowsRequests else { throw APIError.unreachable("offline") }
        let file: URL
        let response: URLResponse
        do {
            (file, response) = try await session.download(for: request)
        } catch {
            reachability.failed(error)
            throw APIClient.transportError(error)
        }
        reachability.answered()
        do {
            try APIClient.checkDownloaded(file, response: response)
        } catch {
            try? FileManager.default.removeItem(at: file)
            throw error
        }
        return (file, response)
    }

    /// Checks that a finished transfer is the file asked for: a success, not
    /// a login page from something in front of the server, and not cut off
    /// partway. A failure's body, which the server sends as JSON, says why.
    static func checkDownloaded(_ file: URL, response: URLResponse?) throws {
        guard let http = response as? HTTPURLResponse else {
            throw APIError.unreachable("No response")
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = (try? Data(contentsOf: file)) ?? Data()
            throw APIClient.error(status: http.statusCode, response: http, body: body)
        }
        let size = (try? FileManager.default.attributesOfItem(atPath: file.path)[.size] as? Int64) ?? -1
        if http.mimeType == "text/html" {
            throw APIError.blockedByProxy
        }
        // A resumed transfer's response is the 206 for the rest, whose
        // length is less than the whole file.
        if http.statusCode == 200, http.expectedContentLength >= 0, size != http.expectedContentLength {
            throw APIError.unreachable("The download stopped partway")
        }
    }

    // MARK: - Downloads

    /// A session that transfers Downloads in the background, outside the app
    /// and across its relaunches (ADR 0022). It's configured as this
    /// client's is, and its requests come from `downloadRequest`, so they
    /// carry the same headers. iOS hands its events to delegate, and on a
    /// relaunch to the session made again with the same identifier. One
    /// that's wifiOnly is kept off cellular and any network iOS calls
    /// expensive or constrained, even while the app is suspended, and so is
    /// a transfer carried on in it from one that started in another.
    func backgroundSession(identifier: String, wifiOnly: Bool, delegate: URLSessionDelegate) -> URLSession {
        let configuration = URLSessionConfiguration.background(withIdentifier: identifier)
        configuration.allowsCellularAccess = !wifiOnly
        configuration.allowsExpensiveNetworkAccess = !wifiOnly
        configuration.allowsConstrainedNetworkAccess = !wifiOnly
        configuration.urlCache = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        configuration.httpCookieStorage = nil
        configuration.sessionSendsLaunchEvents = true
        // Not left for when iOS finds it convenient: the user asked for it.
        configuration.isDiscretionary = false
        configuration.httpMaximumConnectionsPerHost = 2
        return URLSession(configuration: configuration, delegate: delegate, delegateQueue: nil)
    }

    /// The request for a File's Original File in a background session,
    /// whose own configuration says which networks it may use.
    func downloadRequest(fileID: Int) -> URLRequest {
        request("mobile/files/\(fileID)/download", accept: "*/*")
    }

    private func request(_ path: String, method: String = "GET", query: [URLQueryItem] = [], body: Data? = nil,
                         accept: String = "application/json") -> URLRequest {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)!
        if !query.isEmpty {
            components.queryItems = query
        }
        var request = URLRequest(url: components.url!)
        request.httpMethod = method
        request.setValue(String(Installation.appProtocol), forHTTPHeaderField: "X-Shisho-App-Protocol")
        request.setValue(Installation.appVersion, forHTTPHeaderField: "X-Shisho-App-Version")
        request.setValue(accept, forHTTPHeaderField: "Accept")
        if let credential {
            request.setValue("Bearer \(credential)", forHTTPHeaderField: "Authorization")
        }
        let instance = instance.withLock { $0 }
        if !instance.isEmpty {
            request.setValue(instance, forHTTPHeaderField: "X-Shisho-Instance")
        }
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return request
    }

    private func data(_ path: String, method: String = "GET", query: [URLQueryItem] = [], body: Data? = nil,
                      readsCapabilities: Bool = true, timeout: TimeInterval? = nil, check: Bool = false) async throws -> Data {
        // Known not to answer: fail at once rather than wait out a timeout.
        // A check is how it's found again.
        guard check || reachability.allowsRequests else { throw APIError.unreachable("offline") }
        var request = self.request(path, method: method, query: query, body: body)
        if let timeout { request.timeoutInterval = timeout }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            reachability.failed(error)
            throw APIClient.transportError(error)
        }
        reachability.answered()
        guard let http = response as? HTTPURLResponse else {
            throw APIError.unreachable("No response")
        }
        if readsCapabilities, let listed = http.value(forHTTPHeaderField: "X-Shisho-Capabilities") {
            onCapabilities?(listed.split(separator: ",").map(String.init))
        }
        if let served = http.value(forHTTPHeaderField: "X-Shisho-Instance"), !served.isEmpty {
            // The server refuses a request naming another instance, but
            // something in front of it may drop the header, so an answer
            // from another instance is refused here too, before anything
            // reads it.
            let ok = (200..<300).contains(http.statusCode)
            let (other, adopted) = instance.withLock { known -> (Bool, Bool) in
                if !known.isEmpty {
                    return (known != served, false)
                }
                // Only an answer that took the credential is from the
                // account's server: one set up again refuses it.
                guard ok else { return (false, false) }
                known = served
                return (false, true)
            }
            if other {
                throw APIError.serverReplaced
            }
            if adopted {
                onInstance?(served)
            }
        }
        if (200..<300).contains(http.statusCode) {
            return data
        }
        throw APIClient.error(status: http.statusCode, response: http, body: data)
    }

    private struct ErrorBody: Decodable {
        struct Inner: Decodable {
            let code: String?
            let message: String?
        }

        let error: Inner?
    }

    /// Why a request never got a response. URLSession says the local
    /// network was refused only in the path it attaches to the error.
    static func transportError(_ error: Error) -> APIError {
        if let path = (error as NSError).userInfo["_NSURLErrorNWPathKey"] as? NWPath,
           path.unsatisfiedReason == .localNetworkDenied {
            return .localNetworkDenied
        }
        return .unreachable(error.localizedDescription)
    }

    static func error(status: Int, response: HTTPURLResponse, body: Data) -> APIError {
        let parsed = try? JSONDecoder().decode(ErrorBody.self, from: body)
        let code = parsed?.error?.code ?? ""
        let message = parsed?.error?.message ?? HTTPURLResponse.localizedString(forStatusCode: status)
        let serverVersion = response.value(forHTTPHeaderField: "X-Shisho-Server-Version").flatMap { $0.isEmpty ? nil : $0 }
        // Shisho answers every refusal with its error body, and every 401
        // with the code unauthorized. A 401 or 403 that isn't Shisho's came
        // from something in front of it, and must not sign the device out.
        if (status == 401 && code != "unauthorized") || (status == 403 && code.isEmpty) {
            return .blockedByProxy
        }
        // A redirect RedirectPolicy refused to follow, to another server. Its
        // address is the part before the app's paths, as the address field
        // takes it.
        if (300..<400).contains(status) {
            var server = response.value(forHTTPHeaderField: "Location") ?? ""
            if let paths = server.range(of: "/mobile/") {
                server = String(server[..<paths.lowerBound])
            }
            return .server(status: status, message: String(localized: "The server sends the app to another server (\(server)), which it doesn't follow with your sign-in. Sign in with that address instead."))
        }
        switch (status, code) {
        case (403, "demo_mode"):
            return .demoMode
        case (409, "server_replaced"):
            return .serverReplaced
        case (426, "server_update_required"):
            return .serverTooOld(serverVersion: serverVersion)
        case (426, _):
            return .appTooOld(serverVersion: serverVersion)
        case (403, "password_reset_required"):
            return .passwordResetRequired
        case (403, _):
            return .noAccess
        case (401, _):
            return .signedOut
        default:
            return .server(status: status, message: message)
        }
    }
}
