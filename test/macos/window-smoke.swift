// Observe the packaged child's own WindowServer entry. A dialog owned by
// another process must not count as a visible application window.
import CoreGraphics
import Darwin
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
let gameCheck = app.arguments == ["check-window"]
let channel = Pipe()
if logWindow { app.standardInput = channel }
do {
    try app.run()
} catch {
    print("Child failed to launch: \(error)")
    exit(1)
}
let pid = app.processIdentifier
print("Foreground desktop; checking for a visible window owned by PID \(pid)")
var observed = false
for _ in 0..<100 {
    guard app.isRunning else { break }
    if let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements],
                                                kCGNullWindowID) as? [[String: Any]] {
        observed = windows.contains { w in
            guard w[kCGWindowOwnerPID as String] as? Int32 == pid,
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

func waitForExit(seconds: Int) -> Bool {
    for _ in 0..<(seconds * 10) {
        if !app.isRunning { return true }
        Thread.sleep(forTimeInterval: 0.1)
    }
    return !app.isRunning
}

if observed {
    print("Observed a visible application window owned by PID \(pid)")
} else if app.isRunning {
    print("Timed out after 20 seconds without a visible window owned by PID \(pid)")
} else {
    print("Child exited before a visible window was observed")
}

// Keep stdin open for the entire observation, then let the log viewer close on EOF.
if logWindow { channel.fileHandleForWriting.closeFile() }
var requestedTermination = false
if logWindow || gameCheck { _ = waitForExit(seconds: gameCheck ? 8 : 3) }
if app.isRunning {
    requestedTermination = true
    app.terminate()
}
guard waitForExit(seconds: 2) else {
    print("Child cleanup timed out; sending SIGKILL to PID \(pid)")
    kill(pid, SIGKILL)
    exit(1)
}
let signalled = app.terminationReason == .uncaughtSignal
let status = app.terminationStatus
print("Child exited with \(signalled ? "signal" : "status") \(status)")
let cleanExit = !signalled && status == 0
let expectedSetupClose = !logWindow && !gameCheck && requestedTermination && signalled && status == SIGTERM
exit(observed && (cleanExit || expectedSetupClose) ? 0 : 1)
