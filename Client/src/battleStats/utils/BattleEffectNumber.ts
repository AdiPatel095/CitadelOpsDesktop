// Legacy report effects used one fractional digit and percent units. Preserve
// those units/precision while formatting only the numeric display for the viewer.
export function formatLegacyBattleEffectPercent(value:number,locale:string):string {
 return new Intl.NumberFormat(locale,{style:'percent',minimumFractionDigits:1,maximumFractionDigits:1,signDisplay:value>0?'always':'auto'}).format(value/100);
}
