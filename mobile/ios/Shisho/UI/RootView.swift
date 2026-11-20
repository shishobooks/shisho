import SwiftUI

struct RootView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        ZStack {
            if model.session != nil {
                LibraryView()
                    .transition(.opacity.combined(with: .scale(scale: 1.02)))
            } else {
                SignInView()
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: model.session != nil)
        .background(Theme.background)
    }
}

/// Tracks how far a scroll view has scrolled, for headers that collapse
/// into the navigation bar. Works back to iOS 17, which has no scroll
/// geometry callbacks.
struct ScrollOffsetKey: PreferenceKey {
    static let defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = nextValue()
    }
}

extension View {
    /// Reports this view's top edge in the named scroll coordinate space.
    func readsScrollOffset(in space: String) -> some View {
        background(GeometryReader { geometry in
            Color.clear.preference(key: ScrollOffsetKey.self, value: geometry.frame(in: .named(space)).minY)
        })
    }

    /// The zoom from a cover in the grid to its Book, on iOS 18 and later.
    @ViewBuilder
    func zoomSource(id: Int, in namespace: Namespace.ID) -> some View {
        if #available(iOS 18.0, *) {
            matchedTransitionSource(id: id, in: namespace)
        } else {
            self
        }
    }

    @ViewBuilder
    func zoomDestination(id: Int, in namespace: Namespace.ID) -> some View {
        if #available(iOS 18.0, *) {
            navigationTransition(.zoom(sourceID: id, in: namespace))
        } else {
            self
        }
    }
}
