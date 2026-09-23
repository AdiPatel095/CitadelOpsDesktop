/** Preserve the original rounded day/hour/minute countdown; only presentation is localized. */
export function automationDuration(minutes: number, locale: string): string {
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  const remainingMinutes = minutes % 60;
  const unit = (value:number,name:'day'|'hour'|'minute') => new Intl.NumberFormat(locale,{style:'unit',unit:name,unitDisplay:'short'}).format(value);
  const parts = days > 0 ? [unit(days,'day'),...(hours > 0 ? [unit(hours,'hour')] : [])]
    : hours > 0 ? [unit(hours,'hour'),...(remainingMinutes > 0 ? [unit(remainingMinutes,'minute')] : [])]
    : [unit(remainingMinutes,'minute')];
  return new Intl.ListFormat(locale,{style:'short',type:'unit'}).format(parts);
}
export function nextWakeParameters(timestamp:number,now:number,locale:string) {
  return {state:timestamp<=0?'waiting':timestamp<=now?'due':'future',duration:automationDuration(Math.max(1,Math.ceil((timestamp-now)/60_000)),locale)};
}
export function timedRemainingParameters(expiresAt:number,now:number,locale:string) {
  return {duration:automationDuration(Math.max(1,Math.ceil((expiresAt-now)/60_000)),locale)};
}
