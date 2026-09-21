import { useLocale } from '../../i18n/LocaleContext';
import React, { type ReactNode } from 'react';
import { Save } from 'lucide-react';
import { Button } from './Button';
import { Modal, type ModalProps } from './Modal';
import { ModalTitle } from './ModalTitle';

export interface SettingsModalProps extends Omit<ModalProps, 'title' | 'footer'> {
  title: ReactNode;
  icon: ReactNode;
  description?: ReactNode;
  titleTrailing?: ReactNode;
  onSave: () => void;
  saveLabel?: ReactNode;
  isSaving?: boolean;
  saveDisabled?: boolean;
  cancelDisabled?: boolean;
  cancelLabel?: ReactNode;
  footerLeading?: ReactNode;
}

export const SettingsModal: React.FC<SettingsModalProps> = ({
  title,
  icon,
  description,
  titleTrailing,
  onSave,
  saveLabel,
  isSaving = false,
  saveDisabled = false,
  cancelDisabled = false,
  cancelLabel,
  footerLeading,
  onClose,
  children,
  ...modalProps
}) => {
  const { t, messageLocale } = useLocale();
  return (
  <Modal
    {...modalProps}
    onClose={onClose}
    title={<ModalTitle icon={icon} description={description} trailing={titleTrailing}>{title}</ModalTitle>}
    footer={(
      <>
        {footerLeading}
        <Button lang={messageLocale} variant="ghost" onClick={onClose} disabled={cancelDisabled || isSaving} className="px-6">
          {cancelLabel ?? t('settings.cancel')}
        </Button>
        <Button
          lang={messageLocale}
          variant="primary"
          onClick={onSave}
          disabled={saveDisabled}
          isLoading={isSaving}
          className="px-8"
          leftIcon={<Save className="h-4 w-4" />}
        >
          {saveLabel ?? t('settings.save')}
        </Button>
      </>
    )}
  >
    {children}
  </Modal>
);
};
