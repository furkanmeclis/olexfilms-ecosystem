package expo.modules.callerid

import android.content.Context
import android.os.Build
import android.telecom.Call
import android.telecom.CallScreeningService
import android.telephony.PhoneNumberUtils
import android.telephony.TelephonyManager
import java.util.Locale

/**
 * Bound by the system for an incoming call while the app is the "Caller ID
 * & spam app". The call is always let through unchanged; when the number is
 * in the phone book and the in-app switch is on, the caller card is shown.
 *
 * Android does not ask a screening service about numbers saved in the
 * user's contacts, so the card appears for unknown numbers only.
 */
class CallerIdScreeningService : CallScreeningService() {
  override fun onScreenCall(details: Call.Details) {
    // Answer first: the system waits for it before the phone rings.
    respondToCall(details, CallScreeningService.CallResponse.Builder().build())
    try {
      if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return
      if (details.callDirection != Call.Details.DIRECTION_INCOMING) return
      val raw = details.handle?.schemeSpecificPart ?: return
      val store = CallerIdStore.get(applicationContext)
      if (!store.enabled) return
      val number = normalize(applicationContext, raw) ?: return
      val label = store.lookup(number) ?: return
      CallerOverlay.show(applicationContext, label, "+$number")
    } catch (_: Throwable) {
      // A caller card must never disturb a call.
    }
  }

  companion object {
    /** Digits of the E.164 form (no plus sign), or null when the number cannot be read. */
    internal fun normalize(context: Context, raw: String): String? {
      val trimmed = raw.trim()
      val e164 = PhoneNumberUtils.formatNumberToE164(trimmed, region(context))
      val source = e164 ?: if (trimmed.startsWith("+")) trimmed else return null
      val digits = source.filter { it in '0'..'9' }
      return digits.ifEmpty { null }
    }

    /** Country used to read a number written without a country code. */
    private fun region(context: Context): String {
      val telephony = context.getSystemService(Context.TELEPHONY_SERVICE) as? TelephonyManager
      val iso = telephony?.networkCountryIso?.takeIf { it.isNotEmpty() }
        ?: telephony?.simCountryIso?.takeIf { it.isNotEmpty() }
        ?: Locale.getDefault().country
      return iso.uppercase(Locale.ROOT)
    }
  }
}
