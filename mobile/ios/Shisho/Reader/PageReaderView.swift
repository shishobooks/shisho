import SwiftUI
import UIKit

/// The reader for a Page-based File, a CBZ or a PDF: its pages on black,
/// as the web's page reader shows them, with the reader's bars over them
/// that a tap in the middle shows and hides.
struct PageReaderView: View {
    let model: PageReaderModel
    @State private var showingContents = false
    @State private var showingSettings = false
    let close: () -> Void

    private static let pageForeground = Color.white

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            if let source = model.source {
                PagerHost(pager: source.pager)
                    .id(ObjectIdentifier(source))
                    .padding(.bottom, 24)
                    .accessibilityIdentifier("reader.page")
                    .opacity(model.phase == .reading ? 1 : 0)
                    .accessibilityHidden(model.phase != .reading || showingContents || showingSettings)
            }
            status
            if model.turnWaiting {
                ProgressView()
                    .tint(Self.pageForeground)
                    .padding(14)
                    .background(.ultraThinMaterial, in: Circle())
                    .allowsHitTesting(false)
                    .accessibilityLabel(Text("Loading the next page"))
                    .accessibilityIdentifier("reader.turnLoading")
            }
            VStack(spacing: 0) {
                if model.chromeVisible { topBar.transition(.move(edge: .top).combined(with: .opacity)) }
                if let offer = model.offer, model.phase == .reading {
                    PlaceOfferBanner(offer: offer, kind: .reading, accept: model.acceptOffer, dismiss: model.dismissOffer)
                        .padding(.horizontal, 12)
                        .padding(.top, 8)
                        .transition(.move(edge: .top).combined(with: .opacity))
                }
                Spacer(minLength: 0)
                if model.phase == .reading {
                    if model.chromeVisible {
                        PageReaderBottomBar(model: model).transition(.move(edge: .bottom).combined(with: .opacity))
                    } else {
                        readout
                    }
                }
            }
        }
        .animation(.easeOut(duration: 0.18), value: model.chromeVisible)
        .animation(.easeOut(duration: 0.2), value: model.offer)
        .animation(.easeOut(duration: 0.15), value: model.turnWaiting)
        .statusBarHidden(model.phase == .reading)
        .task { await model.load() }
        .onAppear { ScreenAwake.hold() }
        .onDisappear { ScreenAwake.release() }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didReceiveMemoryWarningNotification)) { _ in
            model.memoryWarning()
        }
        .sheet(isPresented: $showingContents) {
            ContentsSheet(entries: model.contents, currentID: model.currentEntry?.id) { entry in
                showingContents = false
                model.go(to: entry)
            }
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
            .presentationBackground(Theme.background)
            .tint(Theme.primary)
        }
        .sheet(isPresented: $showingSettings) {
            PageReaderSettingsSheet(model: model) { showingSettings = false }
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
                .presentationBackground(Theme.background)
        }
    }

    private var status: some View {
        ReaderStatus(phase: model.phase, foreground: Self.pageForeground) { Task { await model.load() } }
    }

    // MARK: - Controls

    /// The room each side of the title keeps for the buttons: the two on
    /// the right, which don't grow with the text size.
    private static let topBarSide: CGFloat = 2 * 44 + 8

    /// The bars and sheets take the app's look; the pages stay on black.
    private var topBar: some View {
        ReaderTopBar(sideRoom: Self.topBarSide, close: close) {
            PageReaderTitle(model: model)
        } buttons: {
            topBarButtons
        }
    }

    private var topBarButtons: some View {
        HStack(spacing: 0) {
            Button { showingContents = true } label: {
                Icon(.list, size: 20)
                    .frame(minWidth: 44, minHeight: 44)
            }
            // A PDF's outline and a comic's chapters are Shisho's chapters.
            .disabled(model.contents.isEmpty || model.phase != .reading)
            .accessibilityLabel(Text("Contents"))
            .accessibilityShowsLargeContentViewer()
            .accessibilityIdentifier("reader.contents")
            Button { showingSettings = true } label: {
                Icon(.settings, size: 20)
                    .frame(minWidth: 44, minHeight: 44)
            }
            .disabled(model.phase != .reading)
            .accessibilityLabel(Text("Settings"))
            .accessibilityShowsLargeContentViewer()
            .accessibilityIdentifier("reader.settings")
        }
        .foregroundStyle(Theme.foreground)
    }

    /// Where the reader is, small under the page while the controls are
    /// hidden.
    private var readout: some View {
        HStack(spacing: 4) {
            Text(model.pagesLabel())
                .lineLimit(1)
            Text(verbatim: "·")
            Text(model.percentShown)
        }
        .font(.geist(.caption2, weight: .medium))
        .monospacedDigit()
        .foregroundStyle(Self.pageForeground.opacity(0.55))
        .padding(.horizontal, 24)
        .frame(maxWidth: .infinity, minHeight: 22)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(model.placeLabel))
        .accessibilityIdentifier("reader.progress")
    }
}

/// The chapter and the book over the page. Its own view, so following
/// every turn redraws only it.
private struct PageReaderTitle: View {
    let model: PageReaderModel

    var body: some View {
        ReaderTitleText(chapter: model.chapterTitle, book: model.file.book.title)
    }
}

/// The bar under the pages: back and forward through jumps, the thumbnail
/// scrubber, and the page and how far through. The scrubber runs the way
/// the pages turn: right to left in a right-to-left File.
private struct PageReaderBottomBar: View {
    let model: PageReaderModel
    /// The page under the scrubber's handle while it's dragged.
    @State private var scrubbing: Int?
    @Environment(\.verticalSizeClass) private var verticalSizeClass

    private var lastPage: Int { max(0, model.pageCount - 1) }

    private var scrubValue: String {
        let page = scrubbing ?? model.page
        return [model.chapterTitle(atPage: page), model.pagesLabel([page])].compactMap { $0 }.joined(separator: ", ")
    }

    private var scrubberRow: some View {
        HStack(spacing: 2) {
            ReaderHistoryButton(icon: .undo2, label: "Back to previous location", enabled: model.history.canGoBack,
                          identifier: "reader.back", action: model.goBack)
            Scrubber(value: Double(scrubbing ?? model.page), length: Double(lastPage), onDrag: { value in
                scrubbing = value.map { min(lastPage, max(0, Int($0.rounded()))) }
            }, onCommit: { value in
                scrubbing = nil
                model.go(toPage: min(lastPage, max(0, Int(value.rounded()))))
            }, step: 1, onAdjust: { model.go(toPage: min(lastPage, max(0, Int($0.rounded()))), adjusting: true) },
            marks: model.marks.map { $0 * Double(lastPage) })
            // Mirrored, touches and all, so it runs the way the pages turn.
            .scaleEffect(x: model.rightToLeft ? -1 : 1, y: 1)
            .disabled(lastPage == 0)
            .accessibilityLabel(Text("Place in the book"))
            .accessibilityValue(Text(scrubValue))
            .accessibilityIdentifier("reader.scrubber")
            ReaderHistoryButton(icon: .redo2, label: "Forward to next location", enabled: model.history.canGoForward,
                          identifier: "reader.forward", action: model.goForward)
        }
    }

    private func placeRow(showingChapter: Bool) -> some View {
        HStack(spacing: 6) {
            if showingChapter {
                Text(model.chapterTitle ?? "")
                    .lineLimit(1)
                    .accessibilityHidden(true)
                Spacer(minLength: 12)
            }
            Text(verbatim: "\(model.pagesLabel()) · \(model.percentShown)")
                .monospacedDigit()
                .lineLimit(1)
                .accessibilityLabel(Text(model.placeLabel))
                .accessibilityIdentifier("reader.progress")
        }
        .font(.geist(.caption))
        .foregroundStyle(Theme.mutedForeground)
        .accessibilityElement(children: .contain)
    }

    var body: some View {
        Group {
            if verticalSizeClass == .compact {
                HStack(spacing: 12) {
                    scrubberRow
                    placeRow(showingChapter: false).fixedSize()
                }
            } else {
                VStack(spacing: 4) {
                    scrubberRow
                    placeRow(showingChapter: true)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .padding(.bottom, 6)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .background(ReaderView.barBackground.ignoresSafeArea(edges: [.bottom, .horizontal]))
        .overlay(alignment: .top) { Theme.border.frame(height: 1).ignoresSafeArea(edges: .horizontal) }
        // The page a drag would go to, as a thumbnail above the bar where a
        // finger doesn't cover it.
        .overlay(alignment: .top) {
            if let scrubbing, let cache = model.source?.cache {
                PagePreview(cache: cache, page: scrubbing, label: model.pagesLabel([scrubbing]),
                            chapter: model.chapterTitle(atPage: scrubbing))
                    .frame(height: 0, alignment: .bottom)
                    .offset(y: -12)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
                    .transition(.opacity)
            }
        }
        .animation(.easeOut(duration: 0.12), value: scrubbing == nil)
    }
}

/// A page's thumbnail with its number, over the bar while the scrubber is
/// dragged.
private struct PagePreview: View {
    let cache: PageImageCache
    let page: Int
    let label: String
    let chapter: String?
    @State private var image: UIImage?
    @State private var shownPage: Int?

    /// The thumbnail's longer side, in points.
    private static let side: CGFloat = 150

    var body: some View {
        VStack(spacing: 8) {
            ZStack {
                Color.black
                if let image {
                    Image(uiImage: image)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                }
            }
            .frame(width: Self.side * 0.7, height: Self.side)
            .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 6, style: .continuous).strokeBorder(Theme.primary, lineWidth: 2))
            VStack(spacing: 2) {
                if let chapter {
                    Text(chapter)
                        .font(.geist(.footnote, weight: .semibold))
                        .lineLimit(1)
                }
                Text(label)
                    .font(.geist(.footnote))
                    .monospacedDigit()
            }
            .foregroundStyle(Theme.primaryForeground)
            .padding(.horizontal, 14)
            .padding(.vertical, 6)
            .background(Theme.primary, in: Capsule())
        }
        .shadow(color: .black.opacity(0.3), radius: 10, y: 4)
        .padding(.horizontal, 24)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .task(id: page) {
            // Not for every page a fast drag crosses: only once it rests.
            if shownPage != nil { try? await Task.sleep(for: .milliseconds(80)) }
            guard !Task.isCancelled else { return }
            let pixels = Int(Self.side * 3)
            if let kept = cache.cached(page: page, maxPixel: pixels) {
                image = kept
            } else {
                let decoded = await cache.image(page: page, maxPixel: pixels, need: .later)
                guard !Task.isCancelled else { return }
                image = decoded
            }
            shownPage = page
        }
    }
}

/// The page reader's settings, as the web's page reader names them: the
/// flow, kept for the Book, and whether right-to-left Files turn that way,
/// kept on this device.
private struct PageReaderSettingsSheet: View {
    let model: PageReaderModel
    let done: () -> Void

    private var flowHint: LocalizedStringResource {
        guard model.flowChosen else { return "Picked from the page shapes until you choose." }
        return model.flowSyncs ? "Saved for this book." : "Saved for this book on this device."
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    ChoiceSet(title: "Flow", options: PageFlow.allCases,
                              selection: Binding(get: { model.flow }, set: { model.choose($0) }), label: \.label,
                              identifier: "reader.flow",
                              hint: flowHint)
                    VStack(alignment: .leading, spacing: 4) {
                        SettingToggle(title: "Follow Reading Direction",
                                      isOn: Binding(get: { model.followReadingDirection }, set: { model.followReadingDirection = $0 }),
                                      identifier: "reader.followReadingDirection")
                        Group {
                            if model.followReadingDirection {
                                Text("Right to left books turn the other way. Saved on this device.")
                            } else {
                                Text("Every book turns left to right. Saved on this device.")
                            }
                        }
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.top, 8)
                .padding(.bottom, 24)
                .frame(maxWidth: 560, alignment: .leading)
                .frame(maxWidth: .infinity)
            }
            .foregroundStyle(Theme.foreground)
            .background(Theme.background)
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(Theme.background, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done", action: done)
                        .font(.geist(.subheadline, weight: .semibold))
                        .foregroundStyle(Theme.primary)
                        .accessibilityIdentifier("reader.settings.done")
                }
            }
        }
        .tint(Theme.primary)
    }
}
