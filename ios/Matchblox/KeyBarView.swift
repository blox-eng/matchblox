import MatchbloxKit
import SwiftTerm
import UIKit

/// The keys of `KeyBar` in one row, in the console's tokens. A key acts at touch-down, so the
/// first tap always counts. One bar rides on the keyboard and one docks under the terminal
/// while the keyboard is down, so each knows which way `⌨` goes.
final class KeyBarView: UIInputView {
    private static let gap: CGFloat = 4
    private static let radius: CGFloat = 6
    private static let font = UIFont.monospacedSystemFont(ofSize: 15, weight: .medium)
    private static let arrowFont = UIFont.monospacedSystemFont(ofSize: 22, weight: .medium)

    private weak var terminal: TerminalView?
    private let keyboardUp: Bool
    private let haptic = UIImpactFeedbackGenerator(style: .light)

    init(terminal: TerminalView, keyboardUp: Bool) {
        self.terminal = terminal
        self.keyboardUp = keyboardUp
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: KeyBar.height + 2 * Self.gap), inputViewStyle: .default)
        allowsSelfSizing = true
        backgroundColor = UIColor(Theme.bg)
        haptic.prepare()

        let buttons = KeyBar.keys.map { button(for: $0) }
        let row = UIStackView(arrangedSubviews: buttons)
        row.axis = .horizontal
        row.alignment = .center
        row.spacing = Self.gap
        row.translatesAutoresizingMaskIntoConstraints = false
        addSubview(row)
        // Below required, so the docked bar can collapse when its stack view hides it.
        let bottom = row.bottomAnchor.constraint(equalTo: safeAreaLayoutGuide.bottomAnchor, constant: -Self.gap)
        bottom.priority = .init(999)
        NSLayoutConstraint.activate([
            row.topAnchor.constraint(equalTo: topAnchor, constant: Self.gap),
            bottom,
            row.leadingAnchor.constraint(equalTo: safeAreaLayoutGuide.leadingAnchor, constant: Self.gap),
            row.trailingAnchor.constraint(equalTo: safeAreaLayoutGuide.trailingAnchor, constant: -Self.gap),
        ])

        // Each key is a multiple of the first one's width, and that one keeps the floor where
        // the row has room. A view narrower than the keys (any iPhone upright) shrinks them.
        let unit = buttons[0]
        let floor = unit.widthAnchor.constraint(greaterThanOrEqualToConstant: KeyBar.minSide)
        floor.priority = .init(999)
        floor.isActive = true
        for (key, button) in zip(KeyBar.keys, buttons).dropFirst() {
            button.widthAnchor.constraint(equalTo: unit.widthAnchor, multiplier: key.width).isActive = true
        }
    }

    required init?(coder: NSCoder) { nil }

    private func button(for key: KeyBar.Key) -> UIButton {
        let button = KeyButton(frame: .zero)
        let lit = key.action == .keyboard && keyboardUp
        button.setTitle(key.label, for: .normal)
        button.setTitleColor(UIColor(lit ? Theme.accent : Theme.fg), for: .normal)
        button.titleLabel?.font = key.width > 1 ? Self.arrowFont : Self.font
        button.titleLabel?.adjustsFontSizeToFitWidth = true
        button.titleLabel?.minimumScaleFactor = 0.5
        button.layer.cornerRadius = Self.radius
        button.accessibilityLabel = key.action == .keyboard
            ? (keyboardUp ? "Hide keyboard" : "Show keyboard")
            : key.accessibilityLabel
        button.addAction(UIAction { [weak self] _ in self?.press(key) }, for: .touchDown)
        button.heightAnchor.constraint(equalToConstant: key.height).isActive = true
        return button
    }

    private func press(_ key: KeyBar.Key) {
        haptic.impactOccurred()
        haptic.prepare()
        switch key.action {
        case .send(let bytes):
            terminal?.send(bytes)
        case .keyboard:
            _ = keyboardUp ? terminal?.resignFirstResponder() : terminal?.becomeFirstResponder()
        }
    }
}

/// A key on `wash`, and on `hair` while a finger is on it.
private final class KeyButton: UIButton {
    override init(frame: CGRect) {
        super.init(frame: frame)
        backgroundColor = UIColor(Theme.wash)
    }

    required init?(coder: NSCoder) { nil }

    override var isHighlighted: Bool {
        didSet { backgroundColor = UIColor(isHighlighted ? Theme.hair : Theme.wash) }
    }
}
