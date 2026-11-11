/**
 * Periodic background sync of the caller ID list (TEC-1208). Imported once
 * from the root layout: a task has to be defined in the global scope, before
 * the system starts the app in the background.
 */
import { Platform } from 'react-native';

import { useSession } from '@/core/auth/session';

import { CALLER_ID_TASK } from './schedule';
import { syncCallerId } from './sync';

if (Platform.OS !== 'web') {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const TaskManager = require('expo-task-manager') as typeof import('expo-task-manager');
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const BackgroundTask = require('expo-background-task') as typeof import('expo-background-task');
    TaskManager.defineTask(CALLER_ID_TASK, async () => {
      try {
        // Started by the system: the stored session is not read yet.
        if (useSession.getState().status === 'loading') await useSession.getState().hydrate();
        await syncCallerId({ force: true });
        return BackgroundTask.BackgroundTaskResult.Success;
      } catch {
        return BackgroundTask.BackgroundTaskResult.Failed;
      }
    });
  } catch {
    // No task manager in this build (Expo Go without it, older binary).
  }
}
