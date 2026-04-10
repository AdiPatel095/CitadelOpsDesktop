import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import { useAuth } from '../context/AuthContext';
import { Modal, Button, Card, CardContent } from './ui';
import { Icons } from './Icons';

interface UpdateModalProps {
  newVersion: string;
  downloadUrl: string;
  onDismiss: () => void;
}

const UpdateModal: React.FC<UpdateModalProps> = ({ newVersion, downloadUrl, onDismiss }) => {
  const { updateProgress, isUpdating, restartRequired, triggerUpdate, ignoreVersion } = useAuth();
  const [showConfirm, setShowConfirm] = useState(true);
  const patchNotesUrl = "https://citadelops.app/";

  const handleConfirmUpdate = () => {
    setShowConfirm(false);
    triggerUpdate(downloadUrl);
  };

  const handleIgnore = () => {
    ignoreVersion(newVersion);
    onDismiss();
  };

  if (restartRequired) {
    return createPortal(
      <div className="fixed inset-0 z-[200] flex items-center justify-center bg-bg-app transition-colors duration-300">
        <div className="absolute inset-0 opacity-5">
          <div className="absolute inset-0" style={{
            backgroundImage: 'radial-gradient(circle at 1px 1px, currentColor 1px, transparent 0)',
            backgroundSize: '40px 40px',
            color: 'var(--color-text-main)'
          }} />
        </div>

        <div className="relative text-center p-12 max-w-lg animate-fade-in">
          <div className="flex justify-center mb-8">
            <div className="w-24 h-24 rounded-full bg-success/20 flex items-center justify-center shadow-[0_0_40px_var(--color-success)] animate-pulse">
              <svg className="w-12 h-12 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
            </div>
          </div>

          <h1 className="text-3xl font-bold text-text-main mb-4">
            Update Complete!
          </h1>

          <p className="text-text-muted text-lg mb-8 leading-relaxed">
            Version <span className="text-primary font-semibold">{newVersion}</span> has been downloaded and installed successfully.
          </p>

          <Card variant="solid" className="mb-8 p-6 shadow-lg border-primary/30">
            <div className="flex items-center justify-center gap-3 text-primary">
              <Icons.RefreshCw className="w-6 h-6" />
              <span className="text-xl font-bold">Please restart the application</span>
            </div>
            <p className="text-text-muted text-sm mt-3">
              Close this window and reopen CitadelOps Desktop to use the new version.
            </p>
          </Card>

          <p className="text-text-muted opacity-75 text-sm">
            The application will not function properly until restarted.
          </p>
        </div>
      </div>,
      document.body
    );
  }

  if (isUpdating && !showConfirm) {
    return createPortal(
      <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/80 backdrop-blur-sm">
        <div className="relative text-center p-8 max-w-md animate-fade-in">
          <div className="flex justify-center mb-6">
            <div className="w-20 h-20 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_30px_var(--color-primary)]">
              <svg className="w-10 h-10 text-primary animate-bounce" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
            </div>
          </div>

          <h2 className="text-2xl font-bold text-white mb-2">
            Updating to v{newVersion}
          </h2>

          <p className="text-gray-400 mb-6">
            {updateProgress?.stage || 'Preparing update...'}
          </p>

          <div className="w-full h-3 bg-gray-800 rounded-full overflow-hidden mb-3">
            <div
              className="h-full bg-primary transition-all duration-300 rounded-full"
              style={{ width: `${updateProgress?.percent || 0}%` }}
            />
          </div>

          <p className="text-gray-500 text-sm">
            {updateProgress?.percent || 0}% complete
          </p>

          <p className="text-gray-600 text-xs mt-4 font-medium uppercase tracking-wider">
            Please do not close the application
          </p>
        </div>
      </div>,
      document.body
    );
  }

  if (showConfirm) {
    return (
      <Modal
        isOpen={showConfirm}
        onClose={onDismiss}
        maxWidth="md"
        hideCloseButton={true}
        title={
          <div className="flex flex-col items-center pt-4">
            <div className="w-20 h-20 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_20px_var(--color-primary-glow)] mb-6">
              <svg className="w-10 h-10 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
            </div>

            <div className="px-3 py-1 rounded-full bg-primary/10 border border-primary/30 text-primary text-xs font-bold uppercase tracking-wider mb-4">
              Update Available
            </div>
            
            <h3 className="text-2xl font-bold text-text-main text-center">
              Version {newVersion}
            </h3>
          </div>
        }
        footer={
          <div className="flex w-full gap-3">
            <Button variant="ghost" onClick={onDismiss} className="flex-1">
              Later
            </Button>
            <Button variant="outline" onClick={handleIgnore} className="flex-1 border-border-base text-text-muted" title="Don't show this update again">
              Ignore
            </Button>
            <Button variant="primary" onClick={handleConfirmUpdate} className="flex-[2]" leftIcon={<svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>}>
              Update Now
            </Button>
          </div>
        }
      >
        <div className="flex flex-col items-center pb-2">
          <p className="text-text-muted text-center mb-6 leading-relaxed">
            A new version of CitadelOps Desktop is available. Update now to get the latest features and improvements.
          </p>

          <a
            href={patchNotesUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-primary hover:text-primary-hover text-sm font-medium flex items-center gap-1.5 transition-colors mb-6"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            View patch notes
          </a>

          <div className="bg-bg-app/50 border border-border-base rounded-global p-4 w-full">
            <p className="text-text-muted text-sm text-center">
              After the update, you will need to <span className="text-text-main font-medium">restart the application</span> to use the new version.
            </p>
          </div>
        </div>
      </Modal>
    );
  }

  return null;
};

export default UpdateModal;
