import { LocalizedText } from '../../i18n/LocalizedText';
import { messageLanguageAttributes } from '../../i18n/messageLanguage';
import type { PlayerStatusMessage } from './StatusBadge';
import './StatusBadge.css';
export type PlayerRole = 'owner' | 'admin' | 'editor';
export function accountPlayerRole(account: { isOwner: boolean; accessLevel: 'admin' | 'edit' }): PlayerRole {
  return account.isOwner ? 'owner' : account.accessLevel === 'admin' ? 'admin' : 'editor';
}
export function RoleTag({ role, labelMessage }: { role: PlayerRole; labelMessage?: PlayerStatusMessage }) {
  return <span className="player-role-tag" data-player-role={role}><span {...(labelMessage ? messageLanguageAttributes(labelMessage) : {})}>{labelMessage ? labelMessage.text : <LocalizedText messageKey={`playerRole.${role}`} />}</span></span>;
}
