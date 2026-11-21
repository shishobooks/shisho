// swift-tools-version:6.0
// The Shisho API types, generated from the Go structs by `mise tygo`
// (ADR 0021) into Sources/ShishoAPI/Generated, which is gitignored.
import PackageDescription

let package = Package(
    name: "ShishoAPI",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "ShishoAPI", targets: ["ShishoAPI"]),
    ],
    targets: [
        .target(name: "ShishoAPI"),
        .testTarget(name: "ShishoAPITests", dependencies: ["ShishoAPI"]),
    ]
)
