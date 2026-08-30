import React, { useCallback, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Check, Copy, Pause, Play, RefreshCw, Search, X } from 'lucide-react';
import { Icons } from './Icons';
import { Notifications } from './Notifications';
import { Button, Input, Select } from './ui';
import { runtimeFetch } from '../api/RuntimeURL';

type ChannelMeta = { id: string; label: string; description?: string };
type LogTone = 'send' | 'recv' | 'info' | 'warn' | 'error' | 'debug' | 'plain';
type LogFilter = 'all' | 'actions' | 'issues';

type LogLineEntry = {
  id: string;
  raw: string;
};

type LogTailState = {
  entries: LogLineEntry[];
  nextID: number;
};

type LogTailAction =
  | { type: 'replace'; lines: string[] }
  | { type: 'clear' };

type ParsedLogLine = {
  id: string;
  index: number;
  raw: string;
  timestamp: string;
  direction: string;
  primary: string;
  secondary: string;
  message: string;
  tone: LogTone;
  searchText: string;
};

const preferredChannel = 'activity';
const pollIntervalMs = 2000;
const tailLimit = 800;
const channelLinePattern = /^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})(?:\.(\d{1,6}))?\s+\[([^\]]+)]\s+\[([^\]]+)]\s*(.*)$/;

const logTailReducer = (state: LogTailState, action: LogTailAction): LogTailState => {
  if (action.type === 'clear') {
    return state.entries.length === 0 ? state : { ...state, entries: [] };
  }

  if (
    state.entries.length === action.lines.length
    && state.entries.every((entry, index) => entry.raw === action.lines[index])
  ) {
    return state;
  }

  const availableIDs = new Map<string, { ids: string[]; index: number }>();
  state.entries.forEach((entry) => {
    const available = availableIDs.get(entry.raw);
    if (available) {
      available.ids.push(entry.id);
    } else {
      availableIDs.set(entry.raw, { ids: [entry.id], index: 0 });
    }
  });

  let nextID = state.nextID;
  const entries = action.lines.map((raw) => {
    const available = availableIDs.get(raw);
    if (available && available.index < available.ids.length) {
      const id = available.ids[available.index];
      available.index += 1;
      return { id, raw };
    }

    nextID += 1;
    return { id: `log-line-${nextID}`, raw };
  });

  return { entries, nextID };
};

const fallbackChannelDescriptions: Record<string, string> = {
  activity: 'Completed Citadel actions and issues that may need your attention.',
  autobird: 'Completed Auto Bird troop movements and problems requiring attention.',
  autostation: 'Completed station and recall movements and problems requiring attention.',
  autorecruit: 'Completed troop queues and problems requiring attention.',
  autotool: 'Completed tool queues and problems requiring attention.',
  autosceatres: 'Completed crafting and resource actions and problems requiring attention.',
  autohospital: 'Completed hospital actions and problems requiring attention.',
  autotci: 'Completed construction-item equips, upgrades, and purchases and problems requiring attention.',
  autoberiworld: 'Completed Berimond troop transfers, tower attacks, and problems requiring attention.',
  autofoodbalance: 'Completed food and mead shipments and problems requiring attention.',
  autoequipmentcleanup: 'Completed equipment cleanup actions and problems requiring attention.',
  autotowers: 'Launched tower attacks and problems requiring attention.',
  autoinvasion: 'Launched Foreign Lords and Bloodcrow attacks and problems requiring attention.',
  autonomad: 'Launched Nomad and Samurai attacks and other completed event actions.',
  autoadvisor: 'Launched advisor attacks and other completed advisor actions.',
  autokhan: 'Completed Khan attacks, defense, and protection actions.',
  autostorm: 'Completed Storm attacks, construction, logistics, and shop purchases.',
  rift: 'Launched Rift attacks and other completed Rift actions.',
};

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

const directionLabel = (value: string) => {
  switch (value.trim().toUpperCase()) {
    case 'SEND':
      return 'Sent';
    case 'RECV':
      return 'Received';
    case 'MATCH':
      return 'Reply';
    case 'INFO':
      return 'Event';
    case 'WARN':
    case 'WARNING':
      return 'Warning';
    case 'ERROR':
      return 'Error';
    case 'DEBUG':
    case 'TRACE':
      return 'Debug';
    default:
      return value;
  }
};

const toneFromToken = (value: string): LogTone => {
  const token = value.toLowerCase();
  if (token === 'send') return 'send';
  if (token === 'recv' || token === 'receive' || token === 'received' || token === 'match') return 'recv';
  if (token.includes('error') || token.includes('fail') || token.includes('reject') || token.includes('block') || token.includes('denied')) return 'error';
  if (token.includes('warn') || token.includes('cancel') || token.includes('skip') || token.includes('expire')) return 'warn';
  if (token.includes('debug') || token.includes('trace')) return 'debug';
  if (token.includes('info') || token.includes('success') || token.includes('ready')) return 'info';
  return 'plain';
};

const activityEventLabel = (value: string) => value
  .trim()
  .toLowerCase()
  .replace(/\b\w/g, (letter) => letter.toUpperCase());

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
    throw new Error('Could not copy log data');
  }
};

const parseLogLine = (entry: LogLineEntry, index: number): ParsedLogLine => {
  const rawLine = entry.raw;
  const raw = rawLine.trimEnd();
  const channelMatch = raw.match(channelLinePattern);

  if (channelMatch) {
    const [, , time, , rawDirection, event, message] = channelMatch;
    const direction = rawDirection.toUpperCase();
    const tone = toneFromToken(direction);
    return {
      id: entry.id,
      index,
      raw,
      timestamp: formatLogTime(time),
      direction,
      primary: directionLabel(direction),
      secondary: activityEventLabel(event),
      message: message.trim() || 'Citadel completed an action.',
      tone,
      searchText: `${event} ${message}`.toLowerCase(),
    };
  }

  return {
    id: entry.id,
    index,
    raw,
    timestamp: '',
    direction: 'PLAIN',
    primary: '',
    secondary: 'Activity',
    message: 'This activity entry is unavailable.',
    tone: 'plain',
    searchText: '',
  };
};

const matchesFilter = (line: ParsedLogLine, filter: LogFilter) => {
  switch (filter) {
    case 'actions':
      return line.tone !== 'warn' && line.tone !== 'error';
    case 'issues':
      return line.tone === 'warn' || line.tone === 'error';
    default:
      return true;
  }
};

export const LoggerDock = React.memo(function LoggerDock() {
  const [open, setOpen] = useState(false);
  const [channels, setChannels] = useState<ChannelMeta[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [logTail, dispatchLogTail] = useReducer(logTailReducer, { entries: [], nextID: 0 });
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [filter, setFilter] = useState<LogFilter>('all');
  const [isFollowingLive, setIsFollowingLive] = useState(true);
  const [liveUpdatesPaused, setLiveUpdatesPaused] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [copiedTarget, setCopiedTarget] = useState<string | null>(null);
  const logStreamRef = useRef<HTMLDivElement>(null);
  const copyFeedbackTimerRef = useRef<number | null>(null);
  const fetchSequenceRef = useRef(0);
  const logEntriesRef = useRef(logTail.entries);
  const isFollowingLiveRef = useRef(isFollowingLive);
  const pendingScrollAnchorRef = useRef<{ lineID: string; offset: number } | null>(null);

  const currentChannel = useMemo(
    () => channels.find((channel) => channel.id === activeId) ?? null,
    [activeId, channels],
  );
  const currentChannelDescription = currentChannel?.description
    || (activeId ? fallbackChannelDescriptions[activeId] : '')
    || 'Live application activity and command history.';
  const parsedLines = useMemo(() => logTail.entries.map(parseLogLine), [logTail.entries]);

  const filteredLines = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    return parsedLines.filter((line) => matchesFilter(line, filter) && (!query || line.searchText.includes(query)));
  }, [filter, parsedLines, searchQuery]);
  const filterCounts = useMemo(() => ({
    all: parsedLines.length,
    actions: parsedLines.filter((line) => matchesFilter(line, 'actions')).length,
    issues: parsedLines.filter((line) => matchesFilter(line, 'issues')).length,
  }), [parsedLines]);
  const channelOptions = useMemo(
    () => channels.map((channel) => ({ value: channel.id, label: channel.label, searchText: `${channel.label} ${channel.id}` })),
    [channels],
  );
  const filterOptions = useMemo(() => [
    { value: 'all', label: `All activity · ${filterCounts.all.toLocaleString()}` },
    { value: 'actions', label: `Completed actions · ${filterCounts.actions.toLocaleString()}` },
    { value: 'issues', label: `Warnings & errors · ${filterCounts.issues.toLocaleString()}` },
  ], [filterCounts]);
  const rowVirtualizer = useVirtualizer({
    count: filteredLines.length,
    getScrollElement: () => logStreamRef.current,
    estimateSize: () => 82,
    getItemKey: (index) => {
      const line = filteredLines[index];
      return line?.id ?? index;
    },
    overscan: 8,
    measureElement: (element) => element?.getBoundingClientRect().height ?? 82,
  });
  const filteredLinesRef = useRef(filteredLines);
  const rowVirtualizerRef = useRef(rowVirtualizer);
  const liveStatus = loadError
    ? 'Updates unavailable'
    : liveUpdatesPaused
      ? 'Live updates paused'
      : `Live · every ${pollIntervalMs / 1000} seconds`;

  logEntriesRef.current = logTail.entries;
  isFollowingLiveRef.current = isFollowingLive;
  filteredLinesRef.current = filteredLines;
  rowVirtualizerRef.current = rowVirtualizer;

  const captureScrollAnchor = useCallback(() => {
    const element = logStreamRef.current;
    if (!element || isFollowingLiveRef.current) {
      pendingScrollAnchorRef.current = null;
      return;
    }

    const scrollTop = element.scrollTop;
    const anchorRow = rowVirtualizerRef.current.getVirtualItems().find((row) => row.end > scrollTop);
    const anchorLine = anchorRow ? filteredLinesRef.current[anchorRow.index] : null;
    pendingScrollAnchorRef.current = anchorRow && anchorLine
      ? { lineID: anchorLine.id, offset: anchorRow.start - scrollTop }
      : null;
  }, []);

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
        const res = await runtimeFetch('/api/v2/telemetry/channels');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = (await res.json()) as { channels?: ChannelMeta[] };
        const list = data.channels ?? [];
        if (cancelled) return;
        setChannels(list);
        setLoadError(null);
        setActiveId((current) => {
          if (current && list.some((channel) => channel.id === current)) return current;
          return list.find((channel) => channel.id === preferredChannel)?.id ?? list[0]?.id ?? null;
        });
      } catch (error) {
        if (!cancelled) setLoadError(error instanceof Error ? error.message : 'Failed to load channels');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open]);

  const fetchTail = useCallback(async (showProgress = false) => {
    if (!activeId) return;
    const requestID = ++fetchSequenceRef.current;
    if (showProgress) setIsRefreshing(true);
    try {
      const res = await runtimeFetch(`/api/v2/telemetry/${encodeURIComponent(activeId)}?limit=${tailLimit}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { lines?: string[] };
      if (requestID !== fetchSequenceRef.current) return;
      const nextLines = data.lines ?? [];
      const currentEntries = logEntriesRef.current;
      const changed = currentEntries.length !== nextLines.length
        || currentEntries.some((entry, index) => entry.raw !== nextLines[index]);
      if (changed) {
        captureScrollAnchor();
        dispatchLogTail({ type: 'replace', lines: nextLines });
      }
      setLoadError(null);
    } catch (error) {
      if (requestID === fetchSequenceRef.current) {
        setLoadError(error instanceof Error ? error.message : 'Failed to load log');
      }
    } finally {
      if (requestID === fetchSequenceRef.current) setIsRefreshing(false);
    }
  }, [activeId, captureScrollAnchor]);

  useEffect(() => {
    fetchSequenceRef.current += 1;
    pendingScrollAnchorRef.current = null;
    dispatchLogTail({ type: 'clear' });
  }, [activeId]);

  useEffect(() => {
    if (!open || !activeId) return;
    void fetchTail();
  }, [open, activeId, fetchTail]);

  useEffect(() => {
    if (!open || !activeId || liveUpdatesPaused) return;
    const interval = window.setInterval(() => void fetchTail(), pollIntervalMs);
    return () => window.clearInterval(interval);
  }, [open, activeId, fetchTail, liveUpdatesPaused]);

  useEffect(() => {
    const element = logStreamRef.current;
    if (!element || !isFollowingLive) return;
    element.scrollTop = element.scrollHeight;
  }, [filteredLines, open, isFollowingLive]);

  useLayoutEffect(() => {
    const anchor = pendingScrollAnchorRef.current;
    pendingScrollAnchorRef.current = null;
    const element = logStreamRef.current;
    if (!anchor || !element || isFollowingLiveRef.current) return;

    const anchorIndex = filteredLines.findIndex((line) => line.id === anchor.lineID);
    if (anchorIndex < 0) return;
    const offset = rowVirtualizer.getOffsetForIndex(anchorIndex, 'start');
    if (!offset) return;
    element.scrollTop = Math.max(0, offset[0] - anchor.offset);
  }, [filteredLines, rowVirtualizer]);

  useEffect(() => {
    if (!open) return;
    isFollowingLiveRef.current = true;
    setIsFollowingLive(true);
  }, [open, activeId]);

  useEffect(() => {
    if (open) setLiveUpdatesPaused(false);
  }, [open]);

  useEffect(() => {
    const element = logStreamRef.current;
    if (!element || !open) return;

    const handleScroll = () => {
      const distanceFromBottom = element.scrollHeight - element.scrollTop - element.clientHeight;
      const isFollowing = distanceFromBottom <= 24;
      isFollowingLiveRef.current = isFollowing;
      setIsFollowingLive(isFollowing);
    };

    element.addEventListener('scroll', handleScroll);
    handleScroll();
    return () => element.removeEventListener('scroll', handleScroll);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open]);

  const jumpToLatest = useCallback(() => {
    const element = logStreamRef.current;
    if (!element) return;
    element.scrollTop = element.scrollHeight;
    isFollowingLiveRef.current = true;
    setIsFollowingLive(true);
  }, []);

  const copyLogData = useCallback(async (target: string, value: string) => {
    try {
      await writeClipboardText(value);
      setCopiedTarget(target);
      if (copyFeedbackTimerRef.current != null) {
        window.clearTimeout(copyFeedbackTimerRef.current);
      }
      copyFeedbackTimerRef.current = window.setTimeout(() => {
        setCopiedTarget((current) => current === target ? null : current);
        copyFeedbackTimerRef.current = null;
      }, 1800);
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not copy log data', 'logger-copy');
    }
  }, []);

  useEffect(() => () => {
    if (copyFeedbackTimerRef.current != null) {
      window.clearTimeout(copyFeedbackTimerRef.current);
    }
  }, []);

  return (
    <>
      {open && (
        <div className="liquid-logger-panel animate-fade-in" role="region" aria-label="Citadel activity">
          <div className="liquid-logger-toolbar">
            <div className="liquid-logger-toolbar-main">
              <div className="liquid-log-heading">
                <span className="liquid-log-heading-icon">
                  <Icons.Activity className="h-5 w-5" />
                </span>
                <span className="min-w-0">
                  <span className="liquid-log-title">Live activity</span>
                  <span className="liquid-log-subtitle">{currentChannelDescription}</span>
                </span>
              </div>
              <div className="liquid-log-header-actions">
                <span className={`liquid-log-live-status ${loadError ? 'liquid-log-live-status-error' : liveUpdatesPaused ? 'liquid-log-live-status-paused' : ''}`}>
                  <span className="liquid-log-live-dot" />
                  {liveStatus}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => setOpen(false)}
                  className="liquid-log-close-button"
                  aria-label="Close activity"
                  title="Close activity (Esc)"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </div>

            <div className="liquid-logger-controls">
              <div className="liquid-log-control liquid-log-channel-control">
                <span className="liquid-log-control-label">Channel</span>
                <Select
                  value={activeId ?? ''}
                  onChange={setActiveId}
                  options={channelOptions}
                  placeholder="No channels available"
                  className="liquid-log-shared-select"
                  ariaLabel="Log channel"
                  searchable
                  searchPlaceholder="Find a feature or channel"
                  menuGrowToViewport
                  closeOnScroll={false}
                />
              </div>

              <div className="liquid-log-control liquid-log-filter-control">
                <span className="liquid-log-control-label">Show</span>
                <Select
                  value={filter}
                  onChange={(value) => setFilter(value as LogFilter)}
                  options={filterOptions}
                  className="liquid-log-shared-select"
                  ariaLabel="Filter log activity"
                  closeOnScroll={false}
                />
              </div>

              <div className="liquid-log-control liquid-log-search-control">
                <span className="liquid-log-control-label">
                  Search
                  <span className="liquid-log-result-count">{filteredLines.length.toLocaleString()} of {logTail.entries.length.toLocaleString()}</span>
                </span>
                <Input
                  type="search"
                  value={searchQuery}
                  onChange={(event) => setSearchQuery(event.target.value)}
                  placeholder="Action or issue"
                  className="liquid-log-search-input"
                  leftIcon={<Search className="h-4 w-4" />}
                  aria-label="Search log activity"
                />
              </div>

              <div className="liquid-log-toolbar-buttons">
                {!isFollowingLive && (
                  <Button type="button" variant="outline" size="sm" onClick={jumpToLatest} leftIcon={<Icons.ArrowRight className="h-4 w-4 rotate-90" />}>
                    Latest
                  </Button>
                )}
                <Button
                  type="button"
                  variant={liveUpdatesPaused ? 'outline' : 'ghost'}
                  size="icon"
                  className="liquid-log-toolbar-icon-button"
                  onClick={() => {
                    if (liveUpdatesPaused) void fetchTail();
                    setLiveUpdatesPaused((current) => !current);
                  }}
                  aria-label={liveUpdatesPaused ? 'Resume live log updates' : 'Pause live log updates'}
                  title={liveUpdatesPaused ? 'Resume live updates' : 'Pause live updates'}
                >
                  {liveUpdatesPaused ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="liquid-log-toolbar-icon-button"
                  onClick={() => void copyLogData(
                    'visible-lines',
                    filteredLines.map((line) => [line.timestamp, line.secondary, line.message].filter(Boolean).join(' · ')).join('\n'),
                  )}
                  disabled={filteredLines.length === 0}
                  aria-label="Copy visible log lines"
                  title={`Copy ${filteredLines.length.toLocaleString()} visible log lines`}
                >
                  {copiedTarget === 'visible-lines' ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  size="icon"
                  className="liquid-log-toolbar-icon-button"
                  onClick={() => void fetchTail(true)}
                  disabled={isRefreshing}
                  aria-label="Refresh logs"
                  title="Refresh now"
                >
                  <RefreshCw className={`h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`} />
                </Button>
              </div>
            </div>
          </div>

          <div className="liquid-logger-body">
            <div
              ref={logStreamRef}
              className="liquid-log-stream custom-scrollbar"
              role="log"
              aria-label={currentChannel ? `${currentChannel.label} activity` : 'Log activity'}
              aria-busy={isRefreshing}
            >
              {filteredLines.length === 0 ? (
                <span className="liquid-log-empty">
                  {loadError
                    ? 'Activity is temporarily unavailable. Use Refresh to try again.'
                    : logTail.entries.length === 0
                      ? 'No activity yet for this channel.'
                      : 'No activity matches the current search and filter.'}
                </span>
              ) : (
                <div
                  className="liquid-log-virtual-list"
                  style={{ height: `${rowVirtualizer.getTotalSize()}px` }}
                >
                  {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                    const line = filteredLines[virtualRow.index];
                    if (!line) return null;

                    return (
                      <div
                        key={virtualRow.key}
                        ref={rowVirtualizer.measureElement}
                        data-index={virtualRow.index}
                        data-log-line-id={line.id}
                        className="liquid-log-virtual-row"
                        style={{
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          width: '100%',
                          transform: `translateY(${virtualRow.start}px)`,
                        }}
                      >
                        <article className={`liquid-log-row liquid-log-row-${line.tone}`}>
                          <div className="liquid-log-row-meta">
                            <span className="liquid-log-time">{line.timestamp || `#${line.index + 1}`}</span>
                            {line.primary && (
                              <span className={`liquid-log-badge liquid-log-badge-${line.tone}`}>{line.primary}</span>
                            )}
                            {line.secondary && <span className="liquid-log-chip">{line.secondary}</span>}
                          </div>

                          <div className="liquid-log-row-content">
                            <span className="liquid-log-message">
                              {line.message || 'Citadel completed an action.'}
                            </span>
                          </div>
                        </article>
                      </div>
                    );
                  })}
                </div>
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
              className="liquid-surface-edge group flex h-32 w-10 shrink-0 flex-col items-center justify-center gap-2 rounded-l-[16px] border-r-0 text-text-muted transition-all duration-300 hover:border-primary/50 hover:text-primary"
              title="Show Citadel activity"
            >
              <Icons.Activity className="h-5 w-5 group-hover:animate-pulse" />
              <span
                className="text-xs font-bold uppercase text-text-muted transition-colors group-hover:text-primary"
                style={{ writingMode: 'vertical-rl', transform: 'rotate(180deg)' }}
              >
                Activity
              </span>
            </button>
          </div>
        </div>
      )}
    </>
  );
});
