/// The way back into Pocket: the console the builder left stays, and so does its screen.
extension Opening {
    /// A tapped link for the host whose console is open keeps that console as it is, on any
    /// session: a new console would ask Face ID and draw the screen again.
    public static func keepsConsole(for link: OpenLink, current: Route) -> Bool {
        guard case .console(let host, _) = current else { return false }
        return host8(host.fingerprint) == link.host
    }

    /// The cover shows on the first connect, on a failure, and while the app is away from the
    /// screen, so the app switcher's snapshot holds no console. A console that was live keeps its
    /// last screen while it connects again; the tmux attach draws over it.
    public static func coverShows(live: Bool, failed: Bool, wasLive: Bool, away: Bool) -> Bool {
        if away { return true }
        if live { return false }
        return failed || !wasLive
    }
}
