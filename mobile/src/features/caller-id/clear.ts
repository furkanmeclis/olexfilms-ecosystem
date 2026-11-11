import { callerIdNative } from './native';
import { EMPTY_LIST, newTicket, useCallerId } from './store';

/**
 * Sign-out: the customer list leaves the device (TEC-1208). The switch
 * itself is remembered, so the same user gets the list back after signing in
 * again. Never rejects and never blocks the sign-out.
 */
export async function clearCallerId(): Promise<void> {
  try {
    newTicket();
    if (!callerIdNative.available || !useCallerId.getState().owner) return;
    useCallerId.getState().patch(EMPTY_LIST);
    await callerIdNative.clear();
  } catch {
    // The next sync or sign-in clears it again.
  }
}
