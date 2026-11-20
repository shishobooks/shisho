import Foundation

/// The marketing names of iPhone and iPad models, by the model identifier
/// the hardware reports, so a new Device is named "iPhone 17 Pro" rather
/// than "iPhone". iOS gives an app the name the user chose only with an
/// entitlement Apple grants on request. A model missing here falls back to
/// "iPhone" or "iPad".
///
/// Generated from the simulator device profiles Xcode installs; to add newer
/// models, regenerate it from them:
///
///     for d in /Library/Developer/CoreSimulator/Profiles/DeviceTypes/{iPhone,iPad}*.simdevicetype; do
///       /usr/libexec/PlistBuddy -c "Print :modelIdentifier" "$d/Contents/Resources/profile.plist"; basename "$d" .simdevicetype
///     done
enum DeviceModels {
    static let names: [String: String] = [
        "iPad5,1": "iPad mini 4",
        "iPad5,4": "iPad Air 2",
        "iPad6,4": "iPad Pro (9.7-inch)",
        "iPad6,8": "iPad Pro (12.9-inch) (1st generation)",
        "iPad6,12": "iPad (5th generation)",
        "iPad7,1": "iPad Pro (12.9-inch) (2nd generation)",
        "iPad7,3": "iPad Pro (10.5-inch)",
        "iPad7,6": "iPad (6th generation)",
        "iPad7,12": "iPad (7th generation)",
        "iPad8,1": "iPad Pro (11-inch) (1st generation)",
        "iPad8,5": "iPad Pro (12.9-inch) (3rd generation)",
        "iPad8,9": "iPad Pro (11-inch) (2nd generation)",
        "iPad8,12": "iPad Pro (12.9-inch) (4th generation)",
        "iPad11,1": "iPad mini (5th generation)",
        "iPad11,3": "iPad Air (3rd generation)",
        "iPad11,7": "iPad (8th generation)",
        "iPad12,2": "iPad (9th generation)",
        "iPad13,2": "iPad Air (4th generation)",
        "iPad13,5": "iPad Pro (11-inch) (3rd generation)",
        "iPad13,10": "iPad Pro (12.9-inch) (5th generation)",
        "iPad13,17": "iPad Air (5th generation)",
        "iPad13,18": "iPad (10th generation)",
        "iPad14,1": "iPad mini (6th generation)",
        "iPad14,3": "iPad Pro (11-inch) (4th generation)",
        "iPad14,4": "iPad Pro (11-inch) (4th generation)",
        "iPad14,5": "iPad Pro (12.9-inch) (6th generation)",
        "iPad14,9": "iPad Air 11-inch (M2)",
        "iPad14,11": "iPad Air 13-inch (M2)",
        "iPad15,3": "iPad Air 11-inch (M3)",
        "iPad15,5": "iPad Air 13-inch (M3)",
        "iPad15,7": "iPad (A16)",
        "iPad16,2": "iPad mini (A17 Pro)",
        "iPad16,4": "iPad Pro 11-inch (M4)",
        "iPad16,6": "iPad Pro 13-inch (M4)",
        "iPad16,9": "iPad Air 11-inch (M4)",
        "iPad16,11": "iPad Air 13-inch (M4)",
        "iPad17,2": "iPad Pro 11-inch (M5)",
        "iPad17,4": "iPad Pro 13-inch (M5)",
        "iPad19,2": "iPad mini (A20 Pro)",
        "iPhone8,1": "iPhone 6s",
        "iPhone8,2": "iPhone 6s Plus",
        "iPhone8,4": "iPhone SE (1st generation)",
        "iPhone9,1": "iPhone 7",
        "iPhone9,2": "iPhone 7 Plus",
        "iPhone10,4": "iPhone 8",
        "iPhone10,5": "iPhone 8 Plus",
        "iPhone10,6": "iPhone X",
        "iPhone11,2": "iPhone Xs",
        "iPhone11,4": "iPhone Xs Max",
        "iPhone11,8": "iPhone Xʀ",
        "iPhone12,1": "iPhone 11",
        "iPhone12,3": "iPhone 11 Pro",
        "iPhone12,5": "iPhone 11 Pro Max",
        "iPhone12,8": "iPhone SE (2nd generation)",
        "iPhone13,1": "iPhone 12 mini",
        "iPhone13,2": "iPhone 12",
        "iPhone13,3": "iPhone 12 Pro",
        "iPhone13,4": "iPhone 12 Pro Max",
        "iPhone14,2": "iPhone 13 Pro",
        "iPhone14,3": "iPhone 13 Pro Max",
        "iPhone14,4": "iPhone 13 mini",
        "iPhone14,5": "iPhone 13",
        "iPhone14,6": "iPhone SE (3rd generation)",
        "iPhone14,7": "iPhone 14",
        "iPhone14,8": "iPhone 14 Plus",
        "iPhone15,2": "iPhone 14 Pro",
        "iPhone15,3": "iPhone 14 Pro Max",
        "iPhone15,4": "iPhone 15",
        "iPhone15,5": "iPhone 15 Plus",
        "iPhone16,1": "iPhone 15 Pro",
        "iPhone16,2": "iPhone 15 Pro Max",
        "iPhone17,1": "iPhone 16 Pro",
        "iPhone17,2": "iPhone 16 Pro Max",
        "iPhone17,3": "iPhone 16",
        "iPhone17,4": "iPhone 16 Plus",
        "iPhone17,5": "iPhone 16e",
        "iPhone18,1": "iPhone 17 Pro",
        "iPhone18,2": "iPhone 17 Pro Max",
        "iPhone18,3": "iPhone 17",
        "iPhone18,4": "iPhone Air",
        "iPhone18,5": "iPhone 17e",
        "iPhone19,2": "iPhone 18 Pro",
        "iPhone19,3": "iPhone 18 Pro Max",
        "iPhone19,4": "iPhone Duo",
    ]

    /// The hardware's model identifier, such as "iPhone18,1", or the one a
    /// simulator stands in for.
    static var identifier: String {
        if let simulated = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] {
            return simulated
        }
        var info = utsname()
        uname(&info)
        return withUnsafeBytes(of: &info.machine) { bytes in
            String(decoding: bytes.prefix { $0 != 0 }, as: UTF8.self)
        }
    }

    /// This device's marketing name, or nil for a model the table doesn't
    /// know.
    static var current: String? {
        names[identifier]
    }
}
