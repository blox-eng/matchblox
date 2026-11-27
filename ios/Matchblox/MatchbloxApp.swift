import MatchbloxKit
import SwiftUI
import UserNotifications

/// Asks iOS for the push tokens at launch, names the buzz category and ends stray activities.
/// Nothing here connects to a host.
final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        let buzz = UNNotificationCategory(
            identifier: BuzzPayload.category,
            actions: [],
            intentIdentifiers: [],
            hiddenPreviewsBodyPlaceholder: BuzzPayload.hiddenBody,
            options: []
        )
        UNUserNotificationCenter.current().setNotificationCategories([buzz])
        MainActor.assumeIsolated {
            PushTokens.shared.begin()
            IslandSettings.endStrayActivities()
        }
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        let token = hexToken(deviceToken)
        MainActor.assumeIsolated {
            PushTokens.shared.device = token
            IslandSettings.keptDeviceToken = token
        }
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        islandLog.error("no device token: \(String(describing: error), privacy: .public)")
    }

    /// A tapped buzz opens its session through the link handling of a tapped island:
    /// `userInfo["open"]`, which the buzz extension sets, else the buzz's host and number.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let userInfo = response.notification.request.content.userInfo
        let url = (userInfo["open"] as? String).flatMap(URL.init(string:)).flatMap(OpenLink.init(url:))?.url
            ?? BuzzPayload.event(userInfo).flatMap { OpenLink(host: $0.host, session: $0.n) }?.url
        if let url {
            Task { @MainActor in UIApplication.shared.open(url) }
        }
        completionHandler()
    }

    /// A buzz that comes while the app is open shows as a banner too.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .list, .sound])
    }
}

/// A tap on the island for a session of the open console's host. `tap` counts the taps, so the
/// same session tapped again jumps again.
struct SessionJump: Equatable {
    let session: Int
    let tap: Int
}

@main
struct MatchbloxApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    private let store = HostStore.standard
    /// A tapped link opens a new console even when one is open.
    @State private var linkOpens = 0
    @State private var route: Route
    @State private var jump: SessionJump?
    @State private var scanProblem: String?
    @State private var scanNotice: String?
    /// Home, or Live Activity from it, drawn over the live console: `‹ host` finds it as it was.
    @State private var over: Over?

    private enum Over { case home, island }

    init() {
        #if DEBUG
        // The UI test's empty store.
        if ProcessInfo.processInfo.arguments.contains("-resetHosts") {
            try? FileManager.default.removeItem(at: HostStore.standard.file)
        }
        // The UI test's head as it came: its marks and its colour.
        if ProcessInfo.processInfo.arguments.contains("-resetHead") {
            UserDefaults.standard.removeObject(forKey: HeadMarks.storeKey)
            UserDefaults.standard.removeObject(forKey: HeadMarks.buildNineKey)
            UserDefaults.standard.removeObject(forKey: HeadPigment.storeKey)
        }
        // The UI test's way to the demo host, with no pairing screen in between.
        if ProcessInfo.processInfo.arguments.contains("-openDemoHost") {
            do {
                try HostStore.standard.upsert(.demo)
            } catch {
                fatalError("The demo host did not save: \(error)")
            }
        }
        #endif
        _route = State(initialValue: Opening.first(hosts: HostStore.standard.all()))
    }

    var body: some Scene {
        WindowGroup {
            ZStack {
                Theme.bg.ignoresSafeArea()
                switch route {
                case .scan:
                    ScanView(
                        problem: scanProblem,
                        notice: scanNotice,
                        onCode: open,
                        onDemo: store.all().contains(where: \.isDemo) ? nil : { go(.demoScan) },
                        onCancel: store.all().isEmpty ? nil : { go(Opening.first(hosts: store.all())) }
                    )
                case .demoScan:
                    DemoScanView(
                        store: store,
                        onDone: {
                            withAnimation(UIAccessibility.isReduceMotionEnabled ? nil : .easeOut(duration: 0.3)) {
                                go(.console(.demo, open: nil))
                            }
                        },
                        onBack: { go(Opening.first(hosts: store.all())) }
                    )
                case .pair(let draft):
                    PairingView(
                        draft: draft,
                        store: store,
                        onPaired: { host in
                            // The console slides in from the drawer side.
                            withAnimation(UIAccessibility.isReduceMotionEnabled ? nil : .easeOut(duration: 0.3)) {
                                go(.console(host, open: nil))
                            }
                        },
                        onScanAgain: { go(.scan) }
                    )
                    .id(draft.payload.key)
                case .console(let host, let session):
                    ConsoleView(
                        host: host,
                        open: session,
                        jump: jump,
                        onFailure: { failure in
                            var sight = HostSight()
                            sight.markFailed(host.fingerprint)
                            go(.reach(host, failure))
                        },
                        onHosts: Opening.consoleOffersHosts(hostCount: store.all().count) ? { openHome() } : nil,
                        onScanAgain: { go(.scan) }
                    )
                    .id("\(host.fingerprint)#\(linkOpens)")
                    .transition(.move(edge: .trailing))
                    .zIndex(1)
                    .accessibilityHidden(over != nil)
                    if let over {
                        layer(over, behind: host)
                            .zIndex(2)
                    }
                case .hosts:
                    home(behind: nil)
                case .reach(let host, let failure):
                    ReachView(
                        host: host,
                        failure: failure,
                        store: store,
                        onRetry: { go(.console($0, open: nil)) },
                        onScanAgain: { go(.scan) }
                    )
                case .island:
                    IslandView(store: store, onBack: { go(.hosts) })
                }
            }
            .foregroundStyle(Theme.fg)
            .fontDesign(.monospaced)
            .tint(Theme.accent)
            .preferredColorScheme(.dark)
            .onOpenURL(perform: open)
        }
    }

    /// Home or Live Activity over the console `live`, opaque so the console takes no tap.
    @ViewBuilder
    private func layer(_ shown: Over, behind live: Host) -> some View {
        Group {
            switch shown {
            case .home: home(behind: live)
            case .island: IslandView(store: store, onBack: { over = .home })
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Theme.bg.ignoresSafeArea())
        .contentShape(Rectangle())
    }

    /// Home over the console `live`, or on its own when `live` is nil.
    private func home(behind live: Host?) -> some View {
        HostsView(
            store: store,
            backHost: live,
            onOpen: { host in
                if host.fingerprint == live?.fingerprint {
                    over = nil
                } else {
                    go(.console(host, open: nil))
                }
            },
            onBack: { over = nil },
            onPair: { go(.scan) },
            onIsland: { if live == nil { go(.island) } else { over = .island } },
            onDemo: { go(.demoScan) },
            onHostRemoved: { host in
                if HomeRemove.clearsBack(removedFingerprint: host.fingerprint, backFingerprint: live?.fingerprint) {
                    go(.hosts)
                }
            }
        )
    }

    /// The `BOX` stamp: home covers the console, which stays connected. The keyboard goes down
    /// so it does not sit over home.
    private func openHome() {
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
        over = .home
    }

    private func go(_ next: Route) {
        over = nil
        scanProblem = nil
        scanNotice = nil
        route = next
    }

    /// A tapped session (`matchblox://open`) opens that host's console after Face ID. The open
    /// console of that host stays, and jumps to the session the link names. Any other console
    /// gives way to it and to a new pairing code only.
    private func open(_ url: URL) {
        if url.host == "open" {
            if let link = OpenLink(url: url) {
                if Opening.keepsConsole(for: link, current: route) {
                    if let session = Opening.jump(for: link, current: route) {
                        jump = SessionJump(session: session, tap: (jump?.tap ?? 0) + 1)
                    }
                    return
                }
                linkOpens += 1
                go(Opening.route(for: link, hosts: store.all()))
            } else if case .console = route {
                return
            } else {
                go(Opening.first(hosts: store.all()))
            }
            return
        }
        if case .console = route, url.host?.lowercased() != "pair" { return }
        do {
            let payload = try PairPayload(url: url)
            go(.pair(PairDraft(payload: payload, phoneName: Pairing.cleanName(UIDevice.current.name))))
        } catch {
            scanProblem = Pairing.message(for: error)
            route = .scan
        }
    }
}
