import MatchbloxKit
import SwiftUI

/// The Mine editor, a sheet: Mine's keys in their order, then the catalog in groups, each a
/// strict 6-column grid of equal cells. A tap adds a key to the end of Mine or takes it off;
/// `◂` stays. A drag on Mine's grid moves a key.
struct BeltEditor: View {
    static let homeStays = "◂ always stays: it is the way back"
    private static let help = "Tap a key to add it to your belt or take it off. Drag a key on your belt to move it. ◂ always stays."
    private static let refusalTime: TimeInterval = 1.6

    let catalog: KeyCatalog
    let dismiss: () -> Void
    /// The sheet went away: by Done, or by a pull down.
    let closed: (_ done: Bool) -> Void

    @State private var mine = Mine.load(from: .standard)
    @State private var refused = false
    @State private var refusals = 0
    @State private var done = false
    @State private var dropTarget: String?
    /// The grids' width, so a dragged key keeps its size.
    @State private var gridWidth: CGFloat = 0

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Capsule()
                    .fill(Color(uiColor: BeltLook.grip))
                    .frame(width: 36, height: 4)
                    .frame(maxWidth: .infinity)
                    .padding(.bottom, 8)
                    .accessibilityHidden(true)
                Text("Mine")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(Theme.fg)
                    .accessibilityAddTraits(.isHeader)
                Text(refused ? Self.homeStays : Self.help)
                    .font(.system(size: 12))
                    .foregroundStyle(refused ? Theme.fg : Theme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.bottom, 4)
                    .animation(.easeOut(duration: 0.15), value: refused)

                group("On your belt") {
                    ForEach(catalog.keys(.mine, agent: nil, mine: mine.keys), id: \.id) { key in
                        cell(key, on: true)
                            .draggable(key.id) {
                                cell(key, on: true).frame(width: Cells.width(span: key.cells, in: gridWidth), height: Cells.height)
                            }
                            .dropDestination(for: String.self) { ids, _ in
                                move(ids.first, to: key.id)
                            } isTargeted: { over in
                                if over {
                                    dropTarget = key.id
                                } else if dropTarget == key.id {
                                    dropTarget = nil
                                }
                            }
                            .layoutValue(key: CellSpan.self, value: key.cells)
                    }
                }
                ForEach(Array(catalog.groups.enumerated()), id: \.offset) { _, group in
                    self.group(group.name) {
                        ForEach(group.keys, id: \.id) { key in
                            cell(key, on: mine.keys.contains { $0.id == key.id })
                                .layoutValue(key: CellSpan.self, value: key.cells)
                        }
                    }
                }

                Button {
                    done = true
                    dismiss()
                } label: {
                    Text("Done")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundStyle(Theme.bg)
                        .frame(maxWidth: .infinity, minHeight: 40)
                        .background(Theme.accent, in: RoundedRectangle(cornerRadius: 9))
                }
                .buttonStyle(.plain)
                .padding(.top, 12)
            }
            .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { gridWidth = $0 }
            .padding(EdgeInsets(top: 10, leading: 12, bottom: 16, trailing: 12))
        }
        .background(Color(uiColor: BeltLook.sheet))
        .onDisappear { closed(done) }
    }

    private func group<Content: View>(_ name: String, @ViewBuilder cells: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(name.uppercased())
                .font(.system(size: 10, weight: .medium, design: .monospaced))
                .kerning(0.8)
                .foregroundStyle(Theme.accent)
                .accessibilityAddTraits(.isHeader)
            Cells { cells() }
        }
        .padding(.top, 10)
    }

    private func cell(_ key: BeltKey, on: Bool) -> some View {
        let face = KeyFace(key, in: catalog)
        let shape = RoundedRectangle(cornerRadius: 6)
        let edge = dropTarget == key.id ? Theme.fg : on ? Theme.accent : Theme.hair
        return Button {
            tap(key)
        } label: {
            Text(face.label)
                .font(.system(size: 12, weight: .medium, design: .monospaced))
                .foregroundStyle(on ? Theme.fg : Theme.muted)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(Theme.wash, in: shape)
                .overlay(shape.strokeBorder(edge, lineWidth: 1))
                .overlay(alignment: .topTrailing) {
                    if let mark = face.mark {
                        Image("mark-\(mark)")
                            .resizable()
                            .scaledToFit()
                            .frame(width: 9, height: 9)
                            .padding(2)
                    }
                }
                .contentShape(shape)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(face.spoken)
        .accessibilityValue(on ? "on your belt" : "")
        .accessibilityAddTraits(on ? .isSelected : [])
    }

    private func tap(_ key: BeltKey) {
        guard mine.toggle(key) else { return refuse() }
        mine.save(to: .standard)
        refused = false
    }

    /// `◂` is the way back: a tap on it says so, and changes nothing.
    private func refuse() {
        refusals += 1
        let refusal = refusals
        refused = true
        UIAccessibility.post(notification: .announcement, argument: Self.homeStays)
        Task {
            try? await Task.sleep(for: .seconds(Self.refusalTime))
            if refusals == refusal { refused = false }
        }
    }

    private func move(_ id: String?, to target: String) -> Bool {
        dropTarget = nil
        guard let from = mine.keys.firstIndex(where: { $0.id == id }),
              let to = mine.keys.firstIndex(where: { $0.id == target }) else { return false }
        guard from != to else { return true }
        withAnimation(.easeOut(duration: 0.2)) { mine.move(from: from, to: to) }
        mine.save(to: .standard)
        return true
    }
}

/// The width of a key in the grid, in cells.
private struct CellSpan: LayoutValueKey {
    static let defaultValue = 1
}

/// A strict grid of equal cells, six to a row, 34 pt tall; a key spans as many cells as it is
/// wide. `CellGrid` places them.
private struct Cells: Layout {
    static let height: CGFloat = 34
    private static let gap: CGFloat = 4

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = CGFloat(CellGrid.rows(spots(subviews)))
        let width = proposal.replacingUnspecifiedDimensions().width
        return CGSize(width: width, height: rows == 0 ? 0 : rows * Self.height + (rows - 1) * Self.gap)
    }

    /// The width of a key `span` cells wide in a grid `width` wide.
    static func width(span: Int, in width: CGFloat) -> CGFloat {
        let columns = CGFloat(CellGrid.columns)
        let cell = (width - gap * (columns - 1)) / columns
        return max(0, cell * CGFloat(span) + gap * CGFloat(span - 1))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let step = Self.width(span: 1, in: bounds.width) + Self.gap
        for (view, spot) in zip(subviews, spots(subviews)) {
            view.place(
                at: CGPoint(
                    x: bounds.minX + CGFloat(spot.column) * step,
                    y: bounds.minY + CGFloat(spot.row) * (Self.height + Self.gap)
                ),
                anchor: .topLeading,
                proposal: ProposedViewSize(width: Self.width(span: spot.span, in: bounds.width), height: Self.height)
            )
        }
    }

    private func spots(_ subviews: Subviews) -> [CellGrid.Spot] {
        CellGrid.place(subviews.map { $0[CellSpan.self] })
    }
}
