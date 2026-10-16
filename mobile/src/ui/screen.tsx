import { ScrollView, type ScrollViewProps } from 'react-native';

import { cn } from './cn';

/**
 * Scrollable screen body under a native large-title header. The header
 * collapses natively because the scroll view adjusts its insets automatically.
 */
export function Screen({ children, className, ...props }: ScrollViewProps & { className?: string }) {
  return (
    <ScrollView
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      className="flex-1 bg-background"
      contentContainerClassName={cn('gap-4 px-4 pb-10 pt-2', className)}
      {...props}>
      {children}
    </ScrollView>
  );
}
