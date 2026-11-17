import { useTranslation } from 'react-i18next';

import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';

import { SCANNER_INPUT_MODES, useScannerInput } from './input-mode';

/** Settings section (TEC-1230): read codes with the camera or a hardware reader. */
export function ScannerInputSettings() {
  const { t } = useTranslation('scan');
  const mode = useScannerInput((s) => s.mode);
  return (
    <>
      <SectionHeader title={t('input.section')} />
      <List>
        {SCANNER_INPUT_MODES.map((m, i) => (
          <ListRow
            key={m}
            icon={m === 'camera' ? 'camera' : 'barcode'}
            title={t(`input.${m}`)}
            subtitle={t(`input.${m}_hint`)}
            onPress={() => useScannerInput.getState().setMode(m)}
            trailing={mode === m ? <Icon name="check" tone="primary" size={18} weight="semibold" /> : null}
            last={i === SCANNER_INPUT_MODES.length - 1}
          />
        ))}
      </List>
    </>
  );
}
