import { useLocale } from '../i18n/LocaleContext';
import React from 'react';
import { Moon, Sun } from 'lucide-react';
import { useTheme } from '../context/ThemeContext';

interface ThemeToggleProps {
    className?: string;
}

export const ThemeToggle: React.FC<ThemeToggleProps> = ({ className = '' }) => {
    const { t, messageLocale } = useLocale();
    const { theme, toggleTheme } = useTheme();

    return (
        <button
            lang={messageLocale}
            onClick={toggleTheme}
            className={`
        m3-icon-button liquid-surface-edge flex items-center justify-center rounded-full p-2
        text-text-muted hover:text-primary hover:border-primary/30
        transition-all duration-200 ease-in-out
        ${className}
      `}
            aria-label={t('theme.toggle')}
            title={theme === 'light' ? t('theme.switchDark') : t('theme.switchLight')}
        >
            {theme === 'light' ? (
                <Moon className="h-5 w-5" />
            ) : (
                <Sun className="h-5 w-5" />
            )}
        </button>
    );
};
