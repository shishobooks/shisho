import SwiftUI

/// The signed-in account: which server, the versions on both sides, the
/// last sync, and signing out. The account store already keeps several
/// accounts, each in its own folder; this sheet is where switching between
/// them goes.
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
            .confirmationDialog("Sign out of \(model.session?.account.serverURL.host() ?? "this server")?", isPresented: $confirmingSignOut, titleVisibility: .visible) {
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

    private func identity(_ account: Account) -> some View {
        VStack(spacing: 10) {
            Text(String(account.username.prefix(1).uppercased()))
                .font(.geist(size: 30, relativeTo: .title, weight: .semibold))
                .foregroundStyle(Theme.primary)
                .frame(width: 68, height: 68)
                .background(Theme.primary.opacity(0.12), in: Circle())
                .overlay(Circle().strokeBorder(Theme.primary.opacity(0.25)))
                .accessibilityHidden(true)
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
                Label("Sync Now", systemImage: "arrow.triangle.2.circlepath")
            }
            .buttonStyle(OutlineButtonStyle())
            .disabled(model.isSyncing)
            .accessibilityIdentifier("account.sync")

            Button {
                showingDiagnostics = true
            } label: {
                Label("Export Diagnostics", systemImage: "doc.text.magnifyingglass")
            }
            .buttonStyle(OutlineButtonStyle())
            .accessibilityIdentifier("account.diagnostics")

            Button(role: .destructive) {
                confirmingSignOut = true
            } label: {
                Label("Sign Out", systemImage: "rectangle.portrait.and.arrow.right")
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
                Toggle("Include titles", isOn: $includeTitles)
                    .font(.geist(.body))
                    .tint(Theme.primary)
                    .accessibilityIdentifier("diagnostics.includeTitles")
                Text("Server addresses and sign-in details are always removed. Books and Files are named by id unless you include titles.")
                    .font(.geist(.footnote))
                    .foregroundStyle(Theme.mutedForeground)
                ScrollView {
                    Text(text ?? "")
                        .font(.system(.caption, design: .monospaced))
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
                            Label("Share", systemImage: "square.and.arrow.up")
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
