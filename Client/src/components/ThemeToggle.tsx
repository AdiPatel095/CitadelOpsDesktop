import { Button } from './ui/Button';
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
        <Button iconOnly variant="ghost"
            lang={messageLocale}
            onClick={toggleTheme}
            className={className}
            aria-label={t('theme.toggle')}
            title={theme === 'light' ? t('theme.switchDark') : t('theme.switchLight')}
        >
            {theme === 'light' ? (
                <Moon className="h-5 w-5" />
            ) : (
                <Sun className="h-5 w-5" />
            )}
        </Button>
    );
};
