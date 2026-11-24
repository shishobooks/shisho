import SwiftUI

/// The signed-in account: which server, the versions on both sides, the
/// last sync, and signing out, with every other account on this device to
/// switch to at once, each with its own copy of its library.
struct AccountSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var confirmingSignOut = false
    @State private var showingDiagnostics = false
    @State private var syncState: SyncState?

    var body: some View {
        NavigationStack {
            ScrollView {
                if let account = model.session?.account {
                    VStack(spacing: 20) {
                        identity(account)
                        accountsCard(account)
                        detailsCard(account)
                        actions
                        Text("Signing out removes this device from your account on the server. The library on it stays until you sign in again.")
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 8)
                    }
                    .padding(20)
                    .frame(maxWidth: 520)
                    .frame(maxWidth: .infinity)
                }
            }
            .background(Theme.background)
            .navigationTitle("Account")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .font(.geist(.body, weight: .semibold))
                        .accessibilityIdentifier("account.done")
                }
            }
            .sheet(isPresented: $showingDiagnostics) {
                DiagnosticsSheet()
            }
            .confirmationDialog(signOutTitle, isPresented: $confirmingSignOut, titleVisibility: .visible) {
                Button("Sign Out", role: .destructive) {
                    dismiss()
                    Task { await model.signOut() }
                }
                .accessibilityIdentifier("account.signOut.confirm")
            }
        }
        .tint(Theme.primary)
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .presentationCornerRadius(24)
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $syncState) { db in
            try CatalogStore.syncState(db)
        }
    }

    /// Every account on this device: tapping one opens its library at once,
    /// from its own copy, and syncs it.
    private func accountsCard(_ current: Account) -> some View {
        VStack(spacing: 0) {
            ForEach(model.allAccounts) { account in
                Button {
                    guard account.id != current.id else { return }
                    dismiss()
                    model.switchAccount(to: account.id)
                } label: {
                    HStack(spacing: 12) {
                        AccountAvatar(account: account, size: 32)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(account.username)
                                .font(.geist(.subheadline, weight: .semibold))
                                .foregroundStyle(Theme.foreground)
                            Text(account.signedOut
                                 ? String(localized: "\(account.serverURL.host() ?? account.serverURL.absoluteString) · Signed out")
                                 : account.serverURL.host() ?? account.serverURL.absoluteString)
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                                .lineLimit(1)
                        }
                        Spacer()
                        if account.id == current.id {
                            Icon(.check, size: 18, relativeTo: .subheadline)
                                .foregroundStyle(Theme.primary)
                        }
                    }
                    .padding(.horizontal, 16)
                    .frame(minHeight: 56)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityElement(children: .combine)
                .accessibilityAddTraits(account.id == current.id ? .isSelected : [])
                .accessibilityHint(account.id == current.id ? "" : String(localized: "Switches to this account"))
                .accessibilityIdentifier("account.switch.\(account.username)")
                Divider().overlay(Theme.border)
            }
            Button {
                dismiss()
                model.addingAccount = true
            } label: {
                Label("Add Server", icon: .plus, size: 17)
                    .font(.geist(.subheadline, weight: .semibold))
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 48)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .foregroundStyle(Theme.primary)
            .accessibilityIdentifier("account.add")
        }
        .cardStyle()
    }

    /// Names the account as well as its server, since several accounts can
    /// be on one server.
    private var signOutTitle: String {
        guard let account = model.session?.account else { return String(localized: "Sign out?") }
        return String(localized: "Sign out \(account.username) on \(account.serverURL.host() ?? account.serverURL.absoluteString)?")
    }

    private func identity(_ account: Account) -> some View {
        VStack(spacing: 10) {
            AccountAvatar(account: account, size: 68)
            Text(account.username)
                .font(.geist(.title3, weight: .bold))
            Text(account.serverURL.host() ?? account.serverURL.absoluteString)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
        }
        .padding(.top, 4)
        .accessibilityElement(children: .combine)
    }

    private func detailsCard(_ account: Account) -> some View {
        VStack(spacing: 0) {
            row("Server", value: account.serverURL.absoluteString)
            Divider().overlay(Theme.border)
            row("Server version", value: account.serverVersion)
            Divider().overlay(Theme.border)
            row("App version", value: Installation.appVersion)
            Divider().overlay(Theme.border)
            HStack {
                Text("Last synced")
                    .foregroundStyle(Theme.mutedForeground)
                Spacer()
                if model.isSyncing {
                    ProgressView().controlSize(.small)
                } else if let date = syncState?.lastSyncedAt {
                    Text(date, format: .relative(presentation: .named))
                } else {
                    Text("Never")
                }
            }
            .font(.geist(.subheadline))
            .padding(.horizontal, 16)
            .frame(minHeight: 48)
            .accessibilityElement(children: .combine)
        }
        .cardStyle()
    }

    private func row(_ label: LocalizedStringKey, value: String) -> some View {
        HStack(spacing: 12) {
            Text(label)
                .foregroundStyle(Theme.mutedForeground)
            Spacer()
            Text(value)
                .lineLimit(1)
                .truncationMode(.middle)
        }
        .font(.geist(.subheadline))
        .padding(.horizontal, 16)
        .frame(minHeight: 48)
        .accessibilityElement(children: .combine)
    }

    private var actions: some View {
        VStack(spacing: 10) {
            Button {
                Task { await model.syncNow() }
            } label: {
                Label("Sync Now", icon: .refresh)
            }
            .buttonStyle(OutlineButtonStyle())
            .disabled(model.isSyncing)
            .accessibilityIdentifier("account.sync")

            Button {
                showingDiagnostics = true
            } label: {
                Label("Export Diagnostics", icon: .fileSearch)
            }
            .buttonStyle(OutlineButtonStyle())
            .accessibilityIdentifier("account.diagnostics")

            Button(role: .destructive) {
                confirmingSignOut = true
            } label: {
                Label("Sign Out", icon: .logOut)
            }
            .buttonStyle(OutlineButtonStyle(role: .destructive))
            .accessibilityIdentifier("account.signOut")
        }
    }
}

/// The diagnostics export: the device's log of syncs, journal results, and
/// player errors, with server addresses and credentials removed, shown in
/// full before it's shared. Titles are left out unless included.
struct DiagnosticsSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var includeTitles = false
    @State private var text: String?
    @State private var file: URL?

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                SettingsCard {
                    SettingToggle(title: "Include titles",
                                  description: "Server addresses and sign-in details are always removed. Books and Files are named by id unless you include titles.",
                                  isOn: $includeTitles, identifier: "diagnostics.includeTitles")
                }
                ScrollView {
                    Text(text ?? "")
                        .font(.geistMono(.caption))
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(12)
                }
                .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
                .accessibilityIdentifier("diagnostics.text")
            }
            .padding(20)
            .background(Theme.background)
            .navigationTitle("Diagnostics")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("diagnostics.done")
                }
                ToolbarItem(placement: .primaryAction) {
                    if let file {
                        ShareLink(item: file) {
                            Label("Share", menuIcon: .share)
                        }
                        .accessibilityIdentifier("diagnostics.share")
                    }
                }
            }
            .task(id: includeTitles) {
                let exported = await model.diagnosticsText(includeTitles: includeTitles)
                text = exported
                let url = FileManager.default.temporaryDirectory.appendingPathComponent("Shisho Diagnostics.txt")
                file = (try? Data(exported.utf8).write(to: url, options: .atomic)) == nil ? nil : url
            }
        }
        .tint(Theme.primary)
        .presentationDragIndicator(.visible)
    }
}

/// An account's initial in a circle of its own color, so accounts whose
/// usernames start alike still look different. The color comes from the
/// account's id on this device, so it stays the same across launches.
struct AccountAvatar: View {
    let account: Account
    let size: CGFloat
    /// In the toolbar, where iOS 26 already sets the button in a circle.
    var inToolbar = false

    private var color: Color {
        let hue = Double(account.id.unicodeScalars.reduce(0) { ($0 &* 31 &+ Int($1.value)) & 0xFFFF } % 360) / 360
        return Color(hue: hue, saturation: 0.55, brightness: 0.72)
    }

    var body: some View {
        let initial = Text(String(account.username.prefix(1).uppercased()))
            .font(.geist(size: size * 0.44, relativeTo: .title, weight: .semibold))
            .foregroundStyle(color)
        Group {
            if #available(iOS 26, *), inToolbar {
                initial
            } else {
                initial
                    .frame(width: size, height: size)
                    .background(color.opacity(0.14), in: Circle())
                    .overlay(Circle().strokeBorder(color.opacity(0.3)))
            }
        }
        .accessibilityHidden(true)
    }
}
