import React, { useState } from 'react';
import { useAuth } from '../context/AuthContext';

const CustomMessageSender: React.FC = () => {
    const [messageCode, setMessageCode] = useState('');
    const { sendMessage } = useAuth();

    const handleSend = () => {
        if (!messageCode.trim()) return;

        sendMessage('sendCustomMessage', {
            messageCode: messageCode.trim()
        });

        // Clear input after sending
        setMessageCode('');
    };

    const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter') {
            handleSend();
        }
    };

    return (
        <div className="bg-slate-800 p-4 rounded-xl shadow border border-slate-700 w-full mb-6">
            <h2 className="text-xl font-semibold mb-4 text-slate-100 flex items-center">
                <svg className="w-5 h-5 mr-2 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
                </svg>
                Send Custom Message
            </h2>
            <div className="flex gap-2">
                <input
                    type="text"
                    value={messageCode}
                    onChange={(e) => setMessageCode(e.target.value)}
                    onKeyDown={handleKeyDown}
                    placeholder="e.g. lli, ggm"
                    className="flex-1 bg-slate-900 border border-slate-700 text-slate-100 rounded-lg px-4 py-2 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                    maxLength={10}
                />
                <button
                    onClick={handleSend}
                    disabled={!messageCode.trim()}
                    className="bg-indigo-600 hover:bg-indigo-700 disabled:bg-slate-700 disabled:text-slate-500 text-white px-6 py-2 rounded-lg font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-800"
                >
                    Send
                </button>
            </div>
            <p className="text-xs text-slate-400 mt-2">
                Sends a raw formatted message (%%xt%%EmpireEx_21%%[code]%%1%%{ }%%) directly to the game server.
            </p>
        </div>
    );
};

export default CustomMessageSender;
