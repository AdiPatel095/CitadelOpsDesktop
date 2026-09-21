import { useLocale } from '../i18n/LocaleContext';
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
import IntentConsole from '../components/IntentConsole';
import { Card, CardContent, PageHeader } from '../components/ui';

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
    const { t, messageLocale, direction } = useLocale();
    return (
        <div lang={messageLocale} className="max-w-4xl mx-auto py-8">
            <PageHeader
                className="mb-8"
                title={t('support.title')}
                description={t('support.description')}
            />

			<div lang="en"><IntentConsole /></div>

            <div className="mt-8">
                <Card variant="interactive" className="hover:border-primary/30 transition-all duration-300">
                    <CardContent className="p-8 flex flex-col items-center text-center">
                        <div className="w-20 h-20 bg-[#5865F2]/10 rounded-full flex items-center justify-center mb-6 ring-1 ring-[#5865F2]/20">
                            <Icons.Help className="w-10 h-10 text-[#5865F2]" />
                        </div>

                        <h2 className="text-2xl font-bold text-text-main mb-3">{t('support.discordTitle')}</h2>
                        <p className="text-text-muted max-w-lg mb-8 leading-relaxed">
                            {t('support.discordBody')}
                        </p>

                        <a
                            href={DISCORD_LINK}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center justify-center rounded-global font-semibold transition-all duration-200 active:scale-95 focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap px-8 py-3 text-lg gap-3 group bg-[#5865F2] hover:bg-[#4752C4] text-white shadow-lg shadow-[#5865F2]/20"
                        >
                            <span>{t('support.discordJoin')}</span>
                            <Icons.ArrowRight className={`w-5 h-5 transition-transform ${direction === 'rtl' ? 'rotate-180 group-hover:-translate-x-1' : 'group-hover:translate-x-1'}`} />
                        </a>
                    </CardContent>
                </Card>
            </div>
        </div>
    );
};

export default SupportPage;
