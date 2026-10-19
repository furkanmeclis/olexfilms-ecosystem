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
  /// `roi` (same coordinates) limits recognition to the guide frame: less
  /// work per frame and no stray text from the rest of the car.
  static func recognize(uri: String, roi: [String: Double]?) throws -> [String: Any] {
    let path = URL(string: uri)?.isFileURL == true ? (URL(string: uri)?.path ?? uri) : uri
    guard let image = UIImage(contentsOfFile: path), let cgImage = image.cgImage else {
      throw Failure.unreadableImage
    }

    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    // Plates are not words: language correction turns "34 ABC 123" into prose.
    request.usesLanguageCorrection = false
    request.recognitionLanguages = ["en-US"]

    // Vision's region is normalized with a bottom-left origin; observation
    // boxes come back relative to that region and are mapped back below.
    var region = CGRect(x: 0, y: 0, width: 1, height: 1)
    if let roi, let x = roi["x"], let y = roi["y"], let w = roi["width"], let h = roi["height"], w > 0, h > 0 {
      let r = CGRect(x: x, y: 1 - y - h, width: w, height: h).intersection(region)
      if !r.isNull, r.width > 0, r.height > 0 { region = r }
    }
    request.regionOfInterest = region

    let handler = VNImageRequestHandler(cgImage: cgImage, orientation: orientation(of: image), options: [:])
    try handler.perform([request])

    var lines: [[String: Any]] = []
    for observation in request.results ?? [] {
      let candidates = observation.topCandidates(3)
      guard let top = candidates.first else { continue }
      let local = observation.boundingBox
      let box = CGRect(
        x: region.minX + local.minX * region.width,
        y: region.minY + local.minY * region.height,
        width: local.width * region.width,
        height: local.height * region.height
      )
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
