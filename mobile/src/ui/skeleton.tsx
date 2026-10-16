import { useEffect } from 'react';
import Animated, {
  ReduceMotion,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withTiming,
} from 'react-native-reanimated';

import { cn } from './cn';

/** Placeholder block shown only when nothing is cached yet. Runs on the UI thread. */
export function Skeleton({ className }: { className?: string }) {
  const opacity = useSharedValue(0.5);
  useEffect(() => {
    opacity.value = withRepeat(withTiming(1, { duration: 800, reduceMotion: ReduceMotion.System }), -1, true);
  }, [opacity]);
  const style = useAnimatedStyle(() => ({ opacity: opacity.value }));
  return <Animated.View style={style} className={cn('rounded-md bg-muted', className)} />;
}
