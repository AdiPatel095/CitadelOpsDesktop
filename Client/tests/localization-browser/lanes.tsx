import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import './fixture.css';
import {LocaleProvider,useLocale} from '../../src/i18n/LocaleContext';
import {AutomationStatusLines,type AutomationStatusLane} from '../../src/views/AutomationView';
import {automationDetailMessage} from '../../src/i18n/automationMessages';
import {timedRemainingParameters} from '../../src/i18n/automationDuration';
import fixtures from './lane-fixtures.json';
function Fixture(){
 const {locale,setLocale,t}=useLocale();
 const [enabled,setEnabled]=useState(true);
 const [toggles,setToggles]=useState(0);
 const toggle=(value:boolean)=>{setEnabled(value);setToggles(count=>count+1);};
 const name='Player <literal>{0}';
 const lanes:AutomationStatusLane[]=[
  {id:'attacks',label:'Attacks',status:'blocked',...fixtures.cra,detailDescriptor:automationDetailMessage(fixtures.cra.detail,fixtures.cra.detailDescriptor)},
  {id:'builder',label:'Builder',status:'blocked',...fixtures.eup,detailDescriptor:automationDetailMessage(fixtures.eup.detail,fixtures.eup.detailDescriptor),toggle:{checked:enabled,onChange:toggle,ariaLabel:'Synthetic builder toggle'}},
  {id:'future_lane',label:'Future lane <literal>{0}',status:'future_status <literal>{0}',detail:'Legacy CRA90/EUP440 <literal>{0}',detailDescriptor:automationDetailMessage('Legacy CRA90/EUP440 <literal>{0}',fixtures.cra.detailDescriptor)},
 ];
 return <main className="min-h-screen bg-bg-app p-6 text-text-main"><h1>Synthetic lane lockout fixture</h1><label>Fixture language <select value={locale} onChange={event=>setLocale(event.target.value as 'en'|'de'|'ar')}><option value="en">English</option><option value="de">Deutsch</option><option value="ar">العربية</option></select></label><AutomationStatusLines featureName={name} status="blocked" lanes={lanes}/><p lang={locale} data-testid="countdown">{t('automation.timeLeft',timedRemainingParameters(90_000_000,0,locale))}</p><output data-testid="toggle-state">{JSON.stringify({enabled,toggles})}</output><p>Deadline: 2026-09-23T12:30:00Z</p></main>;
}
createRoot(document.getElementById('root')!).render(<LocaleProvider><Fixture/></LocaleProvider>);
