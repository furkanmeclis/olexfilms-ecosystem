package expo.modules.callerid

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

/**
 * The phone book on the device: `number` is the E.164 number without the
 * plus sign, `label` what the card shows. Plain SQLite in the app's private
 * storage; the screening service reads it while the app is not running.
 */
internal class CallerIdStore private constructor(context: Context) :
  SQLiteOpenHelper(context, "olex_caller_id.db", null, 1) {

  private val prefs = context.getSharedPreferences("olex_caller_id", Context.MODE_PRIVATE)

  /** The in-app switch. Off: the service shows nothing, whatever the system role says. */
  var enabled: Boolean
    get() = prefs.getBoolean("enabled", false)
    set(value) {
      prefs.edit().putBoolean("enabled", value).apply()
    }

  override fun onCreate(db: SQLiteDatabase) {
    db.execSQL("CREATE TABLE entries (number TEXT PRIMARY KEY NOT NULL, label TEXT NOT NULL) WITHOUT ROWID")
  }

  override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
    db.execSQL("DROP TABLE IF EXISTS entries")
    onCreate(db)
  }

  /** Replaces the whole list with `digits<TAB>label` lines; returns how many were stored. */
  fun replaceAll(tsv: String): Int {
    val db = writableDatabase
    var count = 0
    db.beginTransaction()
    try {
      db.delete("entries", null, null)
      val insert = db.compileStatement("INSERT OR REPLACE INTO entries (number, label) VALUES (?, ?)")
      for (line in tsv.lineSequence()) {
        val tab = line.indexOf('\t')
        if (tab <= 0 || tab == line.length - 1) continue
        insert.bindString(1, line.substring(0, tab))
        insert.bindString(2, line.substring(tab + 1))
        insert.executeInsert()
        insert.clearBindings()
        count++
      }
      db.setTransactionSuccessful()
    } finally {
      db.endTransaction()
    }
    return count
  }

  fun clear() {
    writableDatabase.delete("entries", null, null)
  }

  fun lookup(number: String): String? {
    readableDatabase.rawQuery("SELECT label FROM entries WHERE number = ?", arrayOf(number)).use { cursor ->
      return if (cursor.moveToFirst()) cursor.getString(0) else null
    }
  }

  companion object {
    @Volatile
    private var instance: CallerIdStore? = null

    fun get(context: Context): CallerIdStore =
      instance ?: synchronized(this) {
        instance ?: CallerIdStore(context.applicationContext).also { instance = it }
      }
  }
}
