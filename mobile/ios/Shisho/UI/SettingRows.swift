import SwiftUI

/// A setting that's on or off, as every screen shows one: its name beside
/// its switch, with what it does in smaller text under the name. The whole
/// row is the control, so a tap anywhere on it flips the switch, and
/// VoiceOver reads one switch with its name, its state, and what it does.
struct SettingToggle: View {
    let title: LocalizedStringResource
    var description: LocalizedStringResource?
    @Binding var isOn: Bool
    let identifier: String
    /// The description stands out, as when it says why something waits.
    var emphasized = false

    var body: some View {
        Button { isOn.toggle() } label: {
            HStack(alignment: .center, spacing: 16) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(.geist(.subheadline, weight: .medium))
                        .foregroundStyle(Theme.foreground)
                    if let description {
                        Text(description)
                            .font(.geist(.footnote))
                            .foregroundStyle(emphasized ? Theme.foreground : Theme.mutedForeground)
                            .fixedSize(horizontal: false, vertical: true)
                            .accessibilityIdentifier(identifier + ".note")
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                // Drawn only; the row is the control.
                Toggle(isOn: $isOn) { Text(title) }
                    .labelsHidden()
                    .tint(Theme.primary)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
            }
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(title))
        // As a switch's own: "1" or "0", which VoiceOver reads as on or off
        // for a toggle, and the harness reads as checked.
        .accessibilityValue(Text(verbatim: isOn ? "1" : "0"))
        .accessibilityHint(description.map { Text($0) } ?? Text(verbatim: ""))
        .accessibilityAddTraits(.isToggle)
        .accessibilityIdentifier(identifier)
    }
}

/// Settings grouped in a card, as iOS's own settings group them: rows
/// apart by `SettingsDivider`s.
struct SettingsCard<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            content
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .cardStyle()
    }
}

/// The hairline between two rows of a `SettingsCard`.
struct SettingsDivider: View {
    var body: some View {
        Divider()
            .overlay(Theme.border)
            .padding(.vertical, 10)
    }
}
