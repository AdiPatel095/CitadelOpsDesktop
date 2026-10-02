import { buttonAttributes } from '../components/ui/Button';
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

			<IntentConsole />

            <div className="mt-8">
                <Card variant="interactive" className="">
                    <CardContent className="flex flex-col items-center text-center">
                        <div className="w-20 h-20 bg-bg-raised rounded-full flex items-center justify-center mb-6">
                            <Icons.Help className="w-10 h-10 text-text-muted" />
                        </div>

                        <h2 className="text-headline font-bold text-text-main mb-3">{t('support.discordTitle')}</h2>
                        <p className="text-text-muted max-w-lg mb-8">
                            {t('support.discordBody')}
                        </p>

                        <a
                            href={DISCORD_LINK}
                            target="_blank"
                            rel="noopener noreferrer"
                            {...buttonAttributes({ variant: 'secondary', size: 'lg' })}
                        >
                            <span>{t('support.discordJoin')}</span>
                            <Icons.ArrowRight className={`w-5 h-5 ${direction === 'rtl' ? 'rotate-180' : ''}`}  />
                        </a>
                    </CardContent>
                </Card>
            </div>
        </div>
    );
};

export default SupportPage;
