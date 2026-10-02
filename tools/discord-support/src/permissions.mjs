import { PermissionFlagsBits as P, OverwriteType } from 'discord.js';
import { fail } from './errors.mjs';
const bits = (...values) => values.reduce((a,b) => a|b,0n);
export const VIEW = bits(P.ViewChannel,P.ReadMessageHistory);
export const CHAT = bits(VIEW,P.SendMessages,P.AttachFiles,P.EmbedLinks);
export const RESTRICT = bits(P.CreatePublicThreads,P.CreatePrivateThreads,P.SendMessagesInThreads,P.CreateInstantInvite,P.MentionEveryone,P.ManageChannels,P.ManageRoles,P.ManageWebhooks,P.ManageThreads,P.SendTTSMessages);
export const READ_ONLY = bits(RESTRICT,P.SendMessages,P.AttachFiles,P.EmbedLinks,P.AddReactions,P.SendVoiceMessages,P.SendPolls);
const role = (id,allow,deny) => ({id,type:OverwriteType.Role,allow,deny});
const member = (id,allow,deny) => ({id,type:OverwriteType.Member,allow,deny});
export function overwrites(config, kind, requesterId, closed = false) {
  if (kind === 'panel') return [role(config.guildId,VIEW,READ_ONLY),member(config.botId,CHAT,0n)];
  const list = [role(config.guildId,0n,bits(P.ViewChannel,P.ManageRoles)),role(config.supportRoleId,CHAT,RESTRICT),member(config.botId,CHAT,0n)];
  if (kind === 'ticket') list.push(member(requesterId,closed ? VIEW : CHAT,closed ? READ_ONLY : RESTRICT));
  return list;
}
export function exactOverwrites(channel, expected) {
  const actual = [...channel.permissionOverwrites.cache.values()];
  const matches = actual.length === expected.length && expected.every(e => actual.some(a => a.id === e.id && a.type === e.type && a.allow.bitfield === e.allow && a.deny.bitfield === e.deny));
  if (!matches) fail('PRIVACY_OVERWRITE_MISMATCH');
}
export function currentStaff(member, config) {
  return member.guild.id === config.guildId && (member.permissions.has(P.Administrator) || member.roles.cache.has(config.supportRoleId));
}
