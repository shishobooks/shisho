import CoreGraphics
import UIKit

/// A cover's average color, washed behind it on a Book's page.
enum CoverTint {
    struct RGB: Equatable, Sendable {
        let red: Double
        let green: Double
        let blue: Double
    }

    /// The cover's average color, or nil when it has no pixels. It runs off
    /// the main actor (`@concurrent`, whatever the caller's isolation) and on
    /// the CPU: Core Image would compile Metal shaders the first time, which
    /// takes seconds on a machine without a GPU, and on the main actor that
    /// froze the page.
    @concurrent
    static func average(of image: UIImage) async -> RGB? {
        guard let cgImage = image.cgImage else { return nil }
        return average(of: cgImage)
    }

    /// Draws the image small, which averages neighboring pixels, then
    /// averages what's left, weighting each pixel by how opaque it is.
    private static func average(of image: CGImage) -> RGB? {
        let side = 8
        var pixels = [UInt8](repeating: 0, count: side * side * 4)
        let drawn = pixels.withUnsafeMutableBytes { buffer in
            guard let space = CGColorSpace(name: CGColorSpace.sRGB),
                  let context = CGContext(
                      data: buffer.baseAddress, width: side, height: side, bitsPerComponent: 8,
                      bytesPerRow: side * 4, space: space,
                      bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
                  )
            else { return false }
            context.interpolationQuality = .medium
            context.draw(image, in: CGRect(x: 0, y: 0, width: side, height: side))
            return true
        }
        guard drawn else { return nil }
        var red = 0, green = 0, blue = 0, alpha = 0
        for pixel in stride(from: 0, to: pixels.count, by: 4) {
            red += Int(pixels[pixel])
            green += Int(pixels[pixel + 1])
            blue += Int(pixels[pixel + 2])
            alpha += Int(pixels[pixel + 3])
        }
        guard alpha > 0 else { return nil }
        // The channels are premultiplied, so dividing by the total alpha
        // undoes it.
        let total = Double(alpha)
        return RGB(red: Double(red) / total, green: Double(green) / total, blue: Double(blue) / total)
    }
}
