import React, { useEffect, useState } from 'react';
import { FrontendWebsocket } from '../Websocket';
import { Icons } from './Icons';

type AlertCategory = 'green' | 'yellow' | 'red';

const DECORATION_STORAGE_SHORTFALL_ID = 'decoration-storage-shortfall';
const DECORATION_APPLY_RUNNING_ID = 'decoration-apply-running';

interface AlertMessage {
    id: string;
    category: AlertCategory;
    message: string;
    /** Optional bullet lines shown under the title (e.g. decoration storage shortfall). */
    lines?: string[];
    /** When true, no auto-dismiss; user must close or condition clears it. */
    persistent?: boolean;
    /** Show Cancel apply (decoration preset replacer). */
    applyCancel?: boolean;
}

function stripDecorationApplyAlerts(prev: AlertMessage[]) {
    return prev.filter((a) => a.id !== DECORATION_APPLY_RUNNING_ID);
}

function shouldClearDecorationApplyRunning(alertMessage: string, category: string): boolean {
    const m = alertMessage;
    if (category === 'green' && m.includes('Decoration preset applied successfully')) {
        return true;
    }
    if (category === 'yellow' && m.includes('Decoration apply cancel requested')) {
        return true;
    }
    if (category === 'red') {
        if (m.startsWith('Decoration apply')) {
            return true;
        }
        if (m.startsWith('applyDecorationPreset')) {
            return true;
        }
        if (m.startsWith('Decoration preset not found')) {
            return true;
        }
    }
    return false;
}

const ALERT_DURATION_MS = 5000;

export const Alerts: React.FC = () => {
    const [alerts, setAlerts] = useState<AlertMessage[]>([]);

    useEffect(() => {
        const handleMessage = (message: { type?: string; payload?: unknown }) => {
            if (message.type === 'alert') {
                const payload = message.payload as {
                    category?: string;
                    message?: string;
                    lines?: string[];
                    persistent?: boolean;
                };
                if (!payload?.message) return;

                const m = payload.message;
                const category = (payload.category as AlertCategory) ?? 'yellow';

                // Replace generic "started" toast with an in-progress banner (Cancel) in this stack only.
                if (m.includes('Decoration preset apply started')) {
                    setAlerts((prev) => [
                        ...stripDecorationApplyAlerts(prev),
                        {
                            id: DECORATION_APPLY_RUNNING_ID,
                            category: 'yellow',
                            message: 'Decoration preset apply started.',
                            persistent: true,
                            applyCancel: true,
                        },
                    ]);
                    return;
                }

                const newAlert: AlertMessage = {
                    id: Math.random().toString(36).substr(2, 9),
                    category,
                    message: m,
                    lines: Array.isArray(payload.lines) ? payload.lines : undefined,
                    persistent: Boolean(payload.persistent),
                };

                setAlerts((prev) => {
                    let next = prev;
                    if (shouldClearDecorationApplyRunning(m, category)) {
                        next = stripDecorationApplyAlerts(next);
                    }
                    return [...next, newAlert];
                });
                return;
            }

            if (message.type === 'decorationPlacerStorageMismatch' && message.payload && typeof message.payload === 'object') {
                const raw = (message.payload as { items?: { line?: string }[] }).items;
                const lines =
                    Array.isArray(raw) && raw.length > 0
                        ? raw.map((it) => (typeof it?.line === 'string' ? it.line : '')).filter(Boolean)
                        : [];

                setAlerts((prev) => {
                    const without = prev.filter(
                        (a) => a.id !== DECORATION_STORAGE_SHORTFALL_ID && a.id !== DECORATION_APPLY_RUNNING_ID
                    );
                    if (lines.length === 0) {
                        return without;
                    }
                    return [
                        ...without,
                        {
                            id: DECORATION_STORAGE_SHORTFALL_ID,
                            category: 'red' as const,
                            message:
                                'Not enough in storage to place these (after counting stash + pickups still on this castle):',
                            lines,
                            persistent: true,
                        },
                    ];
                });
                return;
            }

            if (message.type === 'decorationPlacerProgress' && message.payload && typeof message.payload === 'object') {
                const m = ((message.payload as { message?: string }).message ?? '').trim();
                const terminal =
                    /^complete(:|\b)/i.test(m) ||
                    m === 'complete' ||
                    /^error:/i.test(m) ||
                    /^cancelled(:|\b)?$/i.test(m);
                if (terminal) {
                    setAlerts((prev) =>
                        prev.filter(
                            (a) => a.id !== DECORATION_STORAGE_SHORTFALL_ID && a.id !== DECORATION_APPLY_RUNNING_ID
                        )
                    );
                }
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
        <div className="fixed top-24 right-6 z-50 flex flex-col gap-3 w-96 max-w-[calc(100vw-3rem)] pointer-events-none">
            {alerts.map((alert) => (
                <AlertItem key={alert.id} alert={alert} onDismiss={() => removeAlert(alert.id)} />
            ))}
        </div>
    );
};

const AlertItem: React.FC<{ alert: AlertMessage; onDismiss: () => void }> = ({ alert, onDismiss }) => {
    const [isExiting, setIsExiting] = useState(false);

    const handleDismiss = () => {
        setIsExiting(true);
        setTimeout(onDismiss, 300);
    };

    useEffect(() => {
        if (alert.persistent) {
            return;
        }
        const timer = setTimeout(() => {
            handleDismiss();
        }, ALERT_DURATION_MS);

        return () => clearTimeout(timer);
        // eslint-disable-next-line react-hooks/exhaustive-deps -- single auto-dismiss on mount for non-persistent only
    }, [alert.persistent]);

    const getStyles = () => {
        switch (alert.category) {
            case 'green':
                return {
                    bg: 'bg-emerald-500/10 dark:bg-emerald-500/20',
                    border: 'border-emerald-500/20 dark:border-emerald-500/50',
                    text: 'text-emerald-950 dark:text-white font-semibold',
                    list: 'text-emerald-950/90 dark:text-white/95',
                    icon: <Icons.Check className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />,
                    shadow: 'shadow-[0_0_15px_rgba(16,185,129,0.1)] dark:shadow-[0_0_15px_rgba(16,185,129,0.2)]',
                };
            case 'yellow':
                return {
                    bg: 'bg-amber-500/10 dark:bg-amber-500/20',
                    border: 'border-amber-500/20 dark:border-amber-500/50',
                    text: 'text-amber-950 dark:text-white font-semibold',
                    list: 'text-amber-950/90 dark:text-white/95',
                    icon: <Icons.AlertTriangle className="w-5 h-5 text-amber-600 dark:text-amber-400" />,
                    shadow: 'shadow-[0_0_15px_rgba(245,158,11,0.1)] dark:shadow-[0_0_15px_rgba(245,158,11,0.2)]',
                };
            case 'red':
                return {
                    bg: 'bg-red-500/10 dark:bg-red-500/20',
                    border: 'border-red-500/20 dark:border-red-500/50',
                    text: 'text-red-950 dark:text-white font-semibold',
                    list: 'text-red-950/90 dark:text-red-100',
                    icon: <Icons.AlertCircle className="w-5 h-5 text-red-600 dark:text-red-400" />,
                    shadow: 'shadow-[0_0_15px_rgba(239,68,68,0.1)] dark:shadow-[0_0_15px_rgba(239,68,68,0.2)]',
                };
            default:
                return {
                    bg: 'bg-bg-card',
                    border: 'border-border-base',
                    text: 'text-text-main dark:text-white font-semibold',
                    list: 'text-text-main',
                    icon: <Icons.Info className="w-5 h-5 text-text-muted" />,
                    shadow: 'shadow-lg',
                };
        }
    };

    const style = getStyles();
    const hasLines = alert.lines && alert.lines.length > 0;

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
            <div
                className={`flex-1 min-w-0 flex flex-col gap-2 text-sm ${style.text} ${
                    hasLines ? 'max-h-[min(70vh,28rem)] overflow-y-auto pr-1' : ''
                }`}
            >
                <div className="leading-snug">{alert.message}</div>
                {hasLines && (
                    <ul
                        className={`mt-0.5 list-disc list-inside space-y-1.5 pl-0.5 text-[13px] font-normal ${style.list}`}
                    >
                        {alert.lines!.map((line, i) => (
                            <li key={`${line}-${i}`}>{line}</li>
                        ))}
                    </ul>
                )}
                {alert.applyCancel && (
                    <button
                        type="button"
                        onClick={() => FrontendWebsocket.sendCancelDecorationApply()}
                        className={`self-start rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors ${style.border} hover:bg-white/10`}
                    >
                        Cancel apply
                    </button>
                )}
            </div>
            <button
                type="button"
                onClick={handleDismiss}
                className={`
                    shrink-0 p-1 rounded-lg
                    hover:bg-white/10 transition-colors
                    ${style.text} opacity-70 hover:opacity-100
                `}
                aria-label="Dismiss"
            >
                <Icons.X className="w-4 h-4" />
            </button>
        </div>
    );
};
