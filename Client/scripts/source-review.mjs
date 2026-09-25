import {createHash} from 'node:crypto';
/** Bound to source content and lexical context, independent of line-number drift. */
export function sourceCandidateIdentity(file,kind,text,context,occurrence=0) {
 return createHash('sha256').update(JSON.stringify([file,kind,text,context,occurrence])).digest('hex');
}
export function applySourceReviews(entries,records={}) {
 const errors=[];
 const known=new Set(entries.map(entry=>entry.id));
 for(const id of Object.keys(records))if(!known.has(id))errors.push(`Stale source review: ${id}`);
 const reviewed=entries.map(entry=>{
  const record=records[entry.id];
  if(!record)return entry;
  if(!['technical','user-content'].includes(record.classification)||typeof record.reason!=='string'||!record.reason.trim()||typeof record.evidence!=='string'||!record.evidence.trim()) {
   errors.push(`Invalid source review: ${entry.id}`);return entry;
  }
  // A review record cannot assert a translated sink. Visible prose must be converted.
  if(entry.category==='visible-jsx-text'||entry.category==='visible-attribute') {
   errors.push(`Visible literal requires key conversion: ${entry.id}`);return entry;
  }
  return {...entry,classification:record.classification,reason:record.reason,evidence:record.evidence};
 });
 return {entries:reviewed,errors};
}
