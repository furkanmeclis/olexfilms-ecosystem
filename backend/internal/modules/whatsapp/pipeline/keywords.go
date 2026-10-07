package pipeline

import (
	"strings"
	"unicode"
)

// Keyword kinds. Keywords are whole messages, matched after folding (lower
// case, Turkish and common accented letters to ASCII, punctuation trimmed),
// and handled without a model call.
const (
	kwNone = iota
	// kwStop opts the number out of marketing messages (DUR / STOP).
	kwStop
	// kwHuman hands the conversation to a person (İNSAN / AGENT).
	kwHuman
	// kwYes accepts the AI guidelines or a confirmation card (EVET).
	kwYes
	// kwNo cancels a confirmation card (HAYIR).
	kwNo
)

type keyword struct {
	kind int
	// locale answers the keyword when the conversation has no locale yet.
	locale string
}

var keywords = func() map[string]keyword {
	m := map[string]keyword{}
	add := func(kind int, locale string, words ...string) {
		for _, w := range words {
			m[fold(w)] = keyword{kind: kind, locale: locale}
		}
	}
	add(kwStop, "tr", "dur", "iptal abonelik", "abonelikten çık")
	add(kwStop, "en", "stop", "unsubscribe")
	add(kwStop, "de", "stopp", "abmelden")
	add(kwStop, "fr", "arrêt", "désabonner")
	add(kwStop, "es", "parar", "baja")
	add(kwStop, "it", "basta", "disiscrivi")
	add(kwStop, "ru", "стоп", "отписаться")
	add(kwStop, "uk", "відписатися")
	add(kwStop, "el", "στοπ", "διακοπή")
	add(kwStop, "zh_CN", "退订", "停止")
	add(kwStop, "az", "dayan")
	add(kwStop, "ar", "توقف", "إلغاء الاشتراك")

	add(kwHuman, "tr", "insan", "temsilci", "müşteri temsilcisi", "yetkili")
	add(kwHuman, "en", "agent", "human", "representative", "operator")
	add(kwHuman, "de", "mensch", "mitarbeiter")
	add(kwHuman, "fr", "humain", "conseiller")
	add(kwHuman, "es", "agente", "humano")
	add(kwHuman, "it", "umano", "operatore")
	add(kwHuman, "ru", "оператор", "человек")
	add(kwHuman, "uk", "людина", "представник")
	add(kwHuman, "el", "άνθρωπος", "εκπρόσωπος")
	add(kwHuman, "zh_CN", "人工", "客服")
	add(kwHuman, "az", "nümayəndə")
	add(kwHuman, "ar", "موظف", "ممثل")

	add(kwYes, "tr", "evet", "kabul", "kabul ediyorum", "onaylıyorum")
	add(kwYes, "en", "yes")
	add(kwYes, "de", "ja")
	add(kwYes, "fr", "oui")
	add(kwYes, "es", "sí", "si")
	add(kwYes, "it", "sì")
	add(kwYes, "ru", "да")
	add(kwYes, "uk", "так")
	add(kwYes, "el", "ναι")
	add(kwYes, "zh_CN", "是", "是的")
	add(kwYes, "az", "bəli")
	add(kwYes, "ar", "نعم")

	add(kwNo, "tr", "hayır", "vazgeç", "iptal")
	add(kwNo, "en", "no", "cancel")
	add(kwNo, "de", "nein")
	add(kwNo, "fr", "non")
	add(kwNo, "ru", "нет")
	add(kwNo, "uk", "ні")
	add(kwNo, "bg", "не")
	add(kwNo, "el", "όχι")
	add(kwNo, "zh_CN", "否", "不")
	add(kwNo, "az", "xeyr")
	add(kwNo, "ar", "لا")
	return m
}()

// fold normalizes a message for keyword matching.
func fold(s string) string {
	s = strings.NewReplacer("İ", "i", "I", "ı").Replace(strings.TrimSpace(s))
	s = strings.ToLower(s)
	s = strings.NewReplacer(
		"ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c", "ə", "e",
		"â", "a", "à", "a", "á", "a", "é", "e", "è", "e", "ê", "e", "í", "i", "ì", "i",
		"ó", "o", "ò", "o", "ú", "u", "ù", "u", "ñ", "n",
		"ά", "α", "έ", "ε", "ή", "η", "ί", "ι", "ό", "ο", "ύ", "υ", "ώ", "ω",
	).Replace(s)
	s = strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r)
	})
	return strings.Join(strings.Fields(s), " ")
}

// keywordOf returns the keyword a whole message is (kwNone otherwise).
func keywordOf(text string) keyword {
	if k, ok := keywords[fold(text)]; ok {
		return k
	}
	return keyword{}
}
