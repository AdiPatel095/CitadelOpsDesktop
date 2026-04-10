import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Icons } from './Icons';

type ChannelMeta = { id: string; label: string };

export const LoggerDock: React.FC = () => {
  const [open, setOpen] = useState(false);
  const [channels, setChannels] = useState<ChannelMeta[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [isFollowingLive, setIsFollowingLive] = useState(true);
  const preRef = useRef<HTMLPreElement>(null);

  const filteredLines = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return lines;
    return lines.filter((line) => line.toLowerCase().includes(query));
  }, [lines, searchQuery]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch('/api/logs/channels');
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
  }, []);

  const fetchTail = useCallback(async () => {
    if (!activeId) return;
    try {
      const res = await fetch(`/api/logs/${encodeURIComponent(activeId)}/tail?n=800`);
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
    const el = preRef.current;
    if (!el) return;
    if (!isFollowingLive) return;
    el.scrollTop = el.scrollHeight;
  }, [filteredLines, open, isFollowingLive]);

  useEffect(() => {
    if (!open) return;
    setIsFollowingLive(true);
  }, [open, activeId]);

  useEffect(() => {
    const el = preRef.current;
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
    const el = preRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    setIsFollowingLive(true);
  }, []);

  return (
    <>
      {open && (
        <div className="fixed top-16 left-64 right-0 bottom-0 z-[90] bg-bg-app border-l border-t border-border-base shadow-2xl flex flex-col animate-fade-in">
          <div className="flex shrink-0 items-center justify-between border-b border-border-base bg-bg-card-hover/80 px-4 py-3 backdrop-blur-md">
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <Icons.Activity className="w-4 h-4 text-primary" />
                <span className="text-xs font-bold uppercase tracking-wide text-text-main">
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
                    className={`rounded-global px-3 py-1 text-[11px] font-bold uppercase tracking-wider transition-colors ${
                      activeId === c.id
                        ? 'bg-primary text-bg-app shadow-md shadow-primary/20'
                        : 'bg-bg-card border border-border-base text-text-muted hover:bg-primary/10 hover:border-primary/30 hover:text-primary'
                    }`}
                  >
                    {c.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex items-center gap-2">
              <div className="relative">
                <Icons.List className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-text-muted" />
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Search logs..."
                  className="w-48 rounded-global border border-border-base bg-bg-card py-1.5 pl-8 pr-3 text-xs text-text-main placeholder:text-text-muted focus:border-primary focus:outline-none"
                />
              </div>
              {!isFollowingLive && (
                <button
                  type="button"
                  onClick={jumpToLatest}
                  className="rounded-global border border-primary/30 bg-primary/10 px-3 py-1.5 text-xs font-medium text-primary hover:bg-primary/15 transition-colors flex items-center gap-1.5"
                >
                  <Icons.ArrowRight className="w-3.5 h-3.5 rotate-90" />
                  Jump to latest
                </button>
              )}
              <button
                type="button"
                onClick={() => void fetchTail()}
                className="rounded-global border border-border-base bg-bg-card px-3 py-1.5 text-xs font-medium text-text-muted hover:bg-bg-input hover:text-text-main transition-colors flex items-center gap-1.5"
              >
                <Icons.RefreshCw className="w-3.5 h-3.5" />
                Refresh
              </button>
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="rounded-global border border-border-base bg-bg-card px-3 py-1.5 text-xs font-medium text-text-muted hover:bg-error/10 hover:border-error/30 hover:text-error transition-colors flex items-center gap-1.5"
              >
                <Icons.X className="w-3.5 h-3.5" />
                Close
              </button>
            </div>
          </div>

          <div className="relative min-h-0 flex-1 overflow-hidden bg-[#0A0A0A]">
            {loadError && (
              <div className="absolute inset-x-0 top-0 z-10 bg-error/15 border-b border-error/30 px-3 py-2 text-center text-xs font-medium text-error shadow-sm backdrop-blur-sm flex items-center justify-center gap-2">
                <Icons.AlertCircle className="w-4 h-4" />
                {loadError}
              </div>
            )}
            <pre
              ref={preRef}
              className="h-full overflow-auto p-4 font-mono text-[11px] leading-relaxed text-gray-300 selection:bg-primary/30 custom-scrollbar"
            >
              {filteredLines.length === 0 ? (
                <span className="text-gray-600 italic">
                  {lines.length === 0 ? 'No lines yet for this channel.' : 'No log lines match the current search.'}
                </span>
              ) : (
                filteredLines.join('\n')
              )}
            </pre>
          </div>
        </div>
      )}

      {!open && (
        <div
          className="fixed bottom-0 right-0 z-[100] flex flex-row items-end pointer-events-none p-0"
          aria-live="polite"
        >
          <div className="pointer-events-auto flex flex-row items-stretch shadow-2xl">
            <button
              type="button"
              onClick={() => setOpen(true)}
              className="flex h-32 w-10 shrink-0 flex-col items-center justify-center gap-2 rounded-tl-xl border border-border-base border-r-0 border-b-0 bg-bg-card text-text-muted shadow-lg transition-all duration-300 hover:bg-bg-card-hover hover:text-primary hover:border-primary/50 group"
              title="Show logs"
            >
              <Icons.Terminal className="w-4 h-4 group-hover:animate-pulse" />
              <span
                className="text-[10px] font-bold uppercase tracking-widest text-text-muted group-hover:text-primary transition-colors"
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
