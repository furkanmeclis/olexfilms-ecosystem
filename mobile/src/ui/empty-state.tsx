import { View } from 'react-native';

import { Button } from './button';
import { Icon } from './icon';
import { Text } from './text';
import type { IconName } from '@/ui/icons';

/** Says what will appear here and how to add the first one. */
export function EmptyState({
  icon,
  title,
  body,
  action,
}: {
  icon: IconName;
  title: string;
  body?: string;
  action?: { title: string; onPress: () => void };
}) {
  return (
    <View className="items-center gap-3 px-8 py-12">
      <View className="size-14 items-center justify-center rounded-xl bg-secondary">
        <Icon name={icon} tone="primary" size={26} />
      </View>
      <Text variant="headline" className="text-center">
        {title}
      </Text>
      {body ? (
        <Text variant="callout" tone="muted" className="text-center">
          {body}
        </Text>
      ) : null}
      {action ? <Button title={action.title} onPress={action.onPress} className="mt-2" /> : null}
    </View>
  );
}
