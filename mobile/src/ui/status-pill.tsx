import { View } from 'react-native';

import { cn } from './cn';
import { Text } from './text';

export type StatusTone = 'neutral' | 'progress' | 'done' | 'problem';

const tones: Record<StatusTone, { box: string; dot: string }> = {
  neutral: { box: 'bg-secondary', dot: 'bg-muted-foreground' },
  progress: { box: 'bg-warning/15', dot: 'bg-warning' },
  done: { box: 'bg-success/15', dot: 'bg-success' },
  problem: { box: 'bg-destructive/15', dot: 'bg-destructive' },
};

/** State always as text plus color: never color alone. */
export function StatusPill({ label, tone = 'neutral' }: { label: string; tone?: StatusTone }) {
  return (
    <View className={cn('flex-row items-center gap-1.5 self-start rounded-full px-2.5 py-1', tones[tone].box)}>
      <View className={cn('size-1.5 rounded-full', tones[tone].dot)} />
      <Text variant="footnote" className="font-semibold">
        {label}
      </Text>
    </View>
  );
}
