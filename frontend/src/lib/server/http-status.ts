/**
 * Statuses whose response must not carry a body, not even an empty one: the
 * Response constructor throws on them (TEC-649).
 */
export function isNullBodyStatus(status: number): boolean {
  return status === 204 || status === 205 || status === 304;
}
