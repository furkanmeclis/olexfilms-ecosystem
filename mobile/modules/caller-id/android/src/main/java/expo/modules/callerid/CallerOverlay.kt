package expo.modules.callerid

import android.content.Context
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.media.AudioManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.text.TextUtils
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView

/**
 * The caller card: a small window over the incoming call screen with the
 * customer's name and number. It closes on a tap, when the ringing stops
 * (answered, declined or missed) and at the latest after [MAX_VISIBLE_MS].
 *
 * The end of the ringing is read from the audio mode, which needs no phone
 * permission; a phone that rings silently may not report it, then the time
 * limit closes the card.
 */
internal object CallerOverlay {
  private const val MAX_VISIBLE_MS = 45_000L
  private const val POLL_MS = 1_000L
  private const val BRAND = "Olexfilms"

  private val handler = Handler(Looper.getMainLooper())
  private var view: View? = null
  private var shownAt = 0L
  private var sawRinging = false

  fun show(context: Context, label: String, number: String) {
    val app = context.applicationContext
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O || !Settings.canDrawOverlays(app)) return
    handler.post {
      dismiss(app)
      try {
        val card = build(app, label, number)
        val params = WindowManager.LayoutParams(
          WindowManager.LayoutParams.MATCH_PARENT,
          WindowManager.LayoutParams.WRAP_CONTENT,
          WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
          WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
            WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL or
            WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED,
          PixelFormat.TRANSLUCENT
        )
        params.gravity = Gravity.TOP
        params.y = dp(app, 96)
        windowManager(app).addView(card, params)
        view = card
        shownAt = System.currentTimeMillis()
        sawRinging = false
        handler.postDelayed({ watch(app) }, POLL_MS)
      } catch (_: Throwable) {
        view = null
      }
    }
  }

  private fun watch(context: Context) {
    if (view == null) return
    val audio = context.getSystemService(Context.AUDIO_SERVICE) as? AudioManager
    val ringing = audio?.mode == AudioManager.MODE_RINGTONE
    if (ringing) sawRinging = true
    val expired = System.currentTimeMillis() - shownAt >= MAX_VISIBLE_MS
    if (expired || (sawRinging && !ringing)) {
      dismiss(context)
    } else {
      handler.postDelayed({ watch(context) }, POLL_MS)
    }
  }

  private fun dismiss(context: Context) {
    val current = view ?: return
    view = null
    try {
      windowManager(context).removeView(current)
    } catch (_: Throwable) {
      // Already gone.
    }
  }

  private fun windowManager(context: Context) =
    context.getSystemService(Context.WINDOW_SERVICE) as WindowManager

  private fun dp(context: Context, value: Int): Int =
    TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, value.toFloat(), context.resources.displayMetrics).toInt()

  private fun build(context: Context, label: String, number: String): View {
    val background = GradientDrawable().apply {
      setColor(Color.parseColor("#003D25"))
      cornerRadius = dp(context, 18).toFloat()
    }
    val texts = LinearLayout(context).apply {
      orientation = LinearLayout.VERTICAL
      addView(text(context, BRAND, 12f, "#B7D8C6", false))
      addView(text(context, label, 19f, "#FFFFFF", true).apply { maxLines = 2 })
      addView(text(context, number, 14f, "#DCEFE4", false))
    }
    val close = text(context, "✕", 18f, "#FFFFFF", false).apply {
      setPadding(dp(context, 12), dp(context, 4), dp(context, 4), dp(context, 4))
    }
    val card = LinearLayout(context).apply {
      orientation = LinearLayout.HORIZONTAL
      gravity = Gravity.CENTER_VERTICAL
      this.background = background
      elevation = dp(context, 6).toFloat()
      setPadding(dp(context, 18), dp(context, 14), dp(context, 14), dp(context, 14))
      addView(texts, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
      addView(close)
    }
    return FrameLayout(context).apply {
      setPadding(dp(context, 12), 0, dp(context, 12), 0)
      addView(card)
      setOnClickListener { dismiss(context) }
    }
  }

  private fun text(context: Context, value: String, size: Float, color: String, bold: Boolean) =
    TextView(context).apply {
      text = value
      setTextSize(TypedValue.COMPLEX_UNIT_SP, size)
      setTextColor(Color.parseColor(color))
      if (bold) setTypeface(typeface, Typeface.BOLD)
      maxLines = 1
      ellipsize = TextUtils.TruncateAt.END
    }
}
