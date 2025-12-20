import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import { useAuth } from '../context/AuthContext';

interface UpdateBannerProps {
    newVersion: string;
    downloadUrl: string;
    onDismiss: () => void;
}

/**
 * UpdateBanner Component
 *
 * A closeable notification banner that appears when a new version is available.
 * Includes confirmation modal and progress indicator for self-update.
 */
const UpdateBanner: React.FC<UpdateBannerProps> = ({ newVersion, downloadUrl, onDismiss }) => {
    const { updateProgress, isUpdating, triggerUpdate } = useAuth();
    const [showConfirm, setShowConfirm] = useState(false);
    const patchNotesUrl = "https://citadelops.app/";

    const handleUpdateClick = () => {
        setShowConfirm(true);
    };

    const handleConfirmUpdate = () => {
        setShowConfirm(false);
        triggerUpdate(downloadUrl);
    };

    return (
        <>
            <div className="bg-dark-card/95 backdrop-blur-md border-b border-primary/30 shadow-[0_4px_20px_rgba(52,211,153,0.15)]">
                <div className="max-w-[1600px] mx-auto px-6 py-3 flex items-center justify-between gap-4">
                    {/* Left Section: Icon + Message */}
                    <div className="flex items-center gap-4">
                        {/* Animated Glow Icon */}
                        <div className="w-10 h-10 rounded-global bg-primary/10 flex items-center justify-center flex-shrink-0 shadow-[0_0_15px_rgba(52,211,153,0.3)]">
                            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-primary">
                                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                                <polyline points="7 10 12 15 17 10"></polyline>
                                <line x1="12" y1="15" x2="12" y2="3"></line>
                            </svg>
                        </div>

                        {/* Message */}
                        <div className="flex flex-col">
                            <div className="flex items-center gap-2">
                                <span className="text-[10px] font-bold text-gray-500 uppercase tracking-wider">
                                    {isUpdating ? 'Updating' : 'Update Available'}
                                </span>
                                <div className="w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_8px_rgba(52,211,153,0.8)] animate-pulse" />
                            </div>
                            <div className="flex items-center gap-2 mt-0.5">
                                {isUpdating && updateProgress ? (
                                    <span className="text-sm font-semibold text-white">
                                        {updateProgress.stage} {updateProgress.percent}%
                                    </span>
                                ) : (
                                    <>
                                        <span className="text-sm font-semibold text-white">Version {newVersion}</span>
                                        <span className="text-xs text-gray-500">is now available</span>
                                    </>
                                )}
                            </div>
                        </div>
                    </div>

                    {/* Right Section: Actions */}
                    <div className="flex items-center gap-3">
                        {isUpdating ? (
                            /* Progress Bar when updating */
                            <div className="w-32 h-2 bg-dark-bg rounded-full overflow-hidden">
                                <div
                                    className="h-full bg-primary transition-all duration-300 rounded-full"
                                    style={{ width: `${updateProgress?.percent || 0}%` }}
                                />
                            </div>
                        ) : (
                            <>
                                {/* View Patch Notes - Ghost Button */}
                                <a
                                    href={patchNotesUrl}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="px-4 py-1.5 rounded-global bg-dark-bg border border-dark-border hover:border-primary/50 hover:bg-primary/10 text-gray-400 hover:text-primary text-xs font-medium transition-all uppercase tracking-wider"
                                >
                                    Patch Notes
                                </a>

                                {/* Update & Restart - Primary Button */}
                                <button
                                    onClick={handleUpdateClick}
                                    className="px-5 py-1.5 rounded-global bg-primary hover:bg-primary-hover text-dark-bg text-xs font-bold uppercase tracking-wider transition-all shadow-lg shadow-primary/20 hover:shadow-primary/40 flex items-center gap-2 active:scale-95"
                                >
                                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                                        <polyline points="7 10 12 15 17 10"></polyline>
                                        <line x1="12" y1="15" x2="12" y2="3"></line>
                                    </svg>
                                    Update & Restart
                                </button>

                                {/* Close Button */}
                                <button
                                    onClick={onDismiss}
                                    className="w-8 h-8 flex items-center justify-center rounded-global text-gray-500 hover:text-white hover:bg-white/10 transition-all"
                                    title="Dismiss"
                                >
                                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                        <line x1="18" y1="6" x2="6" y2="18"></line>
                                        <line x1="6" y1="6" x2="18" y2="18"></line>
                                    </svg>
                                </button>
                            </>
                        )}
                    </div>
                </div>
            </div>

            {/* Confirmation Modal */}
            {showConfirm && createPortal(
                <div className="fixed inset-0 z-[100] flex items-center justify-center">
                    {/* Backdrop */}
                    <div
                        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                        onClick={() => setShowConfirm(false)}
                    />

                    {/* Modal Content */}
                    <div className="relative glass-panel p-6 max-w-sm w-full mx-4 animate-fade-in border border-white/10 shadow-2xl bg-[#0f1115]/90">
                        {/* Icon */}
                        <div className="flex justify-center mb-4">
                            <div className="w-16 h-16 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_15px_rgba(52,211,153,0.2)]">
                                <svg className="w-8 h-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 0 003 3h10a3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                                </svg>
                            </div>
                        </div>

                        <h3 className="text-xl font-bold text-white text-center mb-3">Update to v{newVersion}?</h3>

                        <p className="text-gray-300 text-center mb-6 leading-relaxed">
                            The application will <span className="text-primary font-semibold">download the update</span> and
                            <span className="text-primary font-semibold"> restart automatically</span>.
                        </p>

                        <div className="flex gap-3">
                            <button
                                onClick={() => setShowConfirm(false)}
                                className="flex-1 px-4 py-2.5 rounded-global bg-dark-bg border border-dark-border hover:bg-white/5 text-gray-400 hover:text-white font-semibold transition-all duration-200"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={handleConfirmUpdate}
                                className="flex-1 px-4 py-2.5 rounded-global bg-primary hover:bg-primary-hover text-dark-bg font-bold transition-all shadow-lg shadow-primary/20 hover:shadow-primary/40 active:scale-95 duration-200"
                            >
                                Update Now
                            </button>
                        </div>
                    </div>
                </div>,
                document.body
            )}
        </>
    );
};

export default UpdateBanner;
