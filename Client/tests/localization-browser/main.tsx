import { createRoot } from 'react-dom/client';
import { LocaleProvider } from '../../src/i18n/LocaleContext';
import { Fixture } from './Fixture';

createRoot(document.getElementById('root')!).render(<LocaleProvider><Fixture/></LocaleProvider>);
