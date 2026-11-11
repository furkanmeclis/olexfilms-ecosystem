import CallKit
import Foundation

/// Call Directory extension of the caller ID (TEC-1208). Copied into the
/// extension target by `plugins/with-caller-id`; it is not part of the app.
///
/// Files in the App Group folder `callerid/`:
///   next.tsv     the list the app wants (written by the app)
///   applied.tsv  the list iOS holds (written here after a finished request)
///   working.tsv  this request's private copy of next.tsv
/// Every line is `digits<TAB>label`; the digits are the E.164 number without
/// the plus sign, ascending and unique.
///
/// An incremental request removes and adds only the difference between
/// applied.tsv and next.tsv. The files are read as streams, never as a whole,
/// because an extension has little memory.
final class CallDirectoryHandler: CXCallDirectoryProvider, CXCallDirectoryExtensionContextDelegate {
  override func beginRequest(with context: CXCallDirectoryExtensionContext) {
    context.delegate = self
    guard let dir = Self.directory() else {
      context.completeRequest()
      return
    }
    let fm = FileManager.default
    let next = dir.appendingPathComponent("next.tsv")
    let applied = dir.appendingPathComponent("applied.tsv")
    let working = dir.appendingPathComponent("working.tsv")

    // The app may replace next.tsv while this request runs: work on a copy.
    try? fm.removeItem(at: working)
    if fm.fileExists(atPath: next.path) {
      try? fm.copyItem(at: next, to: working)
    } else {
      fm.createFile(atPath: working.path, contents: Data())
    }

    if context.isIncremental && fm.fileExists(atPath: applied.path) {
      // Removals first, then the additions in ascending order.
      Self.walk(old: applied, new: working) { number, old, new in
        if let old = old, old != new {
          context.removeIdentificationEntry(withPhoneNumber: number)
        }
      }
      Self.walk(old: applied, new: working) { number, old, new in
        if let new = new, old != new {
          context.addIdentificationEntry(withNextSequentialPhoneNumber: number, label: new)
        }
      }
    } else {
      // First load, or iOS asked for everything again.
      if context.isIncremental {
        context.removeAllIdentificationEntries()
      }
      Self.walk(old: nil, new: working) { number, _, new in
        if let new = new {
          context.addIdentificationEntry(withNextSequentialPhoneNumber: number, label: new)
        }
      }
    }

    // applied.tsv describes what iOS holds once this request is accepted;
    // requestFailed removes it, so the next request loads everything.
    try? fm.removeItem(at: applied)
    try? fm.moveItem(at: working, to: applied)
    context.completeRequest()
  }

  func requestFailed(for extensionContext: CXCallDirectoryExtensionContext, withError error: Error) {
    guard let dir = Self.directory() else { return }
    try? FileManager.default.removeItem(at: dir.appendingPathComponent("applied.tsv"))
  }

  private static func directory() -> URL? {
    guard
      let group = Bundle.main.object(forInfoDictionaryKey: "OlexCallerIdAppGroup") as? String, !group.isEmpty,
      let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group)
    else {
      return nil
    }
    let dir = container.appendingPathComponent("callerid", isDirectory: true)
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
  }

  /// Walks two sorted lists together and reports every number with its old
  /// and its new label (nil: not in that list).
  private static func walk(old: URL?, new: URL, _ visit: (CXCallDirectoryPhoneNumber, String?, String?) -> Void) {
    let oldLines = old.flatMap { LineReader(url: $0) }
    let newLines = LineReader(url: new)
    var a = oldLines?.next()
    var b = newLines?.next()
    while a != nil || b != nil {
      autoreleasepool {
        if let x = a, b == nil || x.number < b!.number {
          visit(x.number, x.label, nil)
          a = oldLines?.next()
        } else if let y = b, a == nil || y.number < a!.number {
          visit(y.number, nil, y.label)
          b = newLines?.next()
        } else if let x = a, let y = b {
          visit(x.number, x.label, y.label)
          a = oldLines?.next()
          b = newLines?.next()
        }
      }
    }
  }
}

/// Reads `digits<TAB>label` lines from a file in 64 KB blocks. Lines that
/// are malformed or not above the previous number are skipped, so one bad
/// line cannot fail the whole request.
private final class LineReader {
  private let handle: FileHandle
  private var buffer = [UInt8]()
  private var position = 0
  private var ended = false
  private var last: CXCallDirectoryPhoneNumber = 0

  init?(url: URL) {
    guard let handle = try? FileHandle(forReadingFrom: url) else { return nil }
    self.handle = handle
  }

  deinit {
    try? handle.close()
  }

  func next() -> (number: CXCallDirectoryPhoneNumber, label: String)? {
    while let line = nextLine() {
      guard let tab = line.firstIndex(of: 9) else { continue }
      guard
        let digits = String(bytes: line[line.startIndex..<tab], encoding: .utf8),
        let number = CXCallDirectoryPhoneNumber(digits), number > last,
        let label = String(bytes: line[line.index(after: tab)...], encoding: .utf8), !label.isEmpty
      else {
        continue
      }
      last = number
      return (number, label)
    }
    return nil
  }

  private func nextLine() -> ArraySlice<UInt8>? {
    while true {
      if let newline = buffer[position...].firstIndex(of: 10) {
        let line = buffer[position..<newline]
        position = newline + 1
        return line
      }
      if ended {
        guard position < buffer.count else { return nil }
        let line = buffer[position...]
        position = buffer.count
        return line
      }
      let block = handle.readData(ofLength: 64 * 1024)
      if block.isEmpty {
        ended = true
      } else {
        buffer = Array(buffer[position...]) + [UInt8](block)
        position = 0
      }
    }
  }
}
