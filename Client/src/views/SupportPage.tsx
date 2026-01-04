/**
 * @fileoverview Support Page Component
 *
 * Displays support options for the user.
 * Directs users to the Discord server for assistance.
 *
 * @module views/SupportPage
 */

import React from 'react';
import { Icons } from '../components/Icons';

// Discord invite link
const DISCORD_LINK = "https://discord.gg/zANyxDqfP3";

/**
 * SupportPage Component
 *
 * Renders the support view with instructions to join the Discord community.
 *
 * @returns The support page component
 */
const SupportPage: React.FC = () => {
    return (
        <div className="max-w-4xl mx-auto py-8">
            <h1 className="heading-1 mb-2">Support & Community</h1>
            <p className="text-text-muted mb-8 text-lg">
                Get help, report issues, or chat with other operators.
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                {/* Discord Card */}
                <div className="glass-panel p-8 flex flex-col items-center text-center hover:border-primary/30 transition-all duration-300 md:col-span-2">
                    <div className="w-20 h-20 bg-[#5865F2]/10 rounded-full flex items-center justify-center mb-6 ring-1 ring-[#5865F2]/20">
                        <Icons.Help className="w-10 h-10 text-[#5865F2]" />
                    </div>

                    <h2 className="text-2xl font-bold text-text-main mb-3">Join our Discord</h2>
                    <p className="text-text-muted max-w-lg mb-8 leading-relaxed">
                        The best way to get support is to join our Discord server.
                        Our team and community are active and ready to help you with any issues or questions.
                    </p>

                    <a
                        href={DISCORD_LINK}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="btn-primary rounded-global px-8 py-3 flex items-center gap-3 text-lg group bg-[#5865F2] hover:bg-[#4752C4] border-[#5865F2] text-white"
                    >
                        <span>Join Discord Server</span>
                        <Icons.ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
                    </a>
                </div>
            </div>
        </div>
    );
};

export default SupportPage;
