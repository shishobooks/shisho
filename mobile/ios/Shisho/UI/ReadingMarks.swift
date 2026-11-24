import SwiftUI

/// A Reading Status as the web app colors and marks it
/// (`ReadingStatusBookmark` and `ReadingStatusIcon`).
enum StatusStyle {
    static func color(_ status: String) -> Color {
        switch status {
        case "currently_reading": Color(red: 124 / 255, green: 58 / 255, blue: 237 / 255) // violet-600
        case "paused": Color(red: 245 / 255, green: 158 / 255, blue: 11 / 255) // amber-500
        case "to_read": Color(red: 14 / 255, green: 165 / 255, blue: 233 / 255) // sky-500
        case "read": Color(red: 5 / 255, green: 150 / 255, blue: 105 / 255) // emerald-600
        case "did_not_finish": Color(red: 225 / 255, green: 29 / 255, blue: 72 / 255) // rose-600
        case "reference": Color(red: 13 / 255, green: 148 / 255, blue: 136 / 255) // teal-600
        default: Theme.mutedForeground
        }
    }

    /// The one icon each Reading Status has wherever it shows, on a cover's
    /// ribbon and beside its name, as the web's `ReadingStatusIcon`.
    static func icon(_ status: String?) -> Lucide {
        switch status {
        case "paused": .pause
        case "to_read": .clock
        case "read": .check
        case "did_not_finish": .x
        case "reference": .library
        default: .bookOpen
        }
    }
}

/// The ribbon bookmark hanging from a cover's top edge, colored and marked
/// by Reading Status, or showing how far through a Book being read is, as
/// the web app's gallery has it.
struct ReadingStatusRibbon: View {
    let status: String
    var progress: Double?
    var scale: CGFloat = 1

    private var percent: String? {
        guard status == "currently_reading", let progress else { return nil }
        return Percent.text(progress)
    }

    var body: some View {
        ZStack(alignment: .top) {
            RibbonShape()
                .fill(StatusStyle.color(status))
            if let percent {
                // Geist at 9 points, as the web's bookmark sets it, so the
                // ribbon is as narrow as the web's.
                Text(verbatim: percent)
                    .font(.custom(Theme.Fonts.geist, size: 9 * scale).weight(.bold))
                    .monospacedDigit()
                    .foregroundStyle(.white)
                    .padding(.top, 6 * scale)
            } else {
                Icon(StatusStyle.icon(status), size: 13 * scale, relativeTo: nil)
                    .foregroundStyle(.white)
                    .padding(.top, 6 * scale)
            }
        }
        .frame(width: (percent == nil ? 24 : 28) * scale, height: 32 * scale)
        .shadow(color: .black.opacity(0.25), radius: 2, y: 1)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(percent.map { String(localized: "\(ReadingStatusLabel.text(status)), \($0) through") }
            ?? ReadingStatusLabel.text(status))
    }
}

private struct RibbonShape: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.minX, y: rect.minY))
        path.addLine(to: CGPoint(x: rect.maxX, y: rect.minY))
        path.addLine(to: CGPoint(x: rect.maxX, y: rect.maxY))
        path.addLine(to: CGPoint(x: rect.midX, y: rect.minY + rect.height * 0.78))
        path.addLine(to: CGPoint(x: rect.minX, y: rect.maxY))
        path.closeSubpath()
        return path
    }
}

/// A Rating on a cover's bottom corner, as the web gallery's badge shows it.
struct CoverRatingBadge: View {
    let units: Int

    var body: some View {
        HStack(spacing: 2) {
            Icon(.starFill, size: 10, relativeTo: nil)
                .foregroundStyle(Theme.rating)
            Text(verbatim: Ratings.text(units))
                .font(.geist(size: 10, relativeTo: .caption2, weight: .semibold))
                .monospacedDigit()
                .foregroundStyle(Theme.foreground)
        }
        .padding(.horizontal, 5)
        .padding(.vertical, 1)
        .background(Theme.secondary, in: RoundedRectangle(cornerRadius: 5, style: .continuous))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(String(localized: "Rated \(Ratings.text(units)) of 5 stars"))
    }
}

/// How far through a Book or File the user is, as a thin bar.
struct ProgressBar: View {
    let value: Double

    var body: some View {
        GeometryReader { geometry in
            ZStack(alignment: .leading) {
                Capsule().fill(Theme.primary.opacity(0.2))
                Capsule().fill(Theme.primary)
                    .frame(width: geometry.size.width * min(1, max(0, value)))
            }
        }
        .frame(height: 4)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(String(localized: "\(Percent.text(value)) through"))
    }
}

/// How far through a File the user is, under its facts on its row: a thin
/// bar, the percentage, and the time, pages, or words left, as the web's row
/// has it.
struct FileProgressLine: View {
    let progress: Double
    let durationSeconds: Double?
    let pageCount: Int?
    var wordCount: Int?

    var body: some View {
        let text = FileProgressText.text(progress, durationSeconds: durationSeconds, pageCount: pageCount, wordCount: wordCount)
        HStack(spacing: 8) {
            ProgressBar(value: progress)
                .frame(maxWidth: 150)
            Text(verbatim: text)
                .font(.geist(.caption))
                .monospacedDigit()
                .foregroundStyle(Theme.mutedForeground)
                .lineLimit(1)
                .layoutPriority(1)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(text)
    }
}

/// The web app's number formats.
/// Text cut to a few lines with a Show more button, which shows only when
/// the text doesn't fit.
struct ExpandableText: View {
    let text: String
    var lineLimit = 5
    let identifier: String

    @State private var expanded = false
    @State private var fullHeight: CGFloat = 0
    @State private var shownHeight: CGFloat = 0

    private var truncated: Bool { fullHeight > shownHeight + 1 }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(text)
                .lineLimit(expanded ? nil : lineLimit)
                .background(GeometryReader { geometry in
                    Color.clear
                        .onAppear { shownHeight = geometry.size.height }
                        .onChange(of: geometry.size.height) { _, height in shownHeight = height }
                })
                // The whole text, measured but not shown.
                .background(alignment: .top) {
                    Text(text)
                        .fixedSize(horizontal: false, vertical: true)
                        .hidden()
                        .background(GeometryReader { geometry in
                            Color.clear
                                .onAppear { fullHeight = geometry.size.height }
                                .onChange(of: geometry.size.height) { _, height in fullHeight = height }
                        })
                        .accessibilityHidden(true)
                }
                .accessibilityIdentifier(identifier)
            if expanded || truncated {
                ShowMoreButton(expanded: $expanded, identifier: "\(identifier).more")
            }
        }
    }
}

/// Text whose names link to their pages, wrapping as running text does, as
/// the web app's bylines and lists of links do. Each link is a URL the
/// environment's openURL turns into a Route.
enum LinkedText {
    /// The scheme of the URLs that stand for Routes in running text.
    static let scheme = "shisho-route"

    static func url(_ route: Route) -> URL? {
        switch route {
        case .resource(let kind, let id, let name):
            var components = URLComponents()
            components.scheme = scheme
            components.host = kind.rawValue
            components.path = "/\(id)"
            components.queryItems = [URLQueryItem(name: "name", value: name)]
            return components.url
        default:
            return nil
        }
    }

    static func route(_ url: URL) -> Route? {
        guard url.scheme == scheme, let host = url.host(), let kind = ResourceKind(rawValue: host),
              let id = Int(url.lastPathComponent) else { return nil }
        let name = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "name" }?.value ?? ""
        return .resource(kind, id: id, name: name)
    }

    /// One linked name, underlined as the web's text links are.
    static func link(_ text: String, to route: Route) -> AttributedString {
        var name = AttributedString(text)
        name.link = url(route)
        name.underlineStyle = .single
        name.foregroundColor = Theme.foreground
        return name
    }

    /// A Book's title linked to its page. Its Route needs the Book, so the
    /// page showing it opens it (bookID(_:)).
    static func bookLink(_ title: String, id: Int) -> AttributedString {
        var components = URLComponents()
        components.scheme = scheme
        components.host = "book"
        components.path = "/\(id)"
        var name = AttributedString(title)
        name.link = components.url
        name.underlineStyle = .single
        name.foregroundColor = Theme.foreground
        return name
    }

    static func bookID(_ url: URL) -> Int? {
        guard url.scheme == scheme, url.host() == "book" else { return nil }
        return Int(url.lastPathComponent)
    }

    /// Muted text between the links.
    static func muted(_ text: String) -> AttributedString {
        var part = AttributedString(text)
        part.foregroundColor = Theme.mutedForeground
        return part
    }
}

extension View {
    /// Opens the Routes LinkedText's links stand for on the tab showing.
    func opensRouteLinks(_ model: AppModel) -> some View {
        environment(\.openURL, OpenURLAction { url in
            guard let route = LinkedText.route(url) else { return .systemAction }
            model.push(route)
            return .handled
        })
    }
}
