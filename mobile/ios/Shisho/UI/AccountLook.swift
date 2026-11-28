import SwiftUI

/// How an account looks in its avatar, picked on this device: a color and
/// an icon, or the username's initial. The username is often the same on
/// every server, so a look tells the accounts apart at a glance. It's kept
/// per account (`AccountData.lookKey`), never sent to the server.
struct AccountLook: Codable, Equatable {
    /// Nil until one is picked: a color of the account's own, from its id.
    var color: AccountColor?
    /// Nil for the username's initial.
    var icon: AccountIcon?

    /// The icons offered, in the picker's order.
    static let icons: [Lucide] = [
        .house, .building2, .briefcase, .graduationCap, .library, .bookOpen,
        .headphones, .server, .globe, .star, .heart, .sparkles,
        .sun, .moon, .leaf, .mountain, .coffee, .rocket,
        .cat, .dog, .ghost, .music, .palette,
    ]

    /// What VoiceOver calls an icon in the picker.
    static func name(of icon: Lucide) -> String {
        switch icon {
        case .house: String(localized: "House")
        case .building2: String(localized: "Building")
        case .briefcase: String(localized: "Briefcase")
        case .graduationCap: String(localized: "Graduation cap")
        case .library: String(localized: "Library")
        case .bookOpen: String(localized: "Book")
        case .headphones: String(localized: "Headphones")
        case .server: String(localized: "Server")
        case .globe: String(localized: "Globe")
        case .star: String(localized: "Star")
        case .heart: String(localized: "Heart")
        case .sparkles: String(localized: "Sparkles")
        case .sun: String(localized: "Sun")
        case .moon: String(localized: "Moon")
        case .leaf: String(localized: "Leaf")
        case .mountain: String(localized: "Mountain")
        case .coffee: String(localized: "Coffee")
        case .rocket: String(localized: "Rocket")
        case .cat: String(localized: "Cat")
        case .dog: String(localized: "Dog")
        case .ghost: String(localized: "Ghost")
        case .music: String(localized: "Music")
        case .palette: String(localized: "Palette")
        default: icon.rawValue
        }
    }

    /// The color the avatar shows: the one picked, or one from the account's
    /// id, which stays the same across launches.
    func tint(accountID: String) -> Color {
        if let color { return color.color }
        let hue = Double(accountID.unicodeScalars.reduce(0) { ($0 &* 31 &+ Int($1.value)) & 0xFFFF } % 360) / 360
        return Color(hue: hue, saturation: 0.6, brightness: 0.68)
    }
}

/// An icon an account can show, by its Lucide name, so a name a later
/// version drops reads back as nil rather than losing the whole look.
struct AccountIcon: Codable, Equatable {
    let name: String

    init(_ icon: Lucide) { name = icon.rawValue }

    var lucide: Lucide? { Lucide(rawValue: name) }
}

/// The colors an account can take: dark enough for a white icon on each,
/// in either appearance.
enum AccountColor: String, Codable, CaseIterable, Identifiable {
    case red, orange, amber, green, teal, sky, blue, indigo, violet, pink, brown, slate

    var id: String { rawValue }

    var color: Color {
        switch self {
        case .red: Color(red: 0.86, green: 0.15, blue: 0.15)
        case .orange: Color(red: 0.92, green: 0.35, blue: 0.05)
        case .amber: Color(red: 0.85, green: 0.47, blue: 0.02)
        case .green: Color(red: 0.09, green: 0.64, blue: 0.29)
        case .teal: Color(red: 0.05, green: 0.58, blue: 0.53)
        case .sky: Color(red: 0.01, green: 0.52, blue: 0.78)
        case .blue: Color(red: 0.15, green: 0.39, blue: 0.92)
        case .indigo: Color(red: 0.36, green: 0.31, blue: 0.82)
        case .violet: Color(red: 0.49, green: 0.23, blue: 0.93)
        case .pink: Color(red: 0.86, green: 0.15, blue: 0.47)
        case .brown: Color(red: 0.54, green: 0.36, blue: 0.22)
        case .slate: Color(red: 0.28, green: 0.33, blue: 0.41)
        }
    }

    var name: String {
        switch self {
        case .red: String(localized: "Red")
        case .orange: String(localized: "Orange")
        case .amber: String(localized: "Amber")
        case .green: String(localized: "Green")
        case .teal: String(localized: "Teal")
        case .sky: String(localized: "Sky")
        case .blue: String(localized: "Blue")
        case .indigo: String(localized: "Indigo")
        case .violet: String(localized: "Violet")
        case .pink: String(localized: "Pink")
        case .brown: String(localized: "Brown")
        case .slate: String(localized: "Slate")
        }
    }
}

/// Every account's look on this device, kept in the settings by account,
/// observed so each avatar changes as soon as one is picked.
@MainActor
@Observable
final class AccountLooks {
    private var looks: [String: AccountLook] = [:]
    /// Looks read from the settings, so an avatar doesn't decode one on
    /// every render. Not observed: reading it never changes what shows.
    @ObservationIgnored private var read: [String: AccountLook] = [:]
    @ObservationIgnored private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    func look(for accountID: String) -> AccountLook {
        if let look = looks[accountID] { return look }
        if let look = read[accountID] { return look }
        let look = Self.stored(accountID: accountID, defaults: defaults)
        read[accountID] = look
        return look
    }

    func set(_ look: AccountLook, for accountID: String) {
        looks[accountID] = look
        if let data = try? JSONEncoder().encode(look) {
            defaults.set(data, forKey: AccountData(accountID: accountID).lookKey)
        }
    }

    private static func stored(accountID: String, defaults: UserDefaults) -> AccountLook {
        guard let data = defaults.data(forKey: AccountData(accountID: accountID).lookKey),
              let look = try? JSONDecoder().decode(AccountLook.self, from: data)
        else { return AccountLook() }
        return look
    }
}

/// An account as a circle of its color with its icon, or its initial, in
/// white.
struct AccountAvatar: View {
    @Environment(AppModel.self) private var model
    let account: Account
    let size: CGFloat
    /// A look to show instead of the account's own, as the picker previews.
    var look: AccountLook?

    var body: some View {
        let look = look ?? model.looks.look(for: account.id)
        Circle()
            .fill(look.tint(accountID: account.id).gradient)
            .frame(width: size, height: size)
            .overlay {
                if let icon = look.icon?.lucide {
                    Icon(icon, size: size * 0.5, relativeTo: nil)
                } else {
                    Text(String(account.username.prefix(1).uppercased()))
                        .font(.geist(size: size * 0.46, relativeTo: .title, weight: .semibold))
                        .minimumScaleFactor(0.5)
                }
            }
            .foregroundStyle(.white)
            .accessibilityHidden(true)
    }
}

/// Picks an account's color and icon, shown in its avatar at once.
struct AccountLookSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let account: Account

    private var look: AccountLook { model.looks.look(for: account.id) }

    private func update(_ change: (inout AccountLook) -> Void) {
        var look = look
        change(&look)
        model.looks.set(look, for: account.id)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 24) {
                    AccountAvatar(account: account, size: 88)
                        .padding(.top, 8)
                    section("Color") {
                        LazyVGrid(columns: columns, spacing: 12) {
                            ForEach(AccountColor.allCases) { color in
                                colorButton(color)
                            }
                        }
                    }
                    section("Icon") {
                        LazyVGrid(columns: columns, spacing: 12) {
                            initialButton
                            ForEach(AccountLook.icons, id: \.self) { icon in
                                iconButton(icon)
                            }
                        }
                    }
                }
                .padding(20)
                .frame(maxWidth: 520)
                .frame(maxWidth: .infinity)
            }
            .background(Theme.background)
            .navigationTitle("Avatar")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .font(.geist(.body, weight: .semibold))
                        .accessibilityIdentifier("accountLook.done")
                }
            }
        }
        .tint(Theme.primary)
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }

    private let columns = [GridItem(.adaptive(minimum: 44, maximum: 60), spacing: 12)]

    private func section(_ title: LocalizedStringKey, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
                .accessibilityAddTraits(.isHeader)
            content()
                .padding(14)
                .cardStyle()
        }
    }

    private func colorButton(_ color: AccountColor) -> some View {
        let selected = look.color == color
        return Button { update { $0.color = color } } label: {
            Circle()
                .fill(color.color.gradient)
                .frame(width: 36, height: 36)
                .overlay {
                    if selected { Icon(.check, size: 18, relativeTo: nil).foregroundStyle(.white) }
                }
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(verbatim: color.name))
        .accessibilityAddTraits(selected ? .isSelected : [])
        .accessibilityIdentifier("accountLook.color.\(color.rawValue)")
    }

    private var initialButton: some View {
        let selected = look.icon == nil
        return Button { update { $0.icon = nil } } label: {
            choiceLabel(selected: selected) {
                Text(String(account.username.prefix(1).uppercased()))
                    .font(.geist(size: 18, relativeTo: .body, weight: .semibold))
                    .minimumScaleFactor(0.5)
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text("Initial"))
        .accessibilityAddTraits(selected ? .isSelected : [])
        .accessibilityIdentifier("accountLook.icon.initial")
    }

    private func iconButton(_ icon: Lucide) -> some View {
        let selected = look.icon == AccountIcon(icon)
        return Button { update { $0.icon = AccountIcon(icon) } } label: {
            choiceLabel(selected: selected) { Icon(icon, size: 20, relativeTo: nil) }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(verbatim: AccountLook.name(of: icon)))
        .accessibilityAddTraits(selected ? .isSelected : [])
        .accessibilityIdentifier("accountLook.icon.\(icon.rawValue)")
    }

    private func choiceLabel(selected: Bool, @ViewBuilder label: () -> some View) -> some View {
        let tint = look.tint(accountID: account.id)
        return label()
            .foregroundStyle(selected ? .white : Theme.foreground)
            .frame(width: 44, height: 44)
            .background(selected ? AnyShapeStyle(tint.gradient) : AnyShapeStyle(Theme.secondary), in: Circle())
            .contentShape(Circle())
    }
}
