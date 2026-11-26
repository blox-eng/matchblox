/// The way back into Pocket: the console the builder left stays, and so does its screen.
extension Opening {
    /// A tapped link for the host whose console is open keeps that console: a new console would
    /// ask Face ID and draw the screen again.
    public static func keepsConsole(for link: OpenLink, current: Route) -> Bool {
        guard case .console(let host, _) = current else { return false }
        return host8(host.fingerprint) == link.host
    }

    /// The session the kept console jumps to: the one the island showed (it waits, or it burns).
    /// A link with no session leaves the console where it is.
    public static func jump(for link: OpenLink, current: Route) -> Int? {
        keepsConsole(for: link, current: current) ? link.session : nil
    }

    /// The cover shows on the first connect, for a `page` (an ended console or a key that needs
    /// a new scan: their way on is in the cover), and while the app is away from the screen, so
    /// the app switcher's snapshot holds no console. A console that was live keeps its last
    /// screen while it connects again, and under a failure Try again fixes: the banner says it.
    public static func coverShows(live: Bool, page: Bool, wasLive: Bool, away: Bool) -> Bool {
        if away { return true }
        if live { return false }
        return page || !wasLive
    }
}
