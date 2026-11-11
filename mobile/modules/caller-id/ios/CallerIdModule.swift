import CallKit
import ExpoModulesCore

/// JS name `OlexCallerId` (TEC-1208).
///
/// iOS never runs app code while the phone rings: it looks the number up in
/// a list the Call Directory extension loaded earlier. This module writes the
/// phone book to the App Group (`callerid/next.tsv`, one `digits<TAB>label`
/// line per number, ascending) and asks iOS to reload the extension, which
/// applies the difference (`modules/caller-id/extension`).
///
/// The App Group and the extension only exist in builds made with the
/// `plugins/with-caller-id` config plugin; without it everything answers
/// "unsupported".
public class CallerIdModule: Module {
  private static let groupKey = "OlexCallerIdAppGroup"
  private static let extensionKey = "OlexCallerIdExtensionId"

  private static var extensionId: String? {
    let id = Bundle.main.object(forInfoDictionaryKey: extensionKey) as? String
    return (id?.isEmpty ?? true) ? nil : id
  }

  private static func directory() -> URL? {
    guard
      let group = Bundle.main.object(forInfoDictionaryKey: groupKey) as? String, !group.isEmpty,
      let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group)
    else {
      return nil
    }
    let dir = container.appendingPathComponent("callerid", isDirectory: true)
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
  }

  /// Writes the phone book and reloads the extension. `reloaded` is false
  /// while the user has the extension switched off in Settings (the file
  /// stays, so iOS loads it when the switch is turned on).
  private static func publish(_ tsv: String, promise: Promise) {
    guard let dir = directory(), let id = extensionId else {
      promise.resolve(["supported": false, "reloaded": false] as [String: Any])
      return
    }
    do {
      try tsv.write(to: dir.appendingPathComponent("next.tsv"), atomically: true, encoding: .utf8)
    } catch {
      promise.reject("ERR_CALLER_ID_WRITE", "The caller ID list could not be saved")
      return
    }
    CXCallDirectoryManager.sharedInstance.reloadExtension(withIdentifier: id) { error in
      if let error = error as NSError? {
        promise.resolve(["supported": true, "reloaded": false, "errorCode": error.code] as [String: Any])
      } else {
        promise.resolve(["supported": true, "reloaded": true] as [String: Any])
      }
    }
  }

  public func definition() -> ModuleDefinition {
    Name("OlexCallerId")

    AsyncFunction("getStatus") { (promise: Promise) in
      guard let id = Self.extensionId, Self.directory() != nil else {
        promise.resolve(["supported": false, "extension": "unknown"] as [String: Any])
        return
      }
      CXCallDirectoryManager.sharedInstance.getEnabledStatusForExtension(withIdentifier: id) { status, error in
        var value = "unknown"
        if error == nil {
          switch status {
          case .enabled: value = "enabled"
          case .disabled: value = "disabled"
          default: value = "unknown"
          }
        }
        promise.resolve(["supported": true, "extension": value] as [String: Any])
      }
    }

    /// Opens Settings > Apps > Phone > Call Blocking & Identification.
    AsyncFunction("openSettings") { (promise: Promise) in
      DispatchQueue.main.async {
        CXCallDirectoryManager.sharedInstance.openSettings { error in
          promise.resolve(error == nil)
        }
      }
    }

    AsyncFunction("setEntries") { (tsv: String, promise: Promise) in
      Self.publish(tsv, promise: promise)
    }

    /// Empties the list on the device (feature switched off, sign-out).
    AsyncFunction("clear") { (promise: Promise) in
      Self.publish("", promise: promise)
    }
  }
}
