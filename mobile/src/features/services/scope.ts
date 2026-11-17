/**
 * Network scope of the center's service list (TEC-1003). Pure, so node --test
 * runs it. The center opens on the whole network ('network') so dealer names
 * show at first sight, like the web (TEC-1219 fix); 'own' narrows the list to
 * the services its own service point performed. A saved choice wins.
 */
export type Scope = 'own' | 'network';

export const DEFAULT_SCOPE: Scope = 'network';

export function initialScope(saved: unknown): Scope {
  return saved === 'own' || saved === 'network' ? saved : DEFAULT_SCOPE;
}
