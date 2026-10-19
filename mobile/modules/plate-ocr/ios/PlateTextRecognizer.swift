import Foundation
import ImageIO
import UIKit
import Vision

/// Apple Vision text recognition on a photo file. Runs fully on device; the
/// recognizer ships with iOS, so nothing is bundled or downloaded.
enum PlateTextRecognizer {
  enum Failure: Error {
    case unreadableImage
  }

  /// Returns `{ width, height, lines: [{ text, confidence, alternatives, box }] }`.
  /// Boxes are normalized (0–1) with a top-left origin in the upright image.
  static func recognize(uri: String) throws -> [String: Any] {
    let path = URL(string: uri)?.isFileURL == true ? (URL(string: uri)?.path ?? uri) : uri
    guard let image = UIImage(contentsOfFile: path), let cgImage = image.cgImage else {
      throw Failure.unreadableImage
    }

    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    // Plates are not words: language correction turns "34 ABC 123" into prose.
    request.usesLanguageCorrection = false
    request.recognitionLanguages = ["en-US"]

    let handler = VNImageRequestHandler(cgImage: cgImage, orientation: orientation(of: image), options: [:])
    try handler.perform([request])

    var lines: [[String: Any]] = []
    for observation in request.results ?? [] {
      let candidates = observation.topCandidates(3)
      guard let top = candidates.first else { continue }
      let box = observation.boundingBox
      lines.append([
        "text": top.string,
        "confidence": Double(top.confidence),
        "alternatives": candidates.dropFirst().map { $0.string },
        "box": [
          "x": Double(box.minX),
          "y": Double(1 - box.maxY),
          "width": Double(box.width),
          "height": Double(box.height),
        ],
      ])
    }

    return [
      "width": Double(image.size.width * image.scale),
      "height": Double(image.size.height * image.scale),
      "lines": lines,
    ]
  }

  private static func orientation(of image: UIImage) -> CGImagePropertyOrientation {
    switch image.imageOrientation {
    case .up: return .up
    case .down: return .down
    case .left: return .left
    case .right: return .right
    case .upMirrored: return .upMirrored
    case .downMirrored: return .downMirrored
    case .leftMirrored: return .leftMirrored
    case .rightMirrored: return .rightMirrored
    @unknown default: return .up
    }
  }
}
