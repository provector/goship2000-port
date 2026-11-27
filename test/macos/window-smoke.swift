// Run against the asset-free game log window and the disposable packaged SETUP. Observe its own WindowServer
// entry so a blocked process or an unrelated dialog cannot count as a GUI pass.
import CoreGraphics
import Foundation

guard let session = CGSessionCopyCurrentDictionary() as? [String: Any],
      session[kCGSessionOnConsoleKey as String] as? Bool == true else {
    print("::warning::No foreground WindowServer session; graphical window check untested")
    exit(0)
}
let app = Process()
app.executableURL = URL(fileURLWithPath: CommandLine.arguments[1])
app.arguments = Array(CommandLine.arguments.dropFirst(2))
let logWindow = app.arguments == ["__log-console"]
let channel = Pipe()
if logWindow { app.standardInput = channel }
try app.run()
if logWindow { channel.fileHandleForWriting.write(Data("Native macOS graphics smoke check\n".utf8)) }
var observed = false
for _ in 0..<100 {
    guard app.isRunning else { break }
    if let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements],
                                                kCGNullWindowID) as? [[String: Any]] {
        observed = windows.contains { w in
            guard w[kCGWindowOwnerPID as String] as? Int32 == app.processIdentifier,
                  w[kCGWindowLayer as String] as? Int == 0,
                  let bounds = w[kCGWindowBounds as String] as? [String: Any],
                  let width = bounds["Width"] as? Double,
                  let height = bounds["Height"] as? Double else { return false }
            return width >= 640 && height >= 400
        }
    }
    if observed { break }
    Thread.sleep(forTimeInterval: 0.2)
}
if logWindow {
    channel.fileHandleForWriting.closeFile()
    app.waitUntilExit()
    guard app.terminationStatus == 0 else { print("Game log window failed"); exit(1) }
} else if app.isRunning {
    app.terminate()
    app.waitUntilExit()
}
guard observed else { print("Packaged application did not create its visible window"); exit(1) }
print("Observed a window owned by \(app.executableURL!.lastPathComponent) on the foreground desktop")
