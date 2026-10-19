import ExpoModulesCore

internal final class PlateImageException: Exception {
  override var reason: String {
    "The photo could not be read for text recognition"
  }
}

/// JS name `PlateOcr`. On iOS the recognizer is part of the OS (Vision), so
/// the status is always `ready` and prepare/release have nothing to do.
public class PlateOcrModule: Module {
  public func definition() -> ModuleDefinition {
    Name("PlateOcr")

    Events("onPrepareProgress")

    AsyncFunction("getStatus") { () -> String in
      "ready"
    }

    AsyncFunction("prepare") { () -> String in
      "ready"
    }

    AsyncFunction("release") { () -> Bool in
      true
    }

    AsyncFunction("recognize") { (uri: String) throws -> [String: Any] in
      do {
        return try PlateTextRecognizer.recognize(uri: uri)
      } catch PlateTextRecognizer.Failure.unreadableImage {
        throw PlateImageException()
      }
    }
  }
}
