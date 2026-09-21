import {LoggerDock} from '../../src/components/LoggerDock';
import {useEffect,useState} from 'react';
import {createRoot} from 'react-dom/client';
import './fixture.css';
import {LocaleProvider,useLocale} from '../../src/i18n/LocaleContext';
import {locales} from '../../src/i18n/locales';
import {describeMessage} from '../../src/i18n/messages';
import {Alerts} from '../../src/components/Alerts';
import {Notifications} from '../../src/components/Notifications';
import {EquipmentSellModal,EquipmentSwapModal} from '../../src/equipment/components/EquipmentModals';
const leader={id:1,kind:'commander' as const,name:'Player {0} <b>literal</b>',position:1,available:true,equipment:{'1':101},gems:{}};
function Fixture(){
 const {locale,setLocale}=useLocale();const [modal,setModal]=useState('');const [action,setAction]=useState('none');
 useEffect(()=>{const onKey=(event:KeyboardEvent)=>{if(!event.altKey)return;const selected=({'1':'en','2':'de','3':'ar'} as const)[event.key as '1'|'2'|'3'];if(selected){event.preventDefault();setLocale(selected);}};window.addEventListener('keydown',onKey);return()=>window.removeEventListener('keydown',onKey);},[setLocale]);
 return <main className="min-h-screen bg-bg-app p-6 text-text-main"><h1>Localization fixture — synthetic data only</h1><p>Open-modal language shortcuts: Alt+1 English, Alt+2 Deutsch, Alt+3 العربية</p><label>Fixture language <select value={locale} onChange={event=>setLocale(event.target.value as typeof locale)}>{locales.map(item=><option key={item.code} value={item.code}>{item.nativeName}</option>)}</select></label><div className="flex gap-4 py-6"><button onClick={()=>setModal('swap')}>Open swap fixture</button><button onClick={()=>setModal('sell')}>Open sell fixture</button><button onClick={()=>Notifications.publish({id:'fixture-notice',category:'green',message:'Equipment loadouts swapped',messageDescriptor:describeMessage('equipment.notification.swapped'),persistent:true})}>Show persistent notification</button></div><fieldset className="flex gap-4" aria-label="Native browser radio reference">{[1,2,3].map(value=><label key={value}><input type="radio" name="native-reference" defaultChecked={value===2}/>Native {value}</label>)}</fieldset><output>Captured fixture action: {action}</output><EquipmentSwapModal isOpen={modal==='swap'} leader={leader} leaders={[leader,{...leader,id:2,name:'Another player',position:2}]} onClose={()=>setModal('')} onConfirm={id=>setAction(String(id))} busy={false}/><EquipmentSellModal isOpen={modal==='sell'} itemType="Equipment" onClose={()=>setModal('')} onConfirm={request=>setAction(JSON.stringify(request))} busy={false}/><Alerts/><LoggerDock/></main>;
}
createRoot(document.getElementById('root')!).render(<LocaleProvider><Fixture/></LocaleProvider>);
