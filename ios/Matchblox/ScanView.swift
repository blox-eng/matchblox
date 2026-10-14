import AVFoundation
import SwiftUI
import UIKit
import VisionKit

struct ScanView: View {
    let problem: String?
    let notice: String?
    let onCode: (URL) -> Void
    let onCancel: (() -> Void)?

    private enum Camera {
        case checking, ready, unsupported, denied
    }

    @State private var camera = Camera.checking
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                Text("Pair a host")
                    .font(.title3)
                Spacer()
                if let onCancel {
                    Button("Cancel", action: onCancel)
                        .foregroundStyle(Theme.muted)
                }
            }
            Text("Run matchblox pair on your host. Then point the camera at the code.")
                .foregroundStyle(Theme.muted)
            if let notice {
                Text(notice)
                    .foregroundStyle(Theme.muted)
            }
            if let problem {
                Text(problem)
                    .foregroundStyle(Theme.neg)
            }
            switch camera {
            case .checking:
                Spacer()
            case .ready:
                QRScanner(onCode: onCode)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .overlay(RoundedRectangle(cornerRadius: 12).stroke(Theme.hair))
                Text("You can also scan the code with the Camera app.")
                    .foregroundStyle(Theme.faint)
            case .unsupported:
                Text("This iPhone can't scan codes in matchblox. Open the Camera app and point it at the code. The Camera opens matchblox.")
                Spacer()
            case .denied:
                Text("matchblox can't use the camera. Allow the camera in Settings, or scan the code with the Camera app.")
                Button("Open Settings") {
                    if let settings = URL(string: UIApplication.openSettingsURLString) {
                        UIApplication.shared.open(settings)
                    }
                }
                Spacer()
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(24)
        .task { camera = await Self.cameraState() }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active, camera == .denied else { return }
            Task { camera = await Self.cameraState() }
        }
    }

    @MainActor
    private static func cameraState() async -> Camera {
        guard DataScannerViewController.isSupported else { return .unsupported }
        if AVCaptureDevice.authorizationStatus(for: .video) == .notDetermined {
            _ = await AVCaptureDevice.requestAccess(for: .video)
        }
        return DataScannerViewController.isAvailable ? .ready : .denied
    }
}

/// Reports only `matchblox://pair` URLs and ignores every other code.
private struct QRScanner: UIViewControllerRepresentable {
    let onCode: (URL) -> Void

    func makeCoordinator() -> Coordinator {
        Coordinator(onCode: onCode)
    }

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let scanner = DataScannerViewController(
            recognizedDataTypes: [.barcode(symbologies: [.qr])],
            qualityLevel: .balanced,
            recognizesMultipleItems: false,
            isHighFrameRateTrackingEnabled: false,
            isHighlightingEnabled: true
        )
        scanner.delegate = context.coordinator
        return scanner
    }

    func updateUIViewController(_ scanner: DataScannerViewController, context: Context) {
        context.coordinator.onCode = onCode
        if !scanner.isScanning {
            try? scanner.startScanning()
        }
    }

    static func dismantleUIViewController(_ scanner: DataScannerViewController, coordinator: Coordinator) {
        scanner.stopScanning()
    }

    @MainActor
    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        var onCode: (URL) -> Void

        init(onCode: @escaping (URL) -> Void) {
            self.onCode = onCode
        }

        func dataScanner(
            _ dataScanner: DataScannerViewController,
            didAdd addedItems: [RecognizedItem],
            allItems: [RecognizedItem]
        ) {
            for item in addedItems {
                guard case .barcode(let code) = item,
                      let text = code.payloadStringValue,
                      let url = URL(string: text),
                      url.scheme?.lowercased() == "matchblox",
                      url.host == "pair"
                else { continue }
                dataScanner.stopScanning()
                onCode(url)
                return
            }
        }
    }
}
