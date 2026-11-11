import { Platform } from 'react-native';

/** Name of the periodic background sync (defined in `background.ts`). */
export const CALLER_ID_TASK = 'olex-caller-id-sync';
/** The system decides the real moment; this is the shortest gap we ask for. */
const MIN_INTERVAL_MINUTES = 6 * 60;

/** Registers or removes the periodic sync. Silent: the foreground sync still runs. */
export async function scheduleCallerIdTask(on: boolean): Promise<void> {
  if (Platform.OS === 'web') return;
  try {
    const BackgroundTask = await import('expo-background-task');
    const TaskManager = await import('expo-task-manager');
    const registered = await TaskManager.isTaskRegisteredAsync(CALLER_ID_TASK);
    if (on && !registered) {
      await BackgroundTask.registerTaskAsync(CALLER_ID_TASK, { minimumInterval: MIN_INTERVAL_MINUTES });
    } else if (!on && registered) {
      await BackgroundTask.unregisterTaskAsync(CALLER_ID_TASK);
    }
  } catch {
    // Background tasks are unavailable (Expo Go, restricted device).
  }
}
