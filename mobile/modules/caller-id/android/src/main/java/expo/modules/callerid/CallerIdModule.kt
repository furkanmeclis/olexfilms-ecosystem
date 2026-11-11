package expo.modules.callerid

import android.app.role.RoleManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import expo.modules.kotlin.exception.Exceptions
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * JS name `OlexCallerId` (TEC-1208). Keeps the phone book the screening
 * service reads, the in-app switch, and opens the two system screens the
 * user has to pass: the "Caller ID & spam app" role and "Display over other
 * apps".
 */
class CallerIdModule : Module() {
  private val context: Context
    get() = appContext.reactContext ?: throw Exceptions.ReactContextLost()

  private val supported: Boolean
    get() = Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q

  private fun roleHeld(): Boolean {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return false
    val roles = context.getSystemService(RoleManager::class.java) ?: return false
    return roles.isRoleAvailable(RoleManager.ROLE_CALL_SCREENING) && roles.isRoleHeld(RoleManager.ROLE_CALL_SCREENING)
  }

  private fun open(intent: Intent): Boolean =
    try {
      context.startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
      true
    } catch (_: Throwable) {
      false
    }

  override fun definition() = ModuleDefinition {
    Name("OlexCallerId")

    AsyncFunction("getStatus") {
      mapOf(
        "supported" to supported,
        "role" to roleHeld(),
        "overlay" to Settings.canDrawOverlays(context)
      )
    }

    /**
     * Shows the system dialog that makes the app the caller ID app. Answers
     * whether the dialog was started; the result is read with `getStatus`
     * when the app is active again. The system stops showing the dialog
     * after repeated refusals: `openDefaultApps` is the way in then.
     */
    AsyncFunction("requestRole") {
      val activity = appContext.currentActivity
      if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q || activity == null) return@AsyncFunction false
      val roles = activity.getSystemService(RoleManager::class.java)
      if (roles == null || !roles.isRoleAvailable(RoleManager.ROLE_CALL_SCREENING)) return@AsyncFunction false
      if (roles.isRoleHeld(RoleManager.ROLE_CALL_SCREENING)) return@AsyncFunction true
      val intent = roles.createRequestRoleIntent(RoleManager.ROLE_CALL_SCREENING)
      activity.runOnUiThread {
        try {
          activity.startActivityForResult(intent, ROLE_REQUEST_CODE)
        } catch (_: Throwable) {
          // No role dialog on this device.
        }
      }
      true
    }

    /** Settings > Apps > Default apps. */
    AsyncFunction("openDefaultApps") {
      open(Intent(Settings.ACTION_MANAGE_DEFAULT_APPS_SETTINGS))
    }

    /** "Display over other apps" for this app. */
    AsyncFunction("openOverlaySettings") {
      open(Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.parse("package:" + context.packageName))) ||
        open(Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION))
    }

    AsyncFunction("setEnabled") { enabled: Boolean ->
      CallerIdStore.get(context).enabled = enabled
      enabled
    }

    AsyncFunction("setEntries") { tsv: String ->
      mapOf("supported" to supported, "count" to CallerIdStore.get(context).replaceAll(tsv))
    }

    AsyncFunction("clear") {
      CallerIdStore.get(context).clear()
      mapOf("supported" to supported)
    }
  }

  companion object {
    private const val ROLE_REQUEST_CODE = 48211
  }
}
