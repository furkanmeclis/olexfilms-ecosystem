/**
 * Factory curves shipped with the app (pure, alias-free), used when our
 * backend (TEC-934, which fetches the vendor file server side) has no curve
 * for the serial or cannot be reached (see resolveCurve in curve.ts).
 * SN 32475 (SmartPTG Professional) is the curve the old app bundled; its
 * values equal the decrypted factory file (see curve.test.ts).
 */
import type { CurvePoint, GaugeCurve } from './curve.ts';

const pts = (rows: [number, number][]): CurvePoint[] => rows.map(([thickness, frequency]) => ({ thickness, frequency }));

export const BUNDLED_CURVES: Record<number, GaugeCurve> = {
  32475: {
    serial: 32475,
    deviceType: 2,
    feThreshold: 22291,
    alThreshold: 23490,
    fe: pts([
      [0, 15836], [58, 17891], [90, 18240], [169, 18848], [216, 19131], [276, 19415], [334, 19647], [367, 19762],
      [466, 20062], [588, 20356], [870, 20825], [1255, 21247], [1816, 21615], [2240, 21781], [3200, 22002],
    ]),
    al: pts([
      [0, 26391], [52, 26250], [90, 26190], [108, 26140], [159, 26026], [222, 25902], [279, 25797], [311, 25745],
      [341, 25691], [400, 25578], [797, 25077], [1203, 24712], [1756, 24377], [2272, 24175], [3205, 23947],
    ]),
  },
};
