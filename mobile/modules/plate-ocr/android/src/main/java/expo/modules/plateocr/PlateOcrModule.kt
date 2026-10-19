package expo.modules.plateocr

import android.content.Context
import android.net.Uri
import com.google.android.gms.common.moduleinstall.InstallStatusListener
import com.google.android.gms.common.moduleinstall.ModuleInstall
import com.google.android.gms.common.moduleinstall.ModuleInstallRequest
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate.InstallState.STATE_CANCELED
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate.InstallState.STATE_COMPLETED
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate.InstallState.STATE_FAILED
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.Text
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.TextRecognizer
import com.google.mlkit.vision.text.latin.TextRecognizerOptions
import expo.modules.kotlin.Promise
import expo.modules.kotlin.exception.Exceptions
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * JS name `PlateOcr`. ML Kit text recognition (Latin) through Google Play
 * services: the model is not in the APK; `prepare` asks Play services to
 * download it (progress via `onPrepareProgress`), `release` lets Play
 * services remove it again.
 */
class PlateOcrModule : Module() {
  private var recognizer: TextRecognizer? = null

  private val context: Context
    get() = appContext.reactContext ?: throw Exceptions.ReactContextLost()

  private fun client(): TextRecognizer =
    recognizer ?: TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS).also { recognizer = it }

  override fun definition() = ModuleDefinition {
    Name("PlateOcr")

    Events("onPrepareProgress")

    AsyncFunction("getStatus") { promise: Promise ->
      ModuleInstall.getClient(context)
        .areModulesAvailable(client())
        .addOnSuccessListener { promise.resolve(if (it.areModulesAvailable()) "ready" else "not_installed") }
        .addOnFailureListener { promise.resolve("unsupported") }
    }

    AsyncFunction("prepare") { promise: Promise ->
      val installer = ModuleInstall.getClient(context)
      // The listener and the request task can both report the end (e.g. a
      // failed install); the promise must settle exactly once.
      var settled = false
      fun settle(block: () -> Unit) {
        if (settled) return
        settled = true
        block()
      }
      val listener = object : InstallStatusListener {
        override fun onInstallStatusUpdated(update: ModuleInstallStatusUpdate) {
          update.progressInfo?.let {
            val total = it.totalBytesToDownload
            if (total > 0) {
              sendEvent("onPrepareProgress", mapOf("progress" to it.bytesDownloaded.toDouble() / total.toDouble()))
            }
          }
          when (update.installState) {
            STATE_COMPLETED -> {
              installer.unregisterListener(this)
              settle { promise.resolve("ready") }
            }
            STATE_FAILED, STATE_CANCELED -> {
              installer.unregisterListener(this)
              settle { promise.reject("ERR_PLATE_OCR_INSTALL", "Text recognition model could not be installed (${update.errorCode})", null) }
            }
            else -> Unit
          }
        }
      }
      val request = ModuleInstallRequest.newBuilder().addApi(client()).setListener(listener).build()
      installer
        .installModules(request)
        .addOnSuccessListener {
          if (it.areModulesAlreadyInstalled()) {
            installer.unregisterListener(listener)
            settle { promise.resolve("ready") }
          }
        }
        .addOnFailureListener { e ->
          installer.unregisterListener(listener)
          settle { promise.reject("ERR_PLATE_OCR_INSTALL", e.message ?: "Text recognition model could not be installed", e) }
        }
    }

    AsyncFunction("release") { promise: Promise ->
      val api = client()
      ModuleInstall.getClient(context)
        .releaseModules(api)
        .addOnCompleteListener {
          api.close()
          recognizer = null
          promise.resolve(it.isSuccessful)
        }
    }

    // `roi` is used on iOS only; here the guide frame is applied in JS when
    // scoring lines, so the whole photo is read.
    AsyncFunction("recognize") { uri: String, _: Map<String, Double>?, promise: Promise ->
      val image = try {
        InputImage.fromFilePath(context, Uri.parse(uri))
      } catch (e: Exception) {
        null
      }
      if (image == null) {
        promise.reject("ERR_PLATE_OCR_IMAGE", "The photo could not be read for text recognition", null)
      } else {
        client()
          .process(image)
          .addOnSuccessListener { promise.resolve(toResult(it, image.width, image.height)) }
          .addOnFailureListener { promise.reject("ERR_PLATE_OCR", it.message ?: "Text recognition failed", it) }
      }
    }

    OnDestroy {
      recognizer?.close()
      recognizer = null
    }
  }

  private fun toResult(text: Text, width: Int, height: Int): Map<String, Any?> {
    val w = width.coerceAtLeast(1).toDouble()
    val h = height.coerceAtLeast(1).toDouble()
    val lines = text.textBlocks.flatMap { block ->
      block.lines.map { line ->
        val box = line.boundingBox
        mapOf(
          "text" to line.text,
          "confidence" to line.confidence.toDouble(),
          "alternatives" to emptyList<String>(),
          "box" to box?.let {
            mapOf(
              "x" to it.left / w,
              "y" to it.top / h,
              "width" to it.width() / w,
              "height" to it.height() / h,
            )
          },
        )
      }
    }
    return mapOf("width" to width, "height" to height, "lines" to lines)
  }
}
