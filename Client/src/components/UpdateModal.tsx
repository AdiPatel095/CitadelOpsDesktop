import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import { useAuth } from '../context/AuthContext';

interface UpdateModalProps {
    newVersion: string;
    downloadUrl: string;
    onDismiss: () => void;
}

/**
 * UpdateModal Component
 *
 * A modal that appears when a new version is available.
 * Shows update confirmation, progress during update, and restart required screen.
 */
const UpdateModal: React.FC<UpdateModalProps> = ({ newVersion, downloadUrl, onDismiss }) => {
    const { updateProgress, isUpdating, restartRequired, triggerUpdate } = useAuth();
    const [showConfirm, setShowConfirm] = useState(true);
    const patchNotesUrl = "https://citadelops.app/";

    const handleConfirmUpdate = () => {
        setShowConfirm(false);
        triggerUpdate(downloadUrl);
    };

    // Full-screen restart required overlay
    if (restartRequired) {
        return createPortal(
            <div className="fixed inset-0 z-[200] flex items-center justify-center bg-bg-app transition-colors duration-300">
                {/* Background pattern */}
                <div className="absolute inset-0 opacity-5">
                    <div className="absolute inset-0" style={{
                        backgroundImage: 'radial-gradient(circle at 1px 1px, currentColor 1px, transparent 0)',
                        backgroundSize: '40px 40px',
                        color: 'var(--color-text-main)'
                    }} />
                </div>

                {/* Content */}
                <div className="relative text-center p-12 max-w-lg animate-fade-in">
                    {/* Success Icon */}
                    <div className="flex justify-center mb-8">
                        <div className="w-24 h-24 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_40px_rgba(52,211,153,0.3)] animate-pulse">
                            <svg className="w-12 h-12 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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

                    <div className="bg-bg-card border border-primary/30 rounded-global p-6 mb-8 shadow-lg">
                        <div className="flex items-center justify-center gap-3 text-primary">
                            <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                            </svg>
                            <span className="text-xl font-bold">Please restart the application</span>
                        </div>
                        <p className="text-text-muted text-sm mt-3">
                            Close this window and reopen CitadelOps Desktop to use the new version.
                        </p>
                    </div>

                    <p className="text-text-muted opacity-75 text-sm">
                        The application will not function properly until restarted.
                    </p>
                </div>
            </div>,
            document.body
        );
    }

    // Update in progress overlay
    if (isUpdating && !showConfirm) {
        return createPortal(
            <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/80 backdrop-blur-sm">
                <div className="relative text-center p-8 max-w-md animate-fade-in">
                    {/* Animated download icon */}
                    <div className="flex justify-center mb-6">
                        <div className="w-20 h-20 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_30px_rgba(52,211,153,0.3)]">
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

                    {/* Progress bar */}
                    <div className="w-full h-3 bg-gray-800 rounded-full overflow-hidden mb-3">
                        <div
                            className="h-full bg-gradient-to-r from-primary to-emerald-400 transition-all duration-300 rounded-full"
                            style={{ width: `${updateProgress?.percent || 0}%` }}
                        />
                    </div>

                    <p className="text-gray-500 text-sm">
                        {updateProgress?.percent || 0}% complete
                    </p>

                    <p className="text-gray-600 text-xs mt-4">
                        Please do not close the application
                    </p>
                </div>
            </div>,
            document.body
        );
    }

    // Initial update available modal
    if (showConfirm) {
        return createPortal(
            <div className="fixed inset-0 z-[100] flex items-center justify-center">
                {/* Backdrop */}
                <div
                    className="absolute inset-0 bg-black/70 backdrop-blur-sm"
                    onClick={onDismiss}
                />

                {/* Modal Content */}
                <div className="relative glass-panel p-8 max-w-md w-full mx-4 animate-fade-in border border-border-base shadow-2xl bg-bg-card">
                    {/* Icon */}
                    <div className="flex justify-center mb-6">
                        <div className="w-20 h-20 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_20px_rgba(52,211,153,0.3)]">
                            <svg className="w-10 h-10 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                            </svg>
                        </div>
                    </div>

                    {/* Badge */}
                    <div className="flex justify-center mb-4">
                        <span className="px-3 py-1 rounded-full bg-primary/10 border border-primary/30 text-primary text-xs font-bold uppercase tracking-wider">
                            Update Available
                        </span>
                    </div>

                    <h3 className="text-2xl font-bold text-text-main text-center mb-3">
                        Version {newVersion}
                    </h3>

                    <p className="text-text-muted text-center mb-6 leading-relaxed">
                        A new version of CitadelOps Desktop is available. Update now to get the latest features and improvements.
                    </p>

                    {/* Patch notes link */}
                    <div className="flex justify-center mb-6">
                        <a
                            href={patchNotesUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:text-primary-hover text-sm font-medium flex items-center gap-1.5 transition-colors"
                        >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                            </svg>
                            View patch notes
                        </a>
                    </div>

                    {/* Info box */}
                    <div className="bg-bg-app/50 border border-border-base rounded-global p-4 mb-6">
                        <p className="text-text-muted text-sm text-center">
                            After the update, you will need to <span className="text-text-main font-medium">restart the application</span> to use the new version.
                        </p>
                    </div>

                    <div className="flex gap-3">
                        <button
                            onClick={onDismiss}
                            className="flex-1 px-4 py-3 rounded-global bg-bg-app border border-border-base hover:bg-bg-card-hover text-text-muted hover:text-text-main font-semibold transition-all duration-200"
                        >
                            Later
                        </button>
                        <button
                            onClick={handleConfirmUpdate}
                            className="flex-1 px-4 py-3 rounded-global bg-primary hover:bg-primary-hover text-bg-app font-bold transition-all shadow-lg shadow-primary/20 hover:shadow-primary/40 active:scale-95 duration-200 flex items-center justify-center gap-2"
                        >
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                            </svg>
                            Update Now
                        </button>
                    </div>
                </div>
            </div>,
            document.body
        );
    }

    return null;
};

export default UpdateModal;
