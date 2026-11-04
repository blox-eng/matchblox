import CoreImage.CIFilterBuiltins
import MatchbloxKit
import SwiftUI
import UIKit

/// A code that scans itself: the scan view's look with a still picture of a code in the viewfinder.
/// It opens no camera and asks for nothing. It saves the demo host and opens its console.
struct DemoScanView: View {
    let store: HostStore
    let onDone: () -> Void
    let onBack: () -> Void

    private enum Stage: Equatable {
        case scanning, found, failed
    }

    @State private var stage = Stage.scanning
    @State private var locked = false
    @State private var code: CGImage?
    @State private var run: Task<Void, Never>?
    @State private var finished = false
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    /// The scan: the code sits, the outline locks on it, the confirmation shows. Under 3 seconds in all.
    private static let look = 0.7
    private static let confirmed = 0.8

    var body: some View {
        ZStack {
            GeometryReader { geo in
                let box = PairStage.boxFrame(in: geo.size)
                let window = MatchBox.window(in: box.size).offsetBy(dx: box.minX, dy: box.minY)
                let qr = Self.codeFrame(in: window)
                MatchBox(pose: .outline)
                    .frame(width: box.width, height: box.height)
                    .modifier(LockMove(box: box, code: locked ? qr.insetBy(dx: -qr.width * 0.12, dy: -qr.width * 0.12) : nil))
                    .position(x: box.midX, y: box.midY)
                if let code {
                    Image(decorative: code, scale: 1)
                        .resizable()
                        .interpolation(.none)
                        .frame(width: qr.width, height: qr.height)
                        .position(x: qr.midX, y: qr.midY)
                        .accessibilityElement()
                        .accessibilityLabel(stage == .scanning ? "Demo code, scanning" : "Demo code, found")
                    Text("demo")
                        .font(.caption)
                        .dynamicTypeSize(...DynamicTypeSize.large)
                        .foregroundStyle(Theme.muted)
                        .position(x: qr.midX, y: qr.maxY + window.width * 0.1)
                        .accessibilityHidden(true)
                }
            }
            .ignoresSafeArea()
            .allowsHitTesting(false)
            VStack(alignment: .leading, spacing: 16) {
                HStack {
                    Text("Try a demo")
                        .font(.title3)
                    Spacer()
                    Button("Cancel", action: cancel)
                        .frame(minHeight: 44)
                        .foregroundStyle(Theme.muted)
                }
                .modifier(OverCamera(on: false))
                ViewThatFits(in: .vertical) {
                    bottom
                    ScrollView { bottom }
                        .scrollBounceBehavior(.basedOnSize)
                }
                .frame(maxHeight: .infinity, alignment: .bottom)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(24)
        }
        // A tap anywhere skips the scan. The failure's own buttons keep their taps.
        .contentShape(Rectangle())
        .onTapGesture { if stage != .failed { skip() } }
        .onAppear { Haptics.prepare() }
        .task {
            code = Self.makeCode()
            play()
        }
        .onDisappear { run?.cancel() }
        // The sleeps of a scan that went to the background are not worth waiting for.
        .onChange(of: scenePhase) { _, phase in
            if phase == .active, stage != .failed { skip() }
        }
    }

    private var bottom: some View {
        VStack(alignment: .leading, spacing: 12) {
            switch stage {
            case .scanning:
                Text("Scanning the demo code. Tap to skip.")
                    .foregroundStyle(Theme.muted)
            case .found:
                Text("Found the demo host. Opening its console.")
                    .foregroundStyle(Theme.muted)
            case .failed:
                Text("This device could not save the demo. Free some space on this device, then try again.")
                    .foregroundStyle(Theme.neg)
                Button(action: play) {
                    Text("Try again")
                        .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                        .contentShape(Rectangle())
                }
                .foregroundStyle(Theme.accent)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .modifier(OverCamera(on: false))
    }

    /// The code is smaller than the window and sits a little above the middle. The caption `demo` goes under it.
    private static func codeFrame(in window: CGRect) -> CGRect {
        let side = window.width * 0.6
        return CGRect(x: window.midX - side / 2, y: window.midY - side / 2 - window.width * 0.06, width: side, height: side)
    }

    private func play() {
        run?.cancel()
        stage = .scanning
        locked = false
        run = Task {
            if reduceMotion {
                found(animated: false)
            } else {
                try? await Task.sleep(for: .seconds(Self.look))
                guard !Task.isCancelled else { return }
                found(animated: true)
            }
            try? await Task.sleep(for: .seconds(Self.confirmed))
            guard !Task.isCancelled else { return }
            save()
        }
    }

    private func found(animated: Bool) {
        Haptics.lock()
        withAnimation(animated ? .easeOut(duration: PairStage.lock) : nil) { locked = true }
        stage = .found
        AccessibilityNotification.Announcement("Found the demo host").post()
    }

    private func skip() {
        run?.cancel()
        if stage == .scanning { found(animated: false) }
        save()
    }

    private func cancel() {
        run?.cancel()
        onBack()
    }

    private func save() {
        guard !finished else { return }
        do {
            try store.upsert(.demo)
        } catch {
            stage = .failed
            Haptics.failed()
            return
        }
        finished = true
        Haptics.stood()
        onDone()
    }

    /// A still picture of a code that says `demo`. It opens nothing.
    private static func makeCode() -> CGImage? {
        let generator = CIFilter.qrCodeGenerator()
        generator.message = Data("demo".utf8)
        generator.correctionLevel = "M"
        let tint = CIFilter.falseColor()
        tint.inputImage = generator.outputImage?.transformed(by: CGAffineTransform(scaleX: 12, y: 12))
        tint.color0 = CIColor(color: UIColor(Theme.fg))
        tint.color1 = CIColor(color: UIColor(Theme.bg))
        guard let image = tint.outputImage else { return nil }
        return CIContext().createCGImage(image, from: image.extent)
    }
}
