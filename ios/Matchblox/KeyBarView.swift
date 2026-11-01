import MatchbloxKit
import SwiftTerm
import UIKit

/// The keys of `KeyBar` in one row under a hairline, in the console's tokens. The hit areas meet
/// edge to edge at the bar's full height; each key is drawn inset within its own. A key acts at
/// touch-down, so the first tap always counts. One bar rides on the keyboard and one docks under
/// the terminal while the keyboard is down, so each knows which way `⌨` goes.
final class KeyBarView: UIInputView {
    private static let inset: CGFloat = 2
    private static let radius: CGFloat = 6
    private static let hairline: CGFloat = 1
    private static let rowHeight = KeyBar.height + 2 * inset
    private static let glyphFont = UIFont.monospacedSystemFont(ofSize: 17, weight: .medium)
    private static let arrowFont = UIFont.monospacedSystemFont(ofSize: 22, weight: .medium)
    private static let nameFont = UIFont.monospacedSystemFont(ofSize: 12, weight: .medium)

    private weak var terminal: TerminalView?
    private let keyboardUp: Bool
    private let haptic = UIImpactFeedbackGenerator(style: .light)
    /// Whether the system keyboard is on screen under this bar. A hardware keyboard shows the
    /// bar alone.
    private var lit = false {
        didSet { keyboardKey?.setNeedsUpdateConfiguration() }
    }
    private weak var keyboardKey: UIButton?

    init(terminal: TerminalView, keyboardUp: Bool) {
        self.terminal = terminal
        self.keyboardUp = keyboardUp
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: Self.hairline + Self.rowHeight), inputViewStyle: .default)
        allowsSelfSizing = true
        backgroundColor = UIColor(Theme.bg)
        haptic.prepare()

        let hair = UIView()
        hair.backgroundColor = UIColor(Theme.hair)
        hair.translatesAutoresizingMaskIntoConstraints = false
        addSubview(hair)

        let buttons = KeyBar.keys.map { button(for: $0) }
        let row = UIStackView(arrangedSubviews: buttons)
        row.axis = .horizontal
        row.translatesAutoresizingMaskIntoConstraints = false
        addSubview(row)
        // Below required, so the docked bar can collapse when its stack view hides it.
        let bottom = row.bottomAnchor.constraint(equalTo: safeAreaLayoutGuide.bottomAnchor)
        bottom.priority = .init(999)
        NSLayoutConstraint.activate([
            hair.topAnchor.constraint(equalTo: topAnchor),
            hair.leadingAnchor.constraint(equalTo: leadingAnchor),
            hair.trailingAnchor.constraint(equalTo: trailingAnchor),
            hair.heightAnchor.constraint(equalToConstant: Self.hairline),
            row.topAnchor.constraint(equalTo: hair.bottomAnchor),
            bottom,
            row.leadingAnchor.constraint(equalTo: safeAreaLayoutGuide.leadingAnchor),
            row.trailingAnchor.constraint(equalTo: safeAreaLayoutGuide.trailingAnchor),
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

        if keyboardUp {
            NotificationCenter.default.addObserver(self, selector: #selector(keyboardChanged), name: UIResponder.keyboardWillChangeFrameNotification, object: nil)
        }
    }

    required init?(coder: NSCoder) { nil }

    /// The keyboard's frame holds this bar. Only a system keyboard makes it taller than the bar
    /// by more than a key.
    @objc private func keyboardChanged(_ note: Notification) {
        guard let end = note.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect else { return }
        let onScreen = end.minY < (window?.windowScene?.screen.bounds.height ?? .greatestFiniteMagnitude)
        lit = onScreen && end.height - max(bounds.height, Self.hairline + Self.rowHeight) > KeyBar.minSide
    }

    private func button(for key: KeyBar.Key) -> UIButton {
        let vertical = (Self.rowHeight - key.height) / 2
        var config = UIButton.Configuration.plain()
        config.title = key.label
        config.titleLineBreakMode = .byClipping
        config.contentInsets = .zero
        config.background.cornerRadius = Self.radius
        config.background.backgroundInsets = NSDirectionalEdgeInsets(top: vertical, leading: Self.inset, bottom: vertical, trailing: Self.inset)
        let font = key.width > 1 ? Self.arrowFont : key.label.count == 1 ? Self.glyphFont : Self.nameFont
        let fg = UIColor(Theme.fg)
        config.titleTextAttributesTransformer = UIConfigurationTextAttributesTransformer { attributes in
            var attributes = attributes
            attributes.font = font
            attributes.foregroundColor = fg
            return attributes
        }

        let button = UIButton(configuration: config)
        let isKeyboard = key.action == .keyboard
        button.configurationUpdateHandler = { [weak self] button in
            let held = button.isHighlighted || (isKeyboard && self?.lit == true)
            button.configuration?.background.backgroundColor = UIColor(held ? Theme.hair : Theme.wash)
            Self.shrinkToFit(button)
        }
        Self.shrinkToFit(button)
        if isKeyboard { keyboardKey = button }
        button.accessibilityLabel = isKeyboard
            ? (keyboardUp ? "Hide keyboard" : "Show keyboard")
            : key.accessibilityLabel
        button.addAction(UIAction { [weak self] _ in self?.press(key) }, for: .touchDown)
        button.heightAnchor.constraint(equalToConstant: Self.rowHeight).isActive = true
        return button
    }

    /// A key name (`⇧Tab`) shrinks rather than clips on a narrow key (320 pt Slide Over). Set
    /// again on each update, as the configuration may rebuild the label.
    private static func shrinkToFit(_ button: UIButton) {
        button.titleLabel?.adjustsFontSizeToFitWidth = true
        button.titleLabel?.minimumScaleFactor = 0.5
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
