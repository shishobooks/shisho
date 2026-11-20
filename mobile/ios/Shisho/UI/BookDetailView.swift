import CoreImage
import SwiftUI

/// A Book from the device's copy of the catalog: its cover, title, and
/// authors, with the cover's color washed behind them.
struct BookDetailView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    let bookID: Int

    @State private var book: BookDetail?
    @State private var tint: Color?

    var body: some View {
        ScrollView {
            if let book {
                content(book)
            } else {
                ContentUnavailableView("This Book isn't in your Libraries anymore", systemImage: "book.closed")
                    .padding(.top, 80)
            }
        }
        .background(alignment: .top) {
            LinearGradient(colors: [(tint ?? .clear).opacity(0.45), .clear], startPoint: .top, endPoint: .bottom)
                .frame(height: 480)
                .ignoresSafeArea()
                .animation(.easeOut(duration: 0.4), value: tint)
        }
        .navigationTitle(book?.summary.title ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .task(id: model.catalogRevision) {
            book = try? model.session?.store.book(id: bookID)
        }
    }

    private func content(_ book: BookDetail) -> some View {
        VStack(spacing: 0) {
            CoverView(book: book.summary, width: sizeClass == .regular ? 240 : 176) { image in
                tint = averageColor(of: image)
            }
            .shadow(color: .black.opacity(0.18), radius: 12, y: 8)
            .padding(.top, 8)

            Text(book.summary.title)
                .font(.geist(.title2, weight: .bold))
                .multilineTextAlignment(.center)
                .padding(.top, 20)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("book.detail.title")
            if let subtitle = book.subtitle, !subtitle.isEmpty {
                Text(subtitle)
                    .font(.geist(.callout))
                    .foregroundStyle(Theme.mutedForeground)
                    .multilineTextAlignment(.center)
                    .padding(.top, 4)
            }
            if !book.authors.isEmpty {
                Text(book.authors.map(\.name).joined(separator: ", "))
                    .font(.geist(.callout, weight: .medium))
                    .multilineTextAlignment(.center)
                    .padding(.top, 8)
                    .accessibilityLabel(String(localized: "By \(book.authors.map(\.name).joined(separator: ", "))"))
                    .accessibilityIdentifier("book.detail.authors")
            }
            if let series = book.series.first {
                Text(seriesLine(series))
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .padding(.top, 4)
            }

            if !book.files.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Files")
                        .font(.geist(.subheadline))
                        .foregroundStyle(Theme.mutedForeground)
                        .accessibilityAddTraits(.isHeader)
                    ForEach(book.files) { file in
                        FileRow(file: file)
                    }
                }
                .padding(.top, 28)
            }
        }
        .padding(.horizontal, 20)
        .padding(.bottom, 32)
        .frame(maxWidth: 640)
        .frame(maxWidth: .infinity)
    }

    private func seriesLine(_ series: BookDetail.Series) -> String {
        guard let number = series.number else { return series.name }
        let format: (Double) -> String = { $0.formatted(.number.precision(.fractionLength(0...2))) }
        if let end = series.numberEnd {
            return String(localized: "\(series.name) #\(format(number))–\(format(end))")
        }
        return String(localized: "\(series.name) #\(format(number))")
    }

    private func averageColor(of image: UIImage) -> Color? {
        guard let input = CIImage(image: image) else { return nil }
        let filter = CIFilter(name: "CIAreaAverage", parameters: [kCIInputImageKey: input, kCIInputExtentKey: CIVector(cgRect: input.extent)])
        guard let output = filter?.outputImage else { return nil }
        var pixel = [UInt8](repeating: 0, count: 4)
        CIContext().render(output, toBitmap: &pixel, rowBytes: 4, bounds: CGRect(x: 0, y: 0, width: 1, height: 1), format: .RGBA8, colorSpace: nil)
        return Color(red: Double(pixel[0]) / 255, green: Double(pixel[1]) / 255, blue: Double(pixel[2]) / 255)
    }
}

private struct FileRow: View {
    let file: BookDetail.File

    /// File Types this version knows; any other is shown as not supported
    /// (ADR 0021).
    private static let known: [String: String] = [
        "epub": "EPUB", "kepub": "KePub", "cbz": "CBZ", "pdf": "PDF", "m4b": "M4B", "mobi": "MOBI", "azw3": "AZW3",
    ]

    var body: some View {
        HStack {
            if let name = FileRow.known[file.fileType] {
                Text(name)
                    .font(.geist(.subheadline, weight: .semibold))
            } else {
                Text("Not supported in this version of the app")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
            }
            if file.fileRole == "supplement" {
                Text("Supplement")
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
            }
            Spacer()
            Text(ByteCountFormatter.string(fromByteCount: Int64(file.sizeBytes), countStyle: .file))
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
                .monospacedDigit()
        }
        .padding(12)
        .background(Theme.card, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(Theme.border))
        .accessibilityElement(children: .combine)
    }
}
