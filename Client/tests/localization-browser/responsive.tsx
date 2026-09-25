import './fixture.css';
import '../../src/MaterialExpressive.css';
import {createRoot} from 'react-dom/client';
// Match the production navigation selector while varying only accessible text.
createRoot(document.getElementById('root')!).render(<main data-view="world-intelligence">{[
 ['en','World Intelligence detail navigation'],['de','Navigation für Weltdetails'],['ar','التنقل في تفاصيل العالم'],
].map(([lang,label])=><nav key={lang} lang={lang} dir={lang==='ar'?'rtl':'ltr'} aria-label={label} className="world-intelligence-detail-nav sticky top-3 z-30 self-start"><button>{label}</button></nav>)}</main>);
