/**
 * Search key: Turkish-aware lowercase with diacritics stripped, so "izmir"
 * matches "İzmir", "canakkale" matches "Çanakkale" and "ayse" matches "Ayşe".
 */
export function foldSearch(value: string): string {
  return value
    .toLocaleLowerCase("tr-TR")
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/ı/g, "i");
}
