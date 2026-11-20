import { Linking, View } from 'react-native';

import { type Part, toBlocks } from '@/features/announcements/lib';
import { Text } from '@/ui/text';

function Inline({ parts }: { parts: Part[] }) {
  return (
    <>
      {parts.map((p, i) =>
        p.url ? (
          <Text
            key={i}
            variant="body"
            tone="primary"
            accessibilityRole="link"
            className="underline"
            onPress={() => void Linking.openURL(p.url!).catch(() => undefined)}>
            {p.text}
          </Text>
        ) : (
          p.text
        ),
      )}
    </>
  );
}

/** Admin Markdown (headings, lists, paragraphs, safe links) as native text. */
export function MarkdownView({ source }: { source: string }) {
  const blocks = toBlocks(source, 'markdown');
  return (
    <View className="gap-3">
      {blocks.map((b, i) =>
        b.kind === 'heading' ? (
          <Text key={i} variant="headline" selectable>
            <Inline parts={b.parts} />
          </Text>
        ) : b.kind === 'bullet' ? (
          <View key={i} className="flex-row gap-2 ps-1">
            <Text variant="body">{b.marker ?? '•'}</Text>
            <Text variant="body" selectable className="flex-1">
              <Inline parts={b.parts} />
            </Text>
          </View>
        ) : (
          <Text key={i} variant="body" selectable>
            <Inline parts={b.parts} />
          </Text>
        ),
      )}
    </View>
  );
}
