import type { CSSProperties } from 'react';
import type { GuidePack } from './useGuideLocale';

export type GuidePanelKind = keyof GuidePack['panels'];
const panelStyle: CSSProperties = { border: '1px solid #c8d0ea', borderRadius: 14, background: '#f8faff', color: '#202b40', padding: 16, maxWidth: 680, width: '100%', boxSizing: 'border-box' };
const cellStyle: CSSProperties = { border: '1px solid #d8def0', borderRadius: 9, background: '#fff', padding: '10px 12px', minWidth: 0, overflowWrap: 'anywhere' };

/** Static localized example: cells are text, never editable settings controls. */
export function GuideIllustration({ pack, kind, locale, large = false, alt, showAdvisor = true }: { pack: GuidePack; kind: GuidePanelKind; locale: string; large?: boolean; alt: string; showAdvisor?: boolean }) {
  const panel = pack.panels[kind];
  return <div role="group" aria-label={alt} lang={locale} dir={locale === 'ar' ? 'rtl' : 'ltr'} style={{ ...panelStyle, maxWidth: large ? 840 : 680 }}>
    <div style={{ color: '#5546ae', fontSize: 11, fontWeight: 800, letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 7 }}>{pack.ui.illustrativeExample}</div>
    <h5 style={{ color: '#344a98', fontSize: 16, fontWeight: 700, margin: '0 0 13px' }}>{panel.title}</h5>
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 180px), 1fr))', gap: 10, fontSize: 14, lineHeight: 1.45 }}>
      {Object.entries(panel).filter(([key]) => key !== 'title' && (showAdvisor || key !== 'advisor')).map(([key, value]) => <div key={key} style={cellStyle}>{value}</div>)}
    </div>
  </div>;
}
