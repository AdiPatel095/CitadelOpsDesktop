import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Check, Copy, Pause, Play, RefreshCw, Search, X } from 'lucide-react';
import { Icons } from './Icons';
import { Notifications } from './Notifications';
import { Button, Input, PillSelector, Select } from './ui';

type ChannelMeta = { id: string; label: string; description?: string };
type LogTone = 'send' | 'recv' | 'info' | 'warn' | 'error' | 'debug' | 'plain';
type LogViewMode = 'readable' | 'raw';
type LogFilter = 'all' | 'events' | 'outbound' | 'inbound' | 'issues';

type ParsedLogLine = {
  index: number;
  raw: string;
  timestamp: string;
  direction: string;
  primary: string;
  secondary: string;
  message: string;
  tone: LogTone;
  payloadParts: string[];
  jsonPayload: string;
  payloadSummary: string;
  wirePayload: string;
  searchText: string;
};

type PayloadInfo = {
  message: string;
  messageType: string;
  payloadParts: string[];
  jsonPayload: string;
  payloadSummary: string;
};

const preferredChannel = 'app_send';
const pollIntervalMs = 2000;
const tailLimit = 800;
const channelLinePattern = /^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})(?:\.(\d{1,6}))?\s+\[([^\]]+)]\s+\[([^\]]+)]\s*(.*)$/;
const goLogLinePattern = /^(\d{4})\/(\d{2})\/(\d{2})\s+(\d{2}:\d{2}:\d{2})(?:\.\d+)?\s*(.*)$/;
const scopedMessagePattern = /^\[([^\]]+)]\s*(.*)$/;

const fallbackChannelDescriptions: Record<string, string> = {
  websocket_game: 'Every game WebSocket frame, including actions performed directly in the game.',
  app_send: 'Commands sent by Citadel and the matching replies received from the game.',
  autobird: 'Auto Bird decisions, actions, and game commands.',
  autostation: 'Auto Station decisions, actions, and game commands.',
  autorecruit: 'Auto Recruit decisions, actions, and game commands.',
  autotool: 'Auto Tool decisions, actions, and game commands.',
  autosceatres: 'Auto Sceat Resources decisions, actions, and game commands.',
  autohospital: 'Auto Hospital decisions, actions, and game commands.',
  autotci: 'Auto TCI decisions, actions, and game commands.',
  autoberiworld: 'Auto Berimond World decisions, actions, and game commands.',
  autofoodbalance: 'Auto Food Balance decisions, actions, and game commands.',
  autoequipmentcleanup: 'Automatic equipment and gem cleanup commands and results.',
  autotowers: 'Auto Towers decisions, attacks, and game replies.',
  autoinvasion: 'Foreign Lords and Bloodcrow decisions, attacks, and game replies.',
  autonomad: 'Nomad and Samurai camp decisions, attacks, and game replies.',
  autokhan: 'Khan attack-chain, defense, and protection activity.',
  autostorm: 'Storm construction, logistics, combat, and shop activity.',
  rift: 'Rift decisions, actions, and game commands.',
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

const trimFrameSuffix = (value: string) => (value.endsWith('%') ? value.slice(0, -1) : value);

const formatSummaryValue = (value: unknown) => {
  if (value == null) return 'none';
  if (Array.isArray(value)) return `${value.length.toLocaleString()} ${value.length === 1 ? 'item' : 'items'}`;
  if (typeof value === 'object') {
    const count = Object.keys(value).length;
    return `${count.toLocaleString()} ${count === 1 ? 'field' : 'fields'}`;
  }
  if (typeof value === 'string') {
    const compact = value.replace(/\s+/g, ' ').trim();
    return compact.length > 46 ? `“${compact.slice(0, 43)}…”` : `“${compact}”`;
  }
  return String(value);
};

const summarizeJson = (value: unknown) => {
  if (Array.isArray(value)) {
    return `${value.length.toLocaleString()} ${value.length === 1 ? 'item' : 'items'}`;
  }
  if (value == null || typeof value !== 'object') {
    return formatSummaryValue(value);
  }

  const entries = Object.entries(value);
  if (entries.length === 0) return 'No parameters';
  const visible = entries.slice(0, 5).map(([key, entry]) => `${key}: ${formatSummaryValue(entry)}`);
  if (entries.length > visible.length) {
    visible.push(`+${entries.length - visible.length} more`);
  }
  return visible.join('  ·  ');
};

const analyzeJsonPayload = (value: string) => {
  const trimmed = value.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
    return { jsonPayload: '', payloadSummary: '' };
  }
  try {
    const parsed: unknown = JSON.parse(trimmed);
    return { jsonPayload: trimmed, payloadSummary: summarizeJson(parsed) };
  } catch {
    return { jsonPayload: '', payloadSummary: '' };
  }
};

const prettyJsonPayload = (value: string) => {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
};

const humanizeFeatureDetail = (payload: string): PayloadInfo | null => {
  const match = payload.match(/^intent=(\S+)(?:\s+operation=(\S+))?\s*(.*)$/);
  if (!match) return null;
  const [, intent, , detail] = match;
  return {
    message: detail.trim() || 'No additional details',
    messageType: '',
    payloadParts: [`Intent: ${intent}`],
    jsonPayload: '',
    payloadSummary: '',
  };
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
    throw new Error('Could not copy log data');
  }
};

const humanizePayload = (payload: string, direction: string): PayloadInfo => {
  const parts = payload.split('%');

  if (parts[1] !== 'xt') {
    const featureDetail = humanizeFeatureDetail(payload);
    if (featureDetail) return featureDetail;
    const analyzed = analyzeJsonPayload(payload);
    return {
      message: payload,
      messageType: '',
      payloadParts: [],
      ...analyzed,
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
    const analyzed = analyzeJsonPayload(payload);
    return {
      message: payload,
      messageType: '',
      payloadParts: [],
      ...analyzed,
    };
  }

  const responseStatus = direction.toUpperCase() !== 'SEND' ? (parts[sequenceIndex + 1]?.trim() ?? '') : '';
  const bodyStartIndex = sequenceIndex + (responseStatus ? 2 : 1);
  const message = trimFrameSuffix(parts.slice(bodyStartIndex).join('%')).trim() || payload;
  const analyzed = analyzeJsonPayload(message);

  return {
    message,
    messageType,
    payloadParts: responseStatus ? [responseStatus === '0' ? 'Status: OK' : `Status: ${responseStatus}`] : [],
    ...analyzed,
  };
};

const parseLogLine = (rawLine: string, index: number): ParsedLogLine => {
  const raw = rawLine.trimEnd();
  const channelMatch = raw.match(channelLinePattern);

  if (channelMatch) {
    const [, , time, , rawDirection, opcode, payload] = channelMatch;
    const direction = rawDirection.toUpperCase();
    const payloadInfo = humanizePayload(payload, direction);
    const secondary = (payloadInfo.messageType || opcode).toUpperCase();
    const semanticTone = toneFromToken(`${opcode} ${payload}`);
    const tone = direction === 'INFO' && semanticTone !== 'plain' ? semanticTone : toneFromToken(direction);
    return {
      index,
      raw,
      timestamp: formatLogTime(time),
      direction,
      primary: directionLabel(direction),
      secondary,
      message: payloadInfo.message,
      tone,
      payloadParts: payloadInfo.payloadParts,
      jsonPayload: payloadInfo.jsonPayload,
      payloadSummary: payloadInfo.payloadSummary,
      wirePayload: payload,
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
      direction: tone.toUpperCase(),
      primary,
      secondary: tone === 'plain' ? '' : directionLabel(tone),
      message,
      tone,
      payloadParts: [],
      jsonPayload: '',
      payloadSummary: '',
      wirePayload: '',
      searchText: raw.toLowerCase(),
    };
  }

  const tone = toneFromToken(raw);
  return {
    index,
    raw,
    timestamp: '',
    direction: tone.toUpperCase(),
    primary: '',
    secondary: tone === 'plain' ? '' : directionLabel(tone),
    message: raw,
    tone,
    payloadParts: [],
    jsonPayload: '',
    payloadSummary: '',
    wirePayload: '',
    searchText: raw.toLowerCase(),
  };
};

const matchesFilter = (line: ParsedLogLine, filter: LogFilter) => {
  switch (filter) {
    case 'events':
      return line.direction !== 'SEND' && line.direction !== 'RECV' && line.direction !== 'MATCH';
    case 'outbound':
      return line.direction === 'SEND';
    case 'inbound':
      return line.direction === 'RECV' || line.direction === 'MATCH';
    case 'issues':
      return line.tone === 'warn' || line.tone === 'error';
    default:
      return true;
  }
};

export const LoggerDock: React.FC = () => {
  const [open, setOpen] = useState(false);
  const [channels, setChannels] = useState<ChannelMeta[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [viewMode, setViewMode] = useState<LogViewMode>('readable');
  const [filter, setFilter] = useState<LogFilter>('all');
  const [isFollowingLive, setIsFollowingLive] = useState(true);
  const [liveUpdatesPaused, setLiveUpdatesPaused] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [expandedRows, setExpandedRows] = useState<Set<string>>(() => new Set());
  const [copiedTarget, setCopiedTarget] = useState<string | null>(null);
  const logStreamRef = useRef<HTMLDivElement>(null);
  const copyFeedbackTimerRef = useRef<number | null>(null);
  const fetchSequenceRef = useRef(0);

  const currentChannel = useMemo(
    () => channels.find((channel) => channel.id === activeId) ?? null,
    [activeId, channels],
  );
  const currentChannelDescription = currentChannel?.description
    || (activeId ? fallbackChannelDescriptions[activeId] : '')
    || 'Live application activity and command history.';
  const parsedLines = useMemo(() => lines.map(parseLogLine), [lines]);

  const filteredLines = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    return parsedLines.filter((line) => matchesFilter(line, filter) && (!query || line.searchText.includes(query)));
  }, [filter, parsedLines, searchQuery]);
  const filterCounts = useMemo(() => ({
    all: parsedLines.length,
    events: parsedLines.filter((line) => matchesFilter(line, 'events')).length,
    outbound: parsedLines.filter((line) => matchesFilter(line, 'outbound')).length,
    inbound: parsedLines.filter((line) => matchesFilter(line, 'inbound')).length,
    issues: parsedLines.filter((line) => matchesFilter(line, 'issues')).length,
  }), [parsedLines]);
  const channelOptions = useMemo(
    () => channels.map((channel) => ({ value: channel.id, label: channel.label, searchText: `${channel.label} ${channel.id}` })),
    [channels],
  );
  const filterOptions = useMemo(() => [
    { value: 'all', label: `All activity · ${filterCounts.all.toLocaleString()}` },
    { value: 'events', label: `Feature events · ${filterCounts.events.toLocaleString()}` },
    { value: 'outbound', label: `Sent commands · ${filterCounts.outbound.toLocaleString()}` },
    { value: 'inbound', label: `Replies received · ${filterCounts.inbound.toLocaleString()}` },
    { value: 'issues', label: `Warnings & errors · ${filterCounts.issues.toLocaleString()}` },
  ], [filterCounts]);
  const rowVirtualizer = useVirtualizer({
    count: filteredLines.length,
    getScrollElement: () => logStreamRef.current,
    estimateSize: () => viewMode === 'raw' ? 68 : 92,
    getItemKey: (index) => {
      const line = filteredLines[index];
      return line ? `${line.index}-${line.raw}` : index;
    },
    overscan: 8,
    measureElement: (element) => element?.getBoundingClientRect().height ?? (viewMode === 'raw' ? 68 : 92),
  });
  const liveStatus = loadError
    ? 'Updates unavailable'
    : liveUpdatesPaused
      ? 'Live updates paused'
      : `Live · every ${pollIntervalMs / 1000} seconds`;

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
      const res = await fetch(`/api/v2/telemetry/${encodeURIComponent(activeId)}?limit=${tailLimit}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { lines?: string[] };
      if (requestID !== fetchSequenceRef.current) return;
      setLines(data.lines ?? []);
      setLoadError(null);
    } catch (error) {
      if (requestID === fetchSequenceRef.current) {
        setLoadError(error instanceof Error ? error.message : 'Failed to load log');
      }
    } finally {
      if (requestID === fetchSequenceRef.current) setIsRefreshing(false);
    }
  }, [activeId]);

  useEffect(() => {
    fetchSequenceRef.current += 1;
    setLines([]);
    setExpandedRows(new Set());
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
  }, [filteredLines, open, isFollowingLive, viewMode]);

  useEffect(() => {
    if (!open) return;
    setIsFollowingLive(true);
  }, [open, activeId]);

  useEffect(() => {
    if (open) setLiveUpdatesPaused(false);
  }, [open]);

  useEffect(() => {
    rowVirtualizer.measure();
  }, [expandedRows, rowVirtualizer, viewMode]);

  useEffect(() => {
    const element = logStreamRef.current;
    if (!element || !open) return;

    const handleScroll = () => {
      const distanceFromBottom = element.scrollHeight - element.scrollTop - element.clientHeight;
      setIsFollowingLive(distanceFromBottom <= 24);
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
    setIsFollowingLive(true);
  }, []);

  const toggleRow = useCallback((rowKey: string) => {
    setExpandedRows((current) => {
      const next = new Set(current);
      if (next.has(rowKey)) {
        next.delete(rowKey);
      } else {
        next.add(rowKey);
      }
      return next;
    });
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
        <div className="liquid-logger-panel animate-fade-in" role="region" aria-label="Activity and logs">
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
                  aria-label="Close logs"
                  title="Close logs (Esc)"
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
                />
              </div>

              <div className="liquid-log-control">
                <span className="liquid-log-control-label">View</span>
                <PillSelector
                  ariaLabel="Log view"
                  value={viewMode}
                  onChange={(value) => setViewMode(value as LogViewMode)}
                  options={[
                    { value: 'readable', label: 'Readable' },
                    { value: 'raw', label: 'Raw frames' },
                  ]}
                  size="sm"
                  fullWidth
                  className="liquid-log-view-selector"
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
                />
              </div>

              <div className="liquid-log-control liquid-log-search-control">
                <span className="liquid-log-control-label">
                  Search
                  <span className="liquid-log-result-count">{filteredLines.length.toLocaleString()} of {lines.length.toLocaleString()}</span>
                </span>
                <Input
                  type="search"
                  value={searchQuery}
                  onChange={(event) => setSearchQuery(event.target.value)}
                  placeholder="Command, status, or text"
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
                  onClick={() => void copyLogData('visible-lines', filteredLines.map((line) => line.raw).join('\n'))}
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
                    : lines.length === 0
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
                    const rowKey = `${line.index}-${line.raw}`;
                    const isExpanded = expandedRows.has(rowKey);
                    const frameCopyTarget = `${rowKey}-frame`;
                    const jsonCopyTarget = `${rowKey}-json`;
                    const exactPayload = line.wirePayload || line.raw;

                    return (
                      <div
                        key={virtualRow.key}
                        ref={rowVirtualizer.measureElement}
                        data-index={virtualRow.index}
                        className="liquid-log-virtual-row"
                        style={{
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          width: '100%',
                          transform: `translateY(${virtualRow.start}px)`,
                        }}
                      >
                        {viewMode === 'raw' ? (
                          <div className={`liquid-log-raw-row liquid-log-row-${line.tone}`}>
                            <code>{line.raw}</code>
                            <button
                              type="button"
                              onClick={() => void copyLogData(`${rowKey}-raw-line`, line.raw)}
                              className="liquid-log-copy-button"
                              title="Copy exact log line"
                            >
                              {copiedTarget === `${rowKey}-raw-line` ? <Icons.Check /> : <Icons.Copy />}
                              {copiedTarget === `${rowKey}-raw-line` ? 'Copied' : 'Copy'}
                            </button>
                          </div>
                        ) : (
                          <article className={`liquid-log-row liquid-log-row-${line.tone}`}>
                            <div className="liquid-log-row-meta">
                              <span className="liquid-log-time">{line.timestamp || `#${line.index + 1}`}</span>
                              {line.primary && (
                                <span className={`liquid-log-badge liquid-log-badge-${line.tone}`}>{line.primary}</span>
                              )}
                              {line.secondary && <span className="liquid-log-chip">{line.secondary}</span>}
                              {line.payloadParts.map((part, partIndex) => (
                                <span key={`${line.index}-${partIndex}-${part}`} className="liquid-log-chip">{part}</span>
                              ))}
                            </div>

                            <div className="liquid-log-row-content">
                              <span className={`liquid-log-message ${line.payloadSummary ? 'liquid-log-message-summary' : ''}`}>
                                {line.payloadSummary || line.message || line.raw}
                              </span>
                              <div className="liquid-log-row-actions">
                                <button
                                  type="button"
                                  className={`liquid-log-details-toggle ${isExpanded ? 'liquid-log-details-toggle-open' : ''}`}
                                  onClick={() => toggleRow(rowKey)}
                                  aria-expanded={isExpanded}
                                >
                                  <Icons.ArrowRight className="liquid-log-details-icon" />
                                  {isExpanded ? 'Hide details' : 'Details'}
                                </button>
                                <button
                                  type="button"
                                  className="liquid-log-copy-button"
                                  onClick={() => void copyLogData(frameCopyTarget, exactPayload)}
                                  title={line.wirePayload ? 'Copy exact game frame' : 'Copy exact log line'}
                                >
                                  {copiedTarget === frameCopyTarget ? <Icons.Check /> : <Icons.Copy />}
                                  {copiedTarget === frameCopyTarget ? 'Copied' : line.wirePayload ? 'Copy frame' : 'Copy line'}
                                </button>
                              </div>

                              {isExpanded && (
                                <div className="liquid-log-details">
                                  {line.jsonPayload && (
                                    <section className="liquid-log-detail-section">
                                      <div className="liquid-log-detail-heading">
                                        <span>Decoded JSON payload</span>
                                        <button
                                          type="button"
                                          className="liquid-log-copy-button"
                                          onClick={() => void copyLogData(jsonCopyTarget, prettyJsonPayload(line.jsonPayload))}
                                        >
                                          {copiedTarget === jsonCopyTarget ? <Icons.Check /> : <Icons.Copy />}
                                          {copiedTarget === jsonCopyTarget ? 'Copied' : 'Copy JSON'}
                                        </button>
                                      </div>
                                      <pre className="liquid-log-json-pre custom-scrollbar">{prettyJsonPayload(line.jsonPayload)}</pre>
                                    </section>
                                  )}
                                  <section className="liquid-log-detail-section">
                                    <div className="liquid-log-detail-heading">
                                      <span>{line.wirePayload ? 'Exact game frame' : 'Original log line'}</span>
                                    </div>
                                    <pre className="liquid-log-frame-pre custom-scrollbar">{exactPayload}</pre>
                                  </section>
                                </div>
                              )}
                            </div>
                          </article>
                        )}
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
              className="liquid-glass-edge group flex h-32 w-10 shrink-0 flex-col items-center justify-center gap-2 rounded-l-[16px] border-r-0 text-text-muted transition-all duration-300 hover:border-primary/50 hover:text-primary"
              title="Show activity and game commands"
            >
              <Icons.Terminal className="h-5 w-5 group-hover:animate-pulse" />
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
