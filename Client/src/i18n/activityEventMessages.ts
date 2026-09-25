import type {MessageKey} from './messages';
/** Server/Telemetry/Store.go featureActivityEvents at e9e6e2a. Protocol identities stay unchanged. */
const eventKeys:Readonly<Record<string,MessageKey>>={
 ACTION:'activity.rowAction','ALLIANCE HELP':'activity.rowAllianceHelp',ATTACK:'game.attack',BUILDING:'activity.rowBuilding',CONSTRUCTION:'activity.rowConstruction',CRAFTING:'activity.rowCrafting',DEFENSE:'activity.rowDefense',EQUIPMENT:'navigation.equipment',ESPIONAGE:'activity.rowEspionage',EVENT:'activity.event',HOSPITAL:'activity.rowHospital',PURCHASE:'activity.rowPurchase',QUEUE:'activity.rowQueue','TIME SKIP':'activity.rowTimeSkip',TRANSPORT:'activity.rowTransport',
};
export function activityEventMessageKey(value:string):MessageKey|undefined {
 return Object.hasOwn(eventKeys,value)?eventKeys[value]:undefined;
}
