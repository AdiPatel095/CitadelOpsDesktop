import { useState } from 'react';
import { Icons } from './Icons';

interface RegistrationPendingProps {
    hardwareID: string | null;
}

const RegistrationPending = ({ hardwareID }: RegistrationPendingProps) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = () => {
        if (hardwareID) {
            navigator.clipboard.writeText(hardwareID);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        }
    };

    return (
        <div className="min-h-screen bg-bg-app flex items-center justify-center p-4 transition-colors duration-300">
            <div className="glass-panel p-8 max-w-lg w-full text-center bg-bg-card">
                {/* Icon */}
                <div className="rounded-global w-16 h-16 mx-auto mb-6 bg-yellow-500/20 flex items-center justify-center">
                    <Icons.AlertTriangle className="w-8 h-8 text-yellow-500" />
                </div>

                {/* Title */}
                <h1 className="text-2xl font-bold text-text-main mb-2">
                    Device Not Registered
                </h1>

                {/* Description */}
                <p className="text-text-muted mb-6">
                    This device needs to be registered before you can use CitadelOps Desktop.
                    Please register the hardware ID below on your cloud dashboard.
                </p>

                {/* Hardware ID Box */}
                <div className="rounded-global bg-bg-app p-4 mb-6 border border-border-base">
                    <p className="text-xs text-text-muted uppercase tracking-wider mb-2">Hardware ID</p>
                    <div className="flex items-center justify-center gap-3">
                        <code className="text-primary font-mono text-sm break-all">
                            {hardwareID || 'Loading...'}
                        </code>
                        {hardwareID && (
                            <button
                                onClick={handleCopy}
                                className="rounded-global p-2 hover:bg-bg-card-hover transition-colors text-text-muted hover:text-text-main"
                                title="Copy to clipboard"
                            >
                                {copied ? (
                                    <Icons.Check className="w-4 h-4 text-green-500" />
                                ) : (
                                    <Icons.Copy className="w-4 h-4" />
                                )}
                            </button>
                        )}
                    </div>
                </div>

                {/* Instructions */}
                <div className="rounded-global text-left bg-bg-app/50 p-4 mb-6 border border-border-base/50">
                    <h3 className="text-sm font-semibold text-text-main mb-2">How to register:</h3>
                    <ol className="text-sm text-text-muted space-y-2">
                        <li className="flex items-start gap-2">
                            <span className="text-primary font-bold">1.</span>
                            <span>Go to <a href="https://citadelops.app" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">citadelops.app</a></span>
                        </li>
                        <li className="flex items-start gap-2">
                            <span className="text-primary font-bold">2.</span>
                            <span>Navigate to "Licenses" in your dashboard</span>
                        </li>
                        <li className="flex items-start gap-2">
                            <span className="text-primary font-bold">3.</span>
                            <span>Click "Link Device" and paste the hardware ID above</span>
                        </li>
                        <li className="flex items-start gap-2">
                            <span className="text-primary font-bold">4.</span>
                            <span>Add credits to your device</span>
                        </li>
                    </ol>
                </div>

                {/* Auto-refresh notice */}
                <p className="text-xs text-text-muted">
                    <Icons.RefreshCw className="w-3 h-3 inline-block mr-1 animate-spin" />
                    This page will automatically update once registration is detected
                </p>
            </div>
        </div>
    );
};

export default RegistrationPending;
