import React, { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { SERVER_OPTIONS, DEFAULT_SERVER } from '../config/servers';

interface LoginCredentialsModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSave: (username: string, password: string, server: string) => void;
    initialUsername?: string;
    initialServer?: string;
}

const LoginCredentialsModal: React.FC<LoginCredentialsModalProps> = ({
    isOpen,
    onClose,
    onSave,
    initialUsername = '',
    initialServer = DEFAULT_SERVER
}) => {
    const [username, setUsername] = useState(initialUsername);
    const [password, setPassword] = useState('');
    const [server, setServer] = useState(initialServer);

    useEffect(() => {
        if (isOpen) {
            setUsername(initialUsername);
            setPassword('');
            setServer(initialServer);
        }
    }, [isOpen, initialUsername, initialServer]);

    const handleSave = () => {
        if (!username.trim() || !password.trim()) {
            return;
        }
        onSave(username.trim(), password, server);
    };

    if (!isOpen) return null;

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
            {/* Backdrop */}
            <div
                className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                onClick={onClose}
            />

            {/* Modal Content */}
            <div className="relative glass-panel p-6 max-w-md w-full mx-4 animate-fade-in shadow-2xl bg-bg-card/95">
                {/* Icon */}
                <div className="flex justify-center mb-4">
                    <div className="w-16 h-16 rounded-full bg-primary/20 flex items-center justify-center shadow-[0_0_15px_rgba(52,211,153,0.2)]">
                        <svg className="w-8 h-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                        </svg>
                    </div>
                </div>

                <h3 className="text-xl font-bold text-text-main text-center mb-2">Login Credentials</h3>
                <p className="text-text-muted text-center text-sm mb-6">
                    Enter your game account details to enable automated login.
                </p>

                {/* Form Fields */}
                <div className="space-y-4 mb-6">
                    {/* Username */}
                    <div>
                        <label className="block text-xs font-bold text-text-muted uppercase tracking-wider mb-2">
                            Username / Email
                        </label>
                        <input
                            type="text"
                            value={username}
                            onChange={(e) => setUsername(e.target.value)}
                            className="w-full px-4 py-2.5 rounded-global bg-bg-input border border-border-base focus:border-primary focus:outline-none text-text-main placeholder-text-muted transition-all"
                            placeholder="Enter your username or email"
                            autoComplete="username"
                        />
                    </div>

                    {/* Password */}
                    <div>
                        <label className="block text-xs font-bold text-text-muted uppercase tracking-wider mb-2">
                            Password
                        </label>
                        <input
                            type="password"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            className="w-full px-4 py-2.5 rounded-global bg-bg-input border border-border-base focus:border-primary focus:outline-none text-text-main placeholder-text-muted transition-all"
                            placeholder="Enter your password"
                            autoComplete="current-password"
                        />
                    </div>

                    {/* Server Selection */}
                    <div>
                        <label className="block text-xs font-bold text-text-muted uppercase tracking-wider mb-2">
                            Server
                        </label>
                        <select
                            value={server}
                            onChange={(e) => setServer(e.target.value)}
                            className="w-full px-4 py-2.5 rounded-global bg-bg-input border border-border-base focus:border-primary focus:outline-none text-text-main transition-all cursor-pointer"
                        >
                            {Object.keys(SERVER_OPTIONS).map((displayName) => (
                                <option key={displayName} value={displayName}>
                                    {displayName}
                                </option>
                            ))}
                        </select>
                    </div>
                </div>

                {/* Info Note */}
                <div className="mb-6 p-3 rounded-global bg-yellow-500/10 border border-yellow-500/30">
                    <p className="text-xs text-yellow-400 text-center">
                        Credentials are stored locally in your browser for persistence.
                    </p>
                </div>

                {/* Buttons */}
                <div className="flex gap-3">
                    <button
                        onClick={onClose}
                        className="flex-1 px-4 py-2.5 rounded-global bg-bg-app border border-border-base hover:bg-bg-card-hover text-text-muted hover:text-text-main font-semibold transition-all duration-200"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleSave}
                        disabled={!username.trim() || !password.trim()}
                        className="flex-1 px-4 py-2.5 rounded-global bg-primary hover:bg-primary-hover text-bg-app font-bold transition-all shadow-lg shadow-primary/20 hover:shadow-primary/40 active:scale-95 duration-200 disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-primary"
                    >
                        Save & Login
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

export default LoginCredentialsModal;
