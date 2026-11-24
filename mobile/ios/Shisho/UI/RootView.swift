import SwiftUI

struct RootView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        ZStack {
            if model.session != nil {
                MainTabs()
                    // Another account's screens start fresh, never showing
                    // the last one's rows while its own load.
                    .id(model.session?.account.id)
                    .transition(.opacity.combined(with: .scale(scale: 1.02)))
                    // Under the reader, out of VoiceOver's way.
                    .accessibilityHidden(model.reader != nil)
            } else {
                SignInView()
                    .transition(.opacity)
            }
            // Over everything, as a full-screen cover would be, but not one:
            // iOS 26 leaves a sheet with detents, as the reader's contents
            // and settings, out of the accessibility tree when it's over a
            // full-screen cover, so VoiceOver couldn't reach them. Sheets
            // from here present as the player's do.
            if let reader = model.reader {
                ReaderView(model: reader) { model.closeReader() }
                    .id(reader.id)
                    // Slides in as one piece: without it, the bars and a
                    // message the reader shows as it opens appear in place
                    // at once while the page slides up under them.
                    .geometryGroup()
                    .transition(.move(edge: .bottom))
                    .zIndex(1)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: model.session != nil)
        .animation(.easeInOut(duration: 0.3), value: model.reader?.id)
        .background(Theme.background)
        .sheet(isPresented: $model.showingPlayer) {
            NowPlayingView()
        }
        .sheet(isPresented: $model.addingAccount, onDismiss: model.addServerClosed) {
            // A sheet of its own, with a bar to cancel from.
            NavigationStack {
                SignInView(addingAccount: true)
                    .navigationTitle("Add Server")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button("Cancel") { model.addingAccount = false }
                                .accessibilityIdentifier("signin.cancel")
                        }
                    }
            }
            .tint(Theme.primary)
            .presentationDragIndicator(.visible)
        }
    }
}

/// How far a scroll view has scrolled down from its top, for headers that
/// shrink or appear as content scrolls under them.
///
/// iOS 18 and later ask the scroll view. iOS 17 has no scroll geometry
/// callbacks, so there a marker at the top of the content reports where it
/// is: put `ScrollTopMarker(space:)` first in the content.
private struct ScrollTopKey: PreferenceKey {
    static let defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = nextValue()
    }
}

struct ScrollTopMarker: View {
    let space: String

    var body: some View {
        Color.clear
            .frame(height: 0)
            .background(GeometryReader { geometry in
                Color.clear.preference(key: ScrollTopKey.self, value: geometry.frame(in: .named(space)).minY)
            })
    }
}

extension View {
    /// Calls action with how far this scroll view has scrolled down from
    /// its top, 0 when it's at the top or pulled down past it.
    @ViewBuilder
    func onScrolledDistanceChange(space: String, _ action: @escaping (CGFloat) -> Void) -> some View {
        if #available(iOS 18.0, *) {
            onScrollGeometryChange(for: CGFloat.self) { geometry in
                max(0, geometry.contentOffset.y + geometry.contentInsets.top)
            } action: { _, distance in
                action(distance)
            }
        } else {
            coordinateSpace(.named(space))
                .onPreferenceChange(ScrollTopKey.self) { top in action(max(0, -top)) }
        }
    }
}
