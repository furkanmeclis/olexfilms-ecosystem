import { uuid } from 'expo';
import { File } from 'expo-file-system';
import * as Haptics from 'expo-haptics';
import { useGlobalSearchParams, usePathname, useSegments } from 'expo-router';
import { useEffect, useEffectEvent, useRef, useState } from 'react';
import { AppState } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useResolvedSpace } from '@/core/spaces';

import { collectMeta } from './context';
import { FeedbackEditor } from './editor';
import { type FeedbackMeta, pushHistory, splitRouteParams } from './meta';
import { feedbackSupported, loadSensors, loadViewShot } from './native';
import { createShakeDetector, SHAKE_SAMPLE_INTERVAL_MS } from './shake';
import { useFeedback } from './store';

/** Last visited paths of this app run (newest last), for `meta.json`. */
let history: string[] = [];

type Draft = { imageUri: string; meta: FeedbackMeta };

function discardTemp(uri: string | null) {
  if (!uri) return;
  try {
    const f = new File(uri);
    if (f.exists) f.delete();
  } catch {
    // A leftover temp file is cleaned by the OS.
  }
}

/**
 * Shake-to-report, mounted once above the root Stack while signed in. With
 * the setting on (and the native modules present) a shake captures the
 * screen and opens the editor. The accelerometer only runs while the setting
 * is on, the app is in the foreground and no editor is open.
 */
export function FeedbackRoot() {
  const enabled = useFeedback((s) => s.enabled);
  const me = useSession((s) => s.me);
  const pathname = usePathname();
  const segments = useSegments() as string[];
  const params = useGlobalSearchParams();
  const space = useResolvedSpace();
  const [foreground, setForeground] = useState(AppState.currentState === 'active');
  const [draft, setDraft] = useState<Draft | null>(null);
  const busy = useRef(false);

  useEffect(() => {
    history = pushHistory(history, pathname);
  }, [pathname]);

  useEffect(() => {
    const sub = AppState.addEventListener('change', (s) => setForeground(s === 'active'));
    return () => sub.remove();
  }, []);

  const onShake = useEffectEvent(async () => {
    if (busy.current || draft) return;
    const shot = loadViewShot();
    if (!shot) return;
    busy.current = true;
    // Where the user is right now, before anything on screen changes.
    const location = {
      pathname,
      segments,
      ...splitRouteParams(segments, params),
      history: [...history],
      space,
    };
    let imageUri: string | null = null;
    try {
      imageUri = await shot.captureScreen({ format: 'png', quality: 1, result: 'tmpfile' });
      const meta = await collectMeta(uuid.v4(), new Date(), location, me);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      setDraft({ imageUri, meta });
    } catch {
      // Capture failed (e.g. a secure screen): no editor, nothing saved.
      discardTemp(imageUri);
    } finally {
      busy.current = false;
    }
  });

  // Off while the editor is open (`draft`) and in the background.
  const listening = enabled && foreground && !draft;
  useEffect(() => {
    if (!listening || !feedbackSupported()) return;
    const sensors = loadSensors();
    if (!sensors) return;
    const detector = createShakeDetector();
    let sub: { remove: () => void } | null = null;
    try {
      sensors.Accelerometer.setUpdateInterval(SHAKE_SAMPLE_INTERVAL_MS);
      sub = sensors.Accelerometer.addListener((m) => {
        if (detector.push({ x: m.x, y: m.y, z: m.z, t: Date.now() })) void onShake();
      });
    } catch {
      sub = null;
    }
    return () => sub?.remove();
  }, [listening]);

  if (!draft) return null;
  return (
    <FeedbackEditor
      imageUri={draft.imageUri}
      onCancel={() => {
        discardTemp(draft.imageUri);
        setDraft(null);
      }}
      onSave={async (note, annotatedUri) => {
        useFeedback.getState().save({ ...draft.meta, note }, draft.imageUri, annotatedUri);
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
        setDraft(null);
      }}
    />
  );
}
