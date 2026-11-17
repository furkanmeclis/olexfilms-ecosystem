import { View } from 'react-native';

import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Text } from '@/ui/text';

export type TimelineStep = {
  key: string;
  /** Who took the step; the caller passes its "System" label for a null user. */
  user: string;
  /** Translated action label. */
  label: string;
  /** Formatted time of the step. */
  when: string;
};

/**
 * Movement history of a document (TEC-1060 / revize 5.4.1, TEC-1220): who did
 * what and when, one row per step. The caller passes the steps newest first,
 * like the order timeline. Renders nothing without steps.
 */
export function StepTimeline({ title, steps }: { title: string; steps: TimelineStep[] }) {
  if (!steps.length) return null;
  return (
    <>
      <SectionHeader title={title} />
      <View className="gap-0 rounded-lg border border-border bg-card">
        {steps.map((s, i) => (
          <View
            key={s.key}
            className={i < steps.length - 1 ? 'flex-row gap-3 border-b border-border px-4 py-3' : 'flex-row gap-3 px-4 py-3'}>
            <View className="pt-0.5">
              <Icon name="clock" tone="muted-foreground" size={16} />
            </View>
            <View className="flex-1 gap-0.5">
              <Text variant="callout" className="font-semibold" numberOfLines={1}>
                {s.user}
              </Text>
              <Text variant="callout">{s.label}</Text>
            </View>
            <Text variant="footnote" tone="muted" className="tabular-nums">
              {s.when}
            </Text>
          </View>
        ))}
      </View>
    </>
  );
}

