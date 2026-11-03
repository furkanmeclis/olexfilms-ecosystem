// `props` native tarafta UserDefaults'a yazılır. JS `null`'ı `NSNull` olur (`JavaScriptValue.getAny`)
// ve `NSNull` içeren sözlük `-[NSUserDefaults setObject:forKey:]` içinde uygulamayı düşürür.
// `undefined` ise atlanır. Saf dosya.
// Sonuç: layout isteğe bağlı alanı doğruluk ya da `?.` ile okur, asla `=== null` ile; alan `undefined` gelir.

/** `null` ve `undefined` alanları (dizide elemanları) derinlemesine atar; layout yokluğu `null` gibi okur. */
export function stripNulls<T>(value: T): T {
  if (Array.isArray(value)) {
    return value.filter((item) => item !== null && item !== undefined).map((item) => stripNulls(item)) as T;
  }
  if (value !== null && typeof value === 'object' && Object.getPrototypeOf(value) === Object.prototype) {
    const out: Record<string, unknown> = {};
    for (const [key, item] of Object.entries(value)) {
      if (item !== null && item !== undefined) out[key] = stripNulls(item);
    }
    return out as T;
  }
  return value;
}
