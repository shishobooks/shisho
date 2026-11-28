import SwiftUI

struct SignInView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.scenePhase) private var scenePhase
    @State private var server = ""
    @State private var username = ""
    @State private var password = ""
    @State private var busy = false
    @State private var error: SignInError?
    @State private var failures = 0
    @FocusState private var focus: Field?
    /// Signing in to another account, from the Account sheet: the screen
    /// opens as a sheet that can be cancelled.
    var addingAccount = false

    private enum Field: CaseIterable { case server, username, password }

    /// The keyboard bar's height and a margin under the focused field.
    private static let keyboardBarClearance: CGFloat = 64

    private var canSubmit: Bool {
        !busy && !server.isEmpty && !username.isEmpty && !password.isEmpty
    }

    var body: some View {
        ScrollViewReader { scroller in
            ScrollView {
                VStack(spacing: 28) {
                    if !addingAccount {
                        brand
                            .padding(.top, 48)
                    }

                    if addingAccount {
                        Text("Sign in to another server, or as someone else on this one. Switch between them in Account.")
                            .font(.geist(.subheadline))
                            .foregroundStyle(Theme.mutedForeground)
                            .multilineTextAlignment(.center)
                            .padding(.top, 16)
                    } else if let account = model.signedOutAccount, error == nil, model.serverReplacedAccountID == account.id {
                        NoticeCard(
                            text: String(localized: "The server at \(account.serverURL.host() ?? account.serverURL.absoluteString) was set up again since \(account.username) signed in, so it's a different server now. Sign in again to start fresh on it. Changes this device hadn't sent were for the earlier server, so none go to this one."),
                            icon: .server,
                            tint: Theme.primary
                        )
                        .accessibilityIdentifier("signin.serverReplaced")
                    } else if let account = model.signedOutAccount, error == nil {
                        NoticeCard(
                            text: String(localized: "This device was signed out of \(account.username) on \(account.serverURL.host() ?? account.serverURL.absoluteString). Sign in again to keep syncing. Everything on this device is still here."),
                            icon: .userX,
                            tint: Theme.primary
                        )
                        .accessibilityIdentifier("signin.signedOut")
                    }

                    VStack(spacing: 16) {
                        LabeledInput(label: "Server", icon: .server, focused: focus == .server) {
                            TextField("https://shisho.example.com", text: $server)
                                .textContentType(.URL)
                                .keyboardType(.URL)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .submitLabel(.next)
                                .focused($focus, equals: .server)
                                .onSubmit { focus = .username }
                                .accessibilityLabel("Server address")
                                .accessibilityIdentifier("signin.server")
                        }
                        .id(Field.server)
                        LabeledInput(label: "Username", icon: .user, focused: focus == .username) {
                            TextField("Username", text: $username)
                                .textContentType(.username)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .submitLabel(.next)
                                .focused($focus, equals: .username)
                                .onSubmit { focus = .password }
                                .accessibilityIdentifier("signin.username")
                        }
                        .id(Field.username)
                        LabeledInput(label: "Password", icon: .lock, focused: focus == .password) {
                            SecureField("Password", text: $password)
                                .textContentType(.password)
                                .submitLabel(.go)
                                .focused($focus, equals: .password)
                                .onSubmit(submit)
                                .accessibilityIdentifier("signin.password")
                        }
                        .id(Field.password)
                    }

                    if let error {
                        NoticeCard(text: error.message, icon: .triangleAlert, tint: Theme.destructive) {
                            if error.offersUpdate {
                                UpdateAppLink()
                            }
                            if error.offersSettings {
                                SettingsLink()
                            }
                        }
                        .accessibilityIdentifier("signin.error")
                        .transition(.move(edge: .top).combined(with: .opacity))
                    }

                    VStack(spacing: 14) {
                        Button(action: submit) {
                            ZStack {
                                Text("Sign In").opacity(busy ? 0 : 1)
                                if busy {
                                    ProgressView()
                                        .tint(Theme.primaryForeground)
                                        .accessibilityLabel("Signing in")
                                }
                            }
                        }
                        .buttonStyle(PrimaryButtonStyle())
                        .disabled(!canSubmit)
                        .accessibilityIdentifier("signin.submit")

                        Text("Use the same username and password as on the web. The server's address is the one you open in a browser.")
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            .multilineTextAlignment(.center)
                    }

                    if !addingAccount {
                        // Not while a sign-in is under way, whose answer
                        // would open its account over the one picked.
                        OtherAccounts()
                            .disabled(busy)
                    }
                }
                .padding(.horizontal, 24)
                .padding(.bottom, 32)
                .frame(maxWidth: 460)
                .frame(maxWidth: .infinity)
                .animation(.spring(response: 0.35, dampingFraction: 0.85), value: error?.message)
            }
            .scrollDismissesKeyboard(.interactively)
            // The bar of Previous, Next, and Done floats over the keyboard on
            // iOS 26 rather than being part of it, so the room the keyboard
            // takes doesn't count it: the focused field is kept above it too.
            .safeAreaPadding(.bottom, focus == nil ? 0 : Self.keyboardBarClearance)
            .onChange(of: focus) { _, field in
                guard let field else { return }
                // After the keyboard and the room above it have settled.
                Task {
                    try? await Task.sleep(for: .milliseconds(50))
                    withAnimation(.easeOut(duration: 0.25)) {
                        scroller.scrollTo(field, anchor: UnitPoint(x: 0.5, y: 0.9))
                    }
                }
            }
        }
        .toolbar {
            ToolbarItemGroup(placement: .keyboard) {
                Button { moveFocus(by: -1) } label: { Icon(.chevronUp, relativeTo: nil) }
                    .disabled(focus == Field.allCases.first)
                    .accessibilityLabel("Previous field")
                    .accessibilityIdentifier("keyboard.previous")
                Button { moveFocus(by: 1) } label: { Icon(.chevronDown, relativeTo: nil) }
                    .disabled(focus == Field.allCases.last)
                    .accessibilityLabel("Next field")
                    .accessibilityIdentifier("keyboard.next")
                Spacer()
                if #available(iOS 26, *) {
                    Button(role: .confirm) { focus = nil }
                        .accessibilityIdentifier("keyboard.done")
                } else {
                    Button("Done") { focus = nil }
                        .fontWeight(.semibold)
                        .accessibilityIdentifier("keyboard.done")
                }
            }
        }
        // A sign-in that couldn't get through while iOS asked for the Local
        // Network permission tries again once the prompt is answered.
        .onChange(of: scenePhase) { old, phase in
            if old == .inactive, phase == .active, error?.retriesWhenActive == true {
                submit()
            }
        }
        .background(backdrop)
        .sensoryFeedback(.error, trigger: failures)
        .onAppear {
            if let account = model.signedOutAccount {
                server = account.serverURL.absoluteString
                username = account.username
            }
        }
    }

    /// The app icon as the home screen shows it: debug builds are Shisho
    /// Dev, whose icon is blue (`AppIconDev`).
    #if DEBUG
    private static let logo = "LogoDev"
    #else
    private static let logo = "Logo"
    #endif

    private var brand: some View {
        VStack(spacing: 18) {
            Image(SignInView.logo)
                .resizable()
                .scaledToFit()
                .frame(width: 84, height: 84)
                .clipShape(RoundedRectangle(cornerRadius: 20, style: .continuous))
                .shadow(color: Theme.primary.opacity(0.35), radius: 22, y: 10)
                .accessibilityHidden(true)
            Wordmark(size: 30, showsMark: false)
            Text("Your library, in your pocket.")
                .font(.geist(.callout))
                .foregroundStyle(Theme.mutedForeground)
        }
    }

    /// A soft glow of the primary color behind the brand.
    private var backdrop: some View {
        ZStack(alignment: .top) {
            Theme.background
            RadialGradient(colors: [Theme.primary.opacity(0.16), .clear], center: .top, startRadius: 0, endRadius: 420)
                .frame(height: 520)
        }
        .ignoresSafeArea()
        .accessibilityHidden(true)
    }

    private func moveFocus(by step: Int) {
        guard let focus, let index = Field.allCases.firstIndex(of: focus) else { return }
        let next = index + step
        if Field.allCases.indices.contains(next) {
            self.focus = Field.allCases[next]
        }
    }

    private func submit() {
        guard canSubmit else { return }
        busy = true
        error = nil
        Task {
            do {
                try await model.signIn(server: server, username: username, password: password)
            } catch let signInError as SignInError {
                error = signInError
                failures += 1
            } catch {
                self.error = SignInError(message: error.localizedDescription)
                failures += 1
            }
            busy = false
        }
    }
}

/// The accounts still signed in on this device, under the sign-in screen
/// for an account whose sign-in ended, so the user's other libraries stay
/// a tap away.
private struct OtherAccounts: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        let accounts = model.otherSignedInAccounts
        if !accounts.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                Text("Or open another account")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityAddTraits(.isHeader)
                VStack(spacing: 0) {
                    ForEach(Array(accounts.enumerated()), id: \.element.id) { index, account in
                        if index > 0 { Divider().overlay(Theme.border) }
                        Button { model.switchAccount(to: account.id) } label: {
                            HStack(spacing: 12) {
                                AccountAvatar(account: account, size: 32)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(account.username)
                                        .font(.geist(.subheadline, weight: .semibold))
                                        .foregroundStyle(Theme.foreground)
                                    Text(account.serverURL.host() ?? account.serverURL.absoluteString)
                                        .font(.geist(.footnote))
                                        .foregroundStyle(Theme.mutedForeground)
                                        .lineLimit(1)
                                }
                                Spacer()
                                Icon(.chevronRight, size: 16, relativeTo: .subheadline)
                                    .foregroundStyle(Theme.mutedForeground)
                            }
                            .padding(.horizontal, 16)
                            .frame(minHeight: 56)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel(String(localized: "\(account.username) on \(account.serverURL.host() ?? account.serverURL.absoluteString)"))
                        .accessibilityHint(String(localized: "Opens this account"))
                        .accessibilityIdentifier("signin.otherAccount")
                    }
                }
                .cardStyle()
            }
        }
    }
}
