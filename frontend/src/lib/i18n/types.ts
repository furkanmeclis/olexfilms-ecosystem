/** Flat keys of one namespace file: { "a.b": "text" }. */
export type MessageDictionary = Record<string, string>;

/** Every namespace of one language: { common: {...}, auth: {...} }. */
export type LocaleCatalog = Record<string, MessageDictionary>;
