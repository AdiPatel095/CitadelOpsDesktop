import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Icons } from './Icons';
import { Notifications } from './Notifications';

type ChannelMeta = { id: string; label: string };
type LogTone = 'send' | 'recv' | 'info' | 'warn' | 'error' | 'debug' | 'plain';

type ParsedLogLine = {
  index: number;
  raw: string;
  timestamp: string;
  primary: string;
  secondary: string;
  message: string;
  tone: LogTone;
  payloadParts: string[];
  payloadMoreCount: number;
  jsonPayload: string;
  searchText: string;
};

const channelLinePattern = /^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})(?:\.(\d{1,6}))?\s+\[([^\]]+)]\s+\[([^\]]+)]\s*(.*)$/;
const goLogLinePattern = /^(\d{4})\/(\d{2})\/(\d{2})\s+(\d{2}:\d{2}:\d{2})(?:\.\d+)?\s*(.*)$/;
const scopedMessagePattern = /^\[([^\]]+)]\s*(.*)$/;

const formatLogTime = (time: string) => {
  const [hourText, minute, second] = time.split(':');
  const hour = Number(hourText);
  if (!Number.isInteger(hour) || hour < 0 || hour > 23 || !minute || !second) {
    return time;
  }
  const suffix = hour >= 12 ? 'PM' : 'AM';
  const hour12 = hour % 12 || 12;
  return `${hour12}:${minute}:${second} ${suffix}`;
};

const toneFromToken = (value: string): LogTone => {
  const token = value.toLowerCase();
  if (token === 'send') return 'send';
  if (token === 'recv' || token === 'receive' || token === 'received') return 'recv';
  if (token.includes('error') || token.includes('failed') || token.includes('fail')) return 'error';
  if (token.includes('warn')) return 'warn';
  if (token.includes('debug') || token.includes('trace')) return 'debug';
  if (token.includes('info') || token.includes('success') || token.includes('ready')) return 'info';
  return 'plain';
};

const trimFrameSuffix = (value: string) => (value.endsWith('%') ? value.slice(0, -1) : value);

const prettyJsonPayload = (value: string) => {
  const trimmed = value.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
    return '';
  }
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return '';
  }
};

const writeClipboardText = async (value: string) => {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // Fall through for environments where clipboard permission is unavailable.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand('copy');
  textarea.remove();
  if (!copied) {
    throw new Error('Could not copy JSON payload');
  }
};

const humanizePayload = (payload: string, direction: string) => {
  const parts = payload.split('%');

  if (parts[1] !== 'xt') {
    return {
      message: payload,
      messageType: '',
      payloadParts: [] as string[],
      payloadMoreCount: 0,
      jsonPayload: prettyJsonPayload(payload),
    };
  }

  const tokenOrType = parts[2]?.trim() ?? '';
  let messageType = tokenOrType;
  let sequenceIndex = 3;
  if (tokenOrType.startsWith('EmpireEx_')) {
    messageType = parts[3]?.trim() ?? '';
    sequenceIndex = 4;
  }

  if (!messageType) {
    return {
      message: payload,
      messageType: '',
      payloadParts: [] as string[],
      payloadMoreCount: 0,
      jsonPayload: prettyJsonPayload(payload),
    };
  }

  const responseStatus = direction.toUpperCase() !== 'SEND' ? (parts[sequenceIndex + 1]?.trim() ?? '') : '';
  const bodyStartIndex = sequenceIndex + (responseStatus ? 2 : 1);
  const message = trimFrameSuffix(parts.slice(bodyStartIndex).join('%')).trim() || payload;

  return {
    message,
    messageType,
    payloadParts: responseStatus ? [`status ${responseStatus}`] : [],
    payloadMoreCount: 0,
    jsonPayload: prettyJsonPayload(message),
  };
};

const parseLogLine = (rawLine: string, index: number): ParsedLogLine => {
  const raw = rawLine.trimEnd();
  const channelMatch = raw.match(channelLinePattern);

  if (channelMatch) {
    const [, , time, , direction, opcode, payload] = channelMatch;
    const payloadInfo = humanizePayload(payload, direction);
    const primary = direction.toUpperCase();
    const secondary = (payloadInfo.messageType || opcode).toUpperCase();
    return {
      index,
      raw,
      timestamp: formatLogTime(time),
      primary,
      secondary,
      message: payloadInfo.message,
      tone: toneFromToken(primary),
      payloadParts: payloadInfo.payloadParts,
      payloadMoreCount: payloadInfo.payloadMoreCount,
      jsonPayload: payloadInfo.jsonPayload,
      searchText: raw.toLowerCase(),
    };
  }

  const goMatch = raw.match(goLogLinePattern);
  if (goMatch) {
    const [, , , , time, rest] = goMatch;
    const scopedMatch = rest.match(scopedMessagePattern);
    const primary = scopedMatch?.[1] ?? '';
    const message = scopedMatch?.[2] ?? rest;
    const tone = toneFromToken(`${primary} ${message}`);
    return {
      index,
      raw,
      timestamp: formatLogTime(time),
      primary,
      secondary: tone === 'plain' ? '' : tone.toUpperCase(),
      message,
      tone,
      payloadParts: [],
      payloadMoreCount: 0,
      jsonPayload: '',
      searchText: raw.toLowerCase(),
    };
  }

  const tone = toneFromToken(raw);
  return {
    index,
    raw,
    timestamp: '',
    primary: '',
    secondary: tone === 'plain' ? '' : tone.toUpperCase(),
    message: raw,
    tone,
    payloadParts: [],
    payloadMoreCount: 0,
    jsonPayload: '',
    searchText: raw.toLowerCase(),
  };
};

export const LoggerDock: React.FC = () => {
  const [open, setOpen] = useState(false);
  const [channels, setChannels] = useState<ChannelMeta[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [isFollowingLive, setIsFollowingLive] = useState(true);
  const [expandedJsonRows, setExpandedJsonRows] = useState<Set<string>>(() => new Set());
  const [copiedJsonRow, setCopiedJsonRow] = useState<string | null>(null);
  const logStreamRef = useRef<HTMLDivElement>(null);
  const copyFeedbackTimerRef = useRef<number | null>(null);

  const parsedLines = useMemo(() => lines.map(parseLogLine), [lines]);

  const filteredLines = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return parsedLines;
    return parsedLines.filter((line) => line.searchText.includes(query));
  }, [parsedLines, searchQuery]);

  useEffect(() => {
    if (loadError) {
      Notifications.error(loadError, 'logger-load');
    }
  }, [loadError]);

  useEffect(() => {
	if (!open) return;
    let cancelled = false;
    (async () => {
      try {
	  const res = await fetch('/api/v2/telemetry/channels');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = (await res.json()) as { channels?: ChannelMeta[] };
        const list = data.channels ?? [];
        if (cancelled) return;
        setChannels(list);
        setActiveId((cur) =>
          cur && list.some((c) => c.id === cur) ? cur : (list[0]?.id ?? null),
        );
      } catch (e) {
        if (!cancelled) setLoadError(e instanceof Error ? e.message : 'Failed to load channels');
      }
    })();
    return () => {
      cancelled = true;
    };
	}, [open]);

  const fetchTail = useCallback(async () => {
    if (!activeId) return;
    try {
	  const res = await fetch(`/api/v2/telemetry/${encodeURIComponent(activeId)}?limit=800`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { lines?: string[] };
      setLines(data.lines ?? []);
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : 'Failed to load log');
    }
  }, [activeId]);

  useEffect(() => {
    if (!open || !activeId) return;
    void fetchTail();
    const t = window.setInterval(() => void fetchTail(), 2000);
    return () => window.clearInterval(t);
  }, [open, activeId, fetchTail]);

  useEffect(() => {
    const el = logStreamRef.current;
    if (!el) return;
    if (!isFollowingLive) return;
    el.scrollTop = el.scrollHeight;
  }, [filteredLines, open, isFollowingLive]);

  useEffect(() => {
    if (!open) return;
    setIsFollowingLive(true);
  }, [open, activeId]);

  useEffect(() => {
    const el = logStreamRef.current;
    if (!el || !open) return;

    const handleScroll = () => {
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      setIsFollowingLive(distanceFromBottom <= 24);
    };

    el.addEventListener('scroll', handleScroll);
    handleScroll();
    return () => el.removeEventListener('scroll', handleScroll);
  }, [open, filteredLines]);

  const jumpToLatest = useCallback(() => {
    const el = logStreamRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    setIsFollowingLive(true);
  }, []);

  const toggleJsonRow = useCallback((rowKey: string) => {
    setExpandedJsonRows((current) => {
      const next = new Set(current);
      if (next.has(rowKey)) {
        next.delete(rowKey);
      } else {
        next.add(rowKey);
      }
      return next;
    });
  }, []);

  const copyJsonPayload = useCallback(async (rowKey: string, jsonPayload: string) => {
    await writeClipboardText(jsonPayload);
    setCopiedJsonRow(rowKey);
    if (copyFeedbackTimerRef.current != null) {
      window.clearTimeout(copyFeedbackTimerRef.current);
    }
    copyFeedbackTimerRef.current = window.setTimeout(() => {
      setCopiedJsonRow((current) => current === rowKey ? null : current);
      copyFeedbackTimerRef.current = null;
    }, 1800);
  }, []);

  useEffect(() => () => {
    if (copyFeedbackTimerRef.current != null) {
      window.clearTimeout(copyFeedbackTimerRef.current);
    }
  }, []);

  return (
    <>
      {open && (
        <div className="liquid-logger-panel animate-fade-in">
          <div className="liquid-logger-toolbar">
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <Icons.Activity className="w-5 h-5 text-primary" />
                <span className="text-sm font-bold uppercase text-text-main">
                  System Logs
                </span>
              </div>
              <div className="h-4 w-px bg-border-base mx-2"></div>
              <div className="flex flex-wrap gap-1.5">
                {channels.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => setActiveId(c.id)}
                    className={`liquid-log-tab ${
                      activeId === c.id
                        ? 'liquid-log-tab-active'
                        : 'liquid-log-tab-idle'
                    }`}
                  >
                    {c.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex items-center gap-2">
              <div className="relative">
                <Icons.List className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" />
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Search logs..."
                  className="liquid-log-search"
                />
              </div>
              {!isFollowingLive && (
                <button
                  type="button"
                  onClick={jumpToLatest}
                  className="flex items-center gap-1.5 rounded-full border border-primary/30 bg-primary/10 px-3 py-2 text-sm font-medium text-primary transition-colors hover:bg-primary/15"
                >
                  <Icons.ArrowRight className="w-4 h-4 rotate-90" />
                  Jump to latest
                </button>
              )}
              <button
                type="button"
                onClick={() => void fetchTail()}
                className="liquid-glass-edge flex items-center gap-1.5 rounded-full px-3 py-2 text-sm font-medium text-text-muted transition-colors hover:text-text-main"
              >
                <Icons.RefreshCw className="w-4 h-4" />
                Refresh
              </button>
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="liquid-glass-edge flex items-center gap-1.5 rounded-full px-3 py-2 text-sm font-medium text-text-muted transition-colors hover:border-error/30 hover:text-error"
              >
                <Icons.X className="w-4 h-4" />
                Close
              </button>
            </div>
          </div>

          <div className="liquid-logger-body">
            <div
              ref={logStreamRef}
              className="liquid-log-stream custom-scrollbar"
              role="log"
            >
              {filteredLines.length === 0 ? (
                <span className="liquid-log-empty">
                  {lines.length === 0 ? 'No lines yet for this channel.' : 'No log lines match the current search.'}
                </span>
              ) : (
                filteredLines.map((line) => {
                  const rowKey = `${line.index}-${line.raw}`;
                  const isJsonExpanded = expandedJsonRows.has(rowKey);

                  return (
                    <div
                      key={rowKey}
                      className={`liquid-log-row liquid-log-row-${line.tone}`}
                      title={line.raw}
                    >
                      <div className="liquid-log-row-meta">
                        <span className="liquid-log-time">
                          {line.timestamp || `#${line.index + 1}`}
                        </span>
                        {line.primary && (
                          <span className={`liquid-log-badge liquid-log-badge-${line.tone}`}>
                            {line.primary}
                          </span>
                        )}
                        {line.secondary && (
                          <span className="liquid-log-chip">
                            {line.secondary}
                          </span>
                        )}
                        {line.payloadParts.map((part, partIndex) => (
                          <span key={`${line.index}-${partIndex}-${part}`} className="liquid-log-chip">
                            {part}
                          </span>
                        ))}
                        {line.payloadMoreCount > 0 && (
                          <span className="liquid-log-chip">
                            +{line.payloadMoreCount}
                          </span>
                        )}
                      </div>
                      <div className="liquid-log-row-content">
                        {line.jsonPayload ? (
                          <div className="liquid-log-json">
                            <div className="liquid-log-json-actions">
                              <button
                                type="button"
                                className={`liquid-log-json-toggle ${isJsonExpanded ? 'liquid-log-json-toggle-open' : ''}`}
                                onClick={() => toggleJsonRow(rowKey)}
                                aria-expanded={isJsonExpanded}
                              >
                                <Icons.ArrowRight className="liquid-log-json-icon" />
                                JSON payload
                              </button>
                              <button
                                type="button"
                                className="liquid-log-json-copy"
                                onClick={() => void copyJsonPayload(rowKey, line.jsonPayload)}
                                aria-label="Copy JSON payload"
                                title="Copy JSON payload"
                              >
                                {copiedJsonRow === rowKey ? (
                                  <>
                                    <Icons.Check className="liquid-log-json-copy-icon" />
                                    Copied
                                  </>
                                ) : (
                                  <>
                                    <Icons.Copy className="liquid-log-json-copy-icon" />
                                    Copy
                                  </>
                                )}
                              </button>
                            </div>
                            {isJsonExpanded && (
                              <pre className="liquid-log-json-pre custom-scrollbar">
                                {line.jsonPayload}
                              </pre>
                            )}
                          </div>
                        ) : (
                          <span className="liquid-log-message">{line.message || line.raw}</span>
                        )}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>
      )}

      {!open && (
        <div className="liquid-log-handle" aria-live="polite">
          <div className="pointer-events-auto flex flex-row items-stretch shadow-2xl">
            <button
              type="button"
              onClick={() => setOpen(true)}
              className="liquid-glass-edge group flex h-32 w-10 shrink-0 flex-col items-center justify-center gap-2 rounded-l-[16px] border-r-0 text-text-muted transition-all duration-300 hover:border-primary/50 hover:text-primary"
              title="Show logs"
            >
              <Icons.Terminal className="w-5 h-5 group-hover:animate-pulse" />
              <span
                className="text-xs font-bold uppercase text-text-muted transition-colors group-hover:text-primary"
                style={{ writingMode: 'vertical-rl', transform: 'rotate(180deg)' }}
              >
                Logs
              </span>
            </button>
          </div>
        </div>
      )}
    </>
  );
};
