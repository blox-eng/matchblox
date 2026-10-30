import MatchbloxKit
import SwiftUI

/// What the tray shows: the matches, the strike in progress, and the session's agent.
@MainActor
@Observable
final class TrayModel {
    var tray: MatchTray
    /// The match that flares, for the strike's first moment.
    private(set) var flaring: BeltLayout?
    var agent: String?
    @ObservationIgnored var onPick: (BeltLayout) -> Void = { _ in }
    @ObservationIgnored var onEdit: () -> Void = {}
    @ObservationIgnored private var strikes = 0

    init(inUse: BeltLayout) {
        tray = MatchTray(inUse: inUse)
    }

    func pick(_ layout: BeltLayout) {
        if tray.pick(layout) != nil {
            strikes += 1
            let strike = strikes
            flaring = layout
            Task { [weak self] in
                try? await Task.sleep(for: .seconds(MatchTray.flare))
                guard let self, self.strikes == strike else { return }
                self.flaring = nil
            }
        }
        onPick(layout)
    }

    /// A fresh box, with the layout in use burning.
    func freshBox(inUse: BeltLayout) {
        strikes += 1
        flaring = nil
        if tray.inUse == inUse {
            tray.close()
        } else {
            tray = MatchTray(inUse: inUse)
        }
    }
}

/// The inside of the matchbox: the four layouts lie on the tray floor as matches, drawn as the
/// island draws its match, with the session's agent and the stamped `EDIT MINE` under them.
struct MatchTrayView: View {
    static let height: CGFloat = 88

    let model: TrayModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .top, spacing: 6) {
                ForEach(BeltLayout.allCases, id: \.self) { layout in
                    TrayMatch(
                        layout: layout,
                        match: model.tray.match(layout),
                        flare: model.flaring == layout,
                        pick: { model.pick(layout) }
                    )
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityLabel("Layouts")
            Spacer(minLength: 0)
            HStack(alignment: .center, spacing: 0) {
                AgentTag(agent: model.agent)
                Spacer(minLength: 8)
                EditStamp(edit: model.onEdit)
            }
        }
        .padding(EdgeInsets(top: 10, leading: 10, bottom: 6, trailing: 10))
        .frame(height: Self.height)
        .background(TrayFloor())
    }
}

/// One layout as a match, with its name under it in faint mono caps.
private struct TrayMatch: View {
    /// The mockup draws each match in an 80 × 30 box with a 5 pt stick.
    private static let box = CGSize(width: 80, height: 30)
    private static let thickness: CGFloat = 5

    let layout: BeltLayout
    let match: MatchTray.Match
    let flare: Bool
    let pick: () -> Void

    var body: some View {
        Button(action: pick) {
            VStack(alignment: .leading, spacing: 0) {
                GeometryReader { geo in
                    drawing
                        .frame(width: Self.box.width, height: Self.box.height)
                        .scaleEffect(min(1, geo.size.width / Self.box.width), anchor: .leading)
                        .frame(maxHeight: .infinity)
                }
                .frame(height: Self.box.height)
                Text(layout.rawValue.uppercased())
                    .font(.system(size: 10.5, weight: .medium, design: .monospaced))
                    .kerning(0.42)
                    .foregroundStyle(match == .burns ? Theme.fg : Theme.faint)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
            }
            .padding(.bottom, 14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(layout.rawValue.capitalized)
        .accessibilityAddTraits(match == .burns ? .isSelected : [])
    }

    @ViewBuilder
    private var drawing: some View {
        switch match {
        case .ready:
            ReadyMatchView(thickness: Self.thickness)
        case .burns:
            if let look = match.look {
                MatchView(match: look, thickness: Self.thickness, flare: flare, clips: false)
            }
        case .out:
            if let look = match.look {
                MatchView(match: look, thickness: Self.thickness, clips: false)
                    .overlay { GoingOut(front: look.front, thickness: Self.thickness) }
            }
        }
    }
}

/// The smoke of a match that just went out: the island's puffs, rising and fading.
private struct GoingOut: View {
    let front: Double
    let thickness: CGFloat
    @State private var gone = false

    var body: some View {
        GeometryReader { geo in
            let g = MatchGeometry(size: geo.size, th: thickness, front: front, minimal: false)
            // The mockup's puffs: three, at twice the island's full strength, on the dark floor.
            SmokeView(x: g.front, y: g.y - g.th * 0.4, th: g.th, puffs: 3, gain: 2 / 0.6)
                .opacity(gone ? 0 : 1)
                .offset(y: gone ? -6 : 0)
        }
        .allowsHitTesting(false)
        .onAppear {
            withAnimation(.easeOut(duration: MatchTray.smokeFade)) { gone = true }
        }
    }
}

/// The session's agent, with its mark.
private struct AgentTag: View {
    private static let markSide: CGFloat = 11

    let agent: String?

    var body: some View {
        if let agent, let name = KeyCatalog.agentNames[agent] {
            HStack(spacing: 4) {
                if let mark = BeltView.catalog.mark(of: agent) {
                    Image("mark-\(mark)")
                        .resizable()
                        .scaledToFit()
                        .frame(width: Self.markSide, height: Self.markSide)
                        .accessibilityHidden(true)
                }
                Text(name)
                    .font(.system(size: 10, weight: .medium, design: .monospaced))
                    .foregroundStyle(Theme.faint)
                    .lineLimit(1)
            }
            .accessibilityElement(children: .combine)
            .accessibilityLabel("Session agent, \(name)")
        }
    }
}

/// `EDIT MINE`, stamped on the floor.
private struct EditStamp: View {
    let edit: () -> Void

    var body: some View {
        Button(action: edit) {
            Text("EDIT MINE")
                .font(.system(size: 10.5, weight: .medium, design: .monospaced))
                .kerning(0.63)
                .foregroundStyle(Color(uiColor: BeltLook.stampInk))
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .overlay(RoundedRectangle(cornerRadius: 3).strokeBorder(Color(uiColor: BeltLook.stampEdge), lineWidth: 1))
                .contentShape(Rectangle().inset(by: -10))
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Edit Mine")
    }
}

/// The tray: its floor (`markTray`) with a faint card grain, its walls (`markLeft`) and the
/// shadow they throw in.
private struct TrayFloor: View {
    private static let radius: CGFloat = 3
    private static let wall: CGFloat = 2

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: Self.radius)
        let light = Color(uiColor: BeltLook.grainLight)
        let dark = Color(uiColor: BeltLook.grainDark)
        Canvas { context, size in
            var x: CGFloat = 0
            while x < size.width {
                context.fill(Path(CGRect(x: x, y: 0, width: 1, height: size.height)), with: .color(light))
                x += 3
            }
            var y: CGFloat = 0
            while y < size.height {
                context.fill(Path(CGRect(x: 0, y: y, width: size.width, height: 1)), with: .color(dark))
                y += 4
            }
        }
        .background(Theme.markTray)
        .overlay(alignment: .top) {
            LinearGradient(colors: [.black.opacity(0.85), .black.opacity(0)], startPoint: .top, endPoint: .bottom)
                .frame(height: 11)
        }
        .overlay(alignment: .bottom) {
            LinearGradient(colors: [.black.opacity(0), .black.opacity(0.6)], startPoint: .top, endPoint: .bottom)
                .frame(height: 5)
        }
        .clipShape(shape)
        .overlay(shape.strokeBorder(Theme.markLeft, lineWidth: Self.wall))
        .accessibilityHidden(true)
    }
}
