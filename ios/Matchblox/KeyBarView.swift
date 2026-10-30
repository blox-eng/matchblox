import MatchbloxKit
import SwiftTerm
import UIKit

/// The seven keys of `KeyBar` in one row, each as wide as the next. A key sends at touch-down,
/// so the first tap always counts.
final class KeyBarView: UIInputView {
    private static let gap: CGFloat = 4

    private weak var terminal: TerminalView?
    private let haptic = UIImpactFeedbackGenerator(style: .light)

    init(terminal: TerminalView) {
        self.terminal = terminal
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: KeyBar.minSide + 2 * Self.gap), inputViewStyle: .keyboard)
        allowsSelfSizing = true

        let row = UIStackView(arrangedSubviews: KeyBar.keys.map { button(for: $0) })
        row.axis = .horizontal
        row.distribution = .fillEqually
        row.spacing = Self.gap
        row.translatesAutoresizingMaskIntoConstraints = false
        addSubview(row)
        NSLayoutConstraint.activate([
            row.topAnchor.constraint(equalTo: topAnchor, constant: Self.gap),
            row.bottomAnchor.constraint(equalTo: safeAreaLayoutGuide.bottomAnchor, constant: -Self.gap),
            row.leadingAnchor.constraint(equalTo: safeAreaLayoutGuide.leadingAnchor, constant: Self.gap),
            row.trailingAnchor.constraint(equalTo: safeAreaLayoutGuide.trailingAnchor, constant: -Self.gap),
        ])
    }

    required init?(coder: NSCoder) { nil }

    private func button(for key: KeyBar.Key) -> UIButton {
        let button = UIButton(type: .system)
        button.setTitle(key.label, for: .normal)
        button.setTitleColor(.label, for: .normal)
        button.titleLabel?.font = .systemFont(ofSize: 15, weight: .medium)
        button.titleLabel?.adjustsFontSizeToFitWidth = true
        button.backgroundColor = .systemGray4
        button.layer.cornerRadius = 6
        button.accessibilityLabel = key.accessibilityLabel
        button.addAction(UIAction { [weak self] _ in self?.press(key) }, for: .touchDown)
        NSLayoutConstraint.activate([
            button.heightAnchor.constraint(equalToConstant: KeyBar.minSide),
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: KeyBar.minSide),
        ])
        return button
    }

    private func press(_ key: KeyBar.Key) {
        haptic.impactOccurred()
        terminal?.send(key.bytes)
    }
}
