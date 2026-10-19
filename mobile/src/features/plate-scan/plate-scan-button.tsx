import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable } from 'react-native';

import { Icon } from '@/ui/icon';

import { usePlateScanAvailable } from './capability';
import { PlateScanModal } from './plate-scan-modal';

/**
 * Plate scanning for any plate field: `available` is false (and `modal` is
 * null) unless the user turned "Plaka tanıma" on and the engine is ready, so
 * callers just render what they get.
 */
export function usePlateScan(onResult: (plate: string) => void) {
  const available = usePlateScanAvailable();
  const [open, setOpen] = useState(false);
  return {
    available,
    open: () => setOpen(true),
    modal: available ? <PlateScanModal visible={open} onClose={() => setOpen(false)} onResult={onResult} /> : null,
  };
}

/**
 * Square scan button for an Input's `trailing` slot (or next to a search
 * field with `compact`); renders nothing when unavailable.
 */
export function PlateScanButton({ onResult, compact }: { onResult: (plate: string) => void; compact?: boolean }) {
  const { t } = useTranslation('plate');
  const scan = usePlateScan(onResult);
  if (!scan.available) return null;
  return (
    <>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('scan_button')}
        hitSlop={4}
        onPress={scan.open}
        className={
          compact
            ? 'size-11 items-center justify-center rounded-lg bg-secondary active:opacity-70'
            : 'min-h-[52px] w-[52px] items-center justify-center rounded-lg border border-border bg-card active:bg-muted'
        }
      >
        <Icon name="scan" tone="primary" size={compact ? 20 : 22} />
      </Pressable>
      {scan.modal}
    </>
  );
}
