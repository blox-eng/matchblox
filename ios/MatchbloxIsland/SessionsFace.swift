import MatchbloxKit
import SwiftUI
import WidgetKit

// The Sessions face (design 0006 §6.6): one match for the top session. The wood is its
// progress; it burns while the agent works, smokes while it waits, is cold when done or
// out; a closed box when nothing is on.

struct SessionsLock: View {
    let m: IslandModel
    init(_ m: IslandModel) { self.m = m }

    var body: some View {
        let who = IslandLook.who(m.state.top, names: m.names)
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                HostLine(label: m.label)
                Spacer(minLength: 10)
                Counts(burn: m.state.burn, wait: m.state.wait)
            }
            if m.state.need > 0 {
                // What needs you first: one row each, then the rest as a count.
                let r = IslandLook.rows(m.state, names: m.names)
                ForEach(Array(r.rows.enumerated()), id: \.offset) { _, row in
                    WhoLine(name: row.name, detail: row.ask)
                }
                if let more = r.more {
                    Text(more).font(IslandFont.mono).foregroundStyle(Tokens.muted)
                }
            } else {
                MatchView(match: IslandLook.match(m.state.top), thickness: 9)
                    .frame(height: 52)
                HStack(alignment: .firstTextBaseline) {
                    WhoLine(name: who.name, detail: who.detail)
                    Spacer(minLength: 10)
                    Text(IslandLook.word(m.state.top))
                        .font(IslandFont.mono)
                        .foregroundStyle(wordColor(IslandLook.state(m.state.top)))
                        .lineLimit(1)
                }
            }
            if let limits = IslandLook.limitsLine(m.state) {
                Text(limits).font(IslandFont.monoSmall).foregroundStyle(Tokens.muted).lineLimit(1)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 14)
    }
}

struct SessionsExpanded: View {
    let m: IslandModel
    init(_ m: IslandModel) { self.m = m }

    var body: some View {
        let who = IslandLook.expandedWho(m.state.top, names: m.names)
        VStack(spacing: 6) {
            MatchView(match: IslandLook.match(m.state.top), thickness: 7)
                .frame(height: 46)
            HStack {
                WhoLine(name: who.name, detail: who.detail, size: 14)
                Spacer(minLength: 10)
                if let link = m.link { IslandButton(title: IslandLook.button(m.state.top), link: link) }
            }
        }
        .padding(.horizontal, 6)
    }
}

struct SessionsCompactLeading: View {
    let m: IslandModel
    init(_ m: IslandModel) { self.m = m }

    var body: some View {
        if IslandLook.state(m.state.top) == .quiet {
            Text("▰").font(IslandFont.mono).foregroundStyle(Tokens.accent)
                .frame(width: 46, height: 24)
        } else {
            MatchView(match: IslandLook.match(m.state.top), thickness: 3.6)
                .frame(width: 46, height: 24)
        }
    }
}

struct SessionsCompactTrailing: View {
    let m: IslandModel
    init(_ m: IslandModel) { self.m = m }

    /// What needs you: its name, or `╿ N`. Else the one limit that matters while work runs;
    /// else the top's own word (out, waits, done); nothing when all is quiet.
    var body: some View {
        let attention = IslandLook.attention(m.state, names: m.names)
        let state = IslandLook.state(m.state.top)
        let limit = state == .burns || state == .quiet ? IslandLook.usageCompact(m.state) : nil
        Group {
            if let attention {
                Text(attention).foregroundStyle(Tokens.accent)
            } else if let limit {
                Text(limit.text).foregroundStyle(barColorLevel(limit.pct).text)
            } else if state == .quiet {
                EmptyView()
            } else {
                Text(IslandLook.compact(m.state.top, names: m.names))
                    .foregroundStyle(IslandLook.state(m.state.top) == .waits ? Tokens.accent : Tokens.fg)
            }
        }
        .font(IslandFont.compact)
        .lineLimit(1)
        .frame(maxWidth: 80, alignment: .trailing)
    }
}

/// The front of the match; a head ready to strike when nothing is on.
struct SessionsMinimal: View {
    let m: IslandModel
    init(_ m: IslandModel) { self.m = m }

    var body: some View {
        let match = IslandLook.match(m.state.top)
        if match.state == .quiet {
            Ellipse().fill(Tokens.matchHead).frame(width: 10, height: 7.2)
        } else {
            MatchView(match: match, thickness: 3.4, minimal: true)
                .frame(width: 30, height: 30)
        }
    }
}

/// `api · waits for you`.
struct WhoLine: View {
    let name: String
    let detail: String?
    var size: CGFloat = 15

    var body: some View {
        (Text(name).font(.system(size: size, weight: .semibold)).foregroundColor(Tokens.fg)
            + Text(detail.map { " · \($0)" } ?? "").font(.system(size: size)).foregroundColor(Tokens.muted))
            .lineLimit(1)
            .truncationMode(.tail)
    }
}

/// `Answer` or `Open`: a link to the top session, in the accent.
struct IslandButton: View {
    let title: String
    let link: URL

    var body: some View {
        Link(destination: link) {
            Text(title)
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(Tokens.bg)
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .background(Tokens.accent, in: RoundedRectangle(cornerRadius: 14))
        }
    }
}

func wordColor(_ s: TopState) -> Color {
    switch s {
    case .burns: Tokens.flameAmber
    case .waits: Tokens.accent
    case .out: Tokens.warn
    case .done, .quiet: Tokens.muted
    }
}
