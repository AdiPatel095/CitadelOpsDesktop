import React, { useEffect, useState } from 'react';
import { FrontendWebsocket } from '../websocket';
import { Icons } from './Icons';

type AlertCategory = 'green' | 'yellow' | 'red';

interface AlertMessage {
    id: string;
    category: AlertCategory;
    message: string;
}

const ALERT_DURATION = 5000; // 5 seconds

export const Alerts: React.FC = () => {
    const [alerts, setAlerts] = useState<AlertMessage[]>([]);

    useEffect(() => {
        const handleMessage = (message: any) => {
            if (message.type === 'alert') {
                const payload = message.payload;
                const newAlert: AlertMessage = {
                    id: Math.random().toString(36).substr(2, 9),
                    category: payload.category as AlertCategory,
                    message: payload.message,
                };

                setAlerts((prev) => [...prev, newAlert]);
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);

        return () => {
            FrontendWebsocket.removeMessageListener(handleMessage);
        };
    }, []);

    const removeAlert = (id: string) => {
        setAlerts((prev) => prev.filter((alert) => alert.id !== id));
    };

    return (
        <div className="fixed top-24 right-6 z-50 flex flex-col gap-3 w-96 pointer-events-none">
            {alerts.map((alert) => (
                <AlertItem key={alert.id} alert={alert} onDismiss={() => removeAlert(alert.id)} />
            ))}
        </div>
    );
};

const AlertItem: React.FC<{ alert: AlertMessage; onDismiss: () => void }> = ({ alert, onDismiss }) => {
    const [isExiting, setIsExiting] = useState(false);

    useEffect(() => {
        const timer = setTimeout(() => {
            handleDismiss();
        }, ALERT_DURATION);

        return () => clearTimeout(timer);
    }, []);

    const handleDismiss = () => {
        setIsExiting(true);
        setTimeout(onDismiss, 300); // Wait for animation
    };

    const getStyles = () => {
        switch (alert.category) {
            case 'green':
                return {
                    bg: 'bg-emerald-500/10 dark:bg-emerald-500/20',
                    border: 'border-emerald-500/20 dark:border-emerald-500/50',
                    // High Contrast Text: Almost black for light, Pure white for dark
                    text: 'text-emerald-950 dark:text-white font-semibold',
                    icon: <Icons.Check className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />,
                    shadow: 'shadow-[0_0_15px_rgba(16,185,129,0.1)] dark:shadow-[0_0_15px_rgba(16,185,129,0.2)]'
                };
            case 'yellow':
                return {
                    bg: 'bg-amber-500/10 dark:bg-amber-500/20',
                    border: 'border-amber-500/20 dark:border-amber-500/50',
                    // High Contrast Text
                    text: 'text-amber-950 dark:text-white font-semibold',
                    icon: <Icons.AlertTriangle className="w-5 h-5 text-amber-600 dark:text-amber-400" />,
                    shadow: 'shadow-[0_0_15px_rgba(245,158,11,0.1)] dark:shadow-[0_0_15px_rgba(245,158,11,0.2)]'
                };
            case 'red':
                return {
                    bg: 'bg-red-500/10 dark:bg-red-500/20',
                    border: 'border-red-500/20 dark:border-red-500/50',
                    // High Contrast Text
                    text: 'text-red-950 dark:text-white font-semibold',
                    icon: <Icons.AlertCircle className="w-5 h-5 text-red-600 dark:text-red-400" />,
                    shadow: 'shadow-[0_0_15px_rgba(239,68,68,0.1)] dark:shadow-[0_0_15px_rgba(239,68,68,0.2)]'
                };
            default:
                return {
                    bg: 'bg-bg-card',
                    border: 'border-border-base',
                    // High Contrast Default
                    text: 'text-text-main dark:text-white font-semibold',
                    icon: <Icons.Info className="w-5 h-5 text-text-muted" />,
                    shadow: 'shadow-lg'
                };
        }
    };

    const style = getStyles();

    return (
        <div
            className={`
                pointer-events-auto
                relative overflow-hidden
                flex items-start gap-3 p-4
                rounded-xl border backdrop-blur-md
                ${style.bg} ${style.border} ${style.shadow}
                transition-all duration-300 ease-out
                ${isExiting ? 'animate-fade-out-right' : 'animate-fade-in-right opacity-0'}
            `}
            role="alert"
        >
            <div className="shrink-0 mt-0.5">{style.icon}</div>
            <div className={`flex-1 text-sm ${style.text}`}>
                {alert.message}
            </div>
            <button
                onClick={handleDismiss}
                className={`
                    shrink-0 p-1 rounded-lg
                    hover:bg-white/10 transition-colors
                    ${style.text} opacity-70 hover:opacity-100
                `}
            >
                <Icons.X className="w-4 h-4" />
            </button>
        </div>
    );
};
