import React, { useEffect, useState } from 'react';
import { FrontendWebsocket } from '../websocket';
import { useAuth } from '../context/AuthContext';

const InsufficientCreditsModal: React.FC = () => {
    const [isVisible, setIsVisible] = useState(false);
    const { credits } = useAuth();

    useEffect(() => {
        const handleMessage = (message: any) => {
            if (message.type === 'insufficientCredits') {
                setIsVisible(true);
            } else if (message.type === 'creditsUpdate') {
                if (message.payload.credits > 0) {
                    setIsVisible(false);
                }
            } else if (message.type === 'registrationStatus') {
                if (message.payload.credits > 0) {
                    setIsVisible(false);
                }
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);

        // Also check credits from context whenever they change
        // If credits drop to 0, should we show it? Maybe only if action failed?
        // User requested: "if the license runs out of credit"
        // But specifically "perform a deduction before sending each message... if... runs out"
        // So explicitly on failure message is best.
        // However, if we receive credits > 0, we can dismiss it.
        if (credits > 0) {
            setIsVisible(false);
        }

        return () => {
            FrontendWebsocket.removeMessageListener(handleMessage);
        };
    }, [credits]);

    if (!isVisible) return null;

    return (
        <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 backdrop-blur-sm">
            <div className="bg-zinc-900 border border-red-500/30 rounded-2xl p-8 max-w-md w-full mx-4 shadow-2xl relative overflow-hidden">
                {/* Background Glow */}
                <div className="absolute top-0 right-0 -mr-16 -mt-16 w-32 h-32 bg-red-500/20 blur-3xl rounded-full" />
                <div className="absolute bottom-0 left-0 -ml-16 -mb-16 w-32 h-32 bg-red-500/20 blur-3xl rounded-full" />

                <div className="relative z-10 flex flex-col items-center text-center">
                    {/* Icon */}
                    <div className="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center mb-6 ring-1 ring-red-500/30">
                        <svg className="w-8 h-8 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                        </svg>
                    </div>

                    <h2 className="text-2xl font-bold text-white mb-2">Insufficient Credits</h2>

                    <p className="text-zinc-400 mb-8 leading-relaxed">
                        Your license has run out of credits. Please go to your cloud dashboard to add more credits to this license to continue.
                    </p>

                    <a
                        href="https://citadelops.app/dashboard"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="w-full py-3 px-4 bg-red-600 hover:bg-red-700 text-white font-medium rounded-xl transition-colors duration-200 flex items-center justify-center group"
                    >
                        <span>Go to Dashboard</span>
                        <svg className="w-4 h-4 ml-2 group-hover:translate-x-1 transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                        </svg>
                    </a>

                    <p className="mt-4 text-xs text-zinc-500">
                        The modal will automatically dismiss when credits are detected.
                    </p>
                </div>
            </div>
        </div>
    );
};

export default InsufficientCreditsModal;
