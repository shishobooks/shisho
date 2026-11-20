import SwiftUI
import UIKit

/// Keeps a page opened with the zoom transition open when it's pulled down,
/// so the pull refreshes it, while a swipe from the left edge still goes
/// back. SwiftUI's zoom transition has no option for which gestures dismiss
/// it, so this adds a pan that claims downward drags on the page and, through
/// UIKit's public failure requirements, makes other pans wait for it to fail.
/// It never claims a sideways drag, and it leaves the scroll view's own pan,
/// taps, and the system's gestures alone.
///
/// Put it in the page's background, where it covers the page.
struct PullDownKeepsPage: UIViewRepresentable {
    func makeUIView(context: Context) -> Anchor { Anchor() }
    func updateUIView(_ view: Anchor, context: Context) {}

    final class Anchor: UIView {
        private lazy var pan = DownwardPan(page: self)

        /// Joins the nearest view controller's view, which receives every
        /// touch on the page, while the page is on screen.
        override func didMoveToWindow() {
            super.didMoveToWindow()
            guard window != nil else {
                pan.view?.removeGestureRecognizer(pan)
                return
            }
            var host = superview
            while let view = host, !(view.next is UIViewController) {
                host = view.superview
            }
            host?.addGestureRecognizer(pan)
        }
    }

    private final class DownwardPan: UIPanGestureRecognizer, UIGestureRecognizerDelegate {
        private weak var page: UIView?

        init(page: UIView) {
            self.page = page
            super.init(target: nil, action: nil)
            delegate = self
            cancelsTouchesInView = false
            delaysTouchesBegan = false
        }

        func gestureRecognizerShouldBegin(_ recognizer: UIGestureRecognizer) -> Bool {
            guard let page, page.bounds.contains(location(in: page)) else { return false }
            let velocity = velocity(in: page)
            return velocity.y > abs(velocity.x)
        }

        func gestureRecognizer(_ recognizer: UIGestureRecognizer,
                               shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool {
            true
        }

        func gestureRecognizer(_ recognizer: UIGestureRecognizer,
                               shouldBeRequiredToFailBy other: UIGestureRecognizer) -> Bool {
            !(other.view is UIScrollView) && !(other.view is UIWindow)
                && !(other is UITapGestureRecognizer) && !(other is UILongPressGestureRecognizer)
        }
    }
}
