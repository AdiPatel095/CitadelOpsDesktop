import { useEffect, useState } from 'react';
import { CitadelAPI } from '../api/CitadelClient';
import { useCitadelAPI } from '../api/ApiContext';
import type { GameServerEntry } from '../api/Contracts';
import { configurationSection } from '../settings/Configuration';
import { serverLabel } from './serverLabel';

/** Uses the existing directory endpoint; leaves world IDs and polling unchanged. */
export function useServerLabel() {
  const { state, configuration } = useCitadelAPI();
  const [servers, setServers] = useState<GameServerEntry[]>([]);
  const connection = configurationSection(configuration, 'session.connection');
  const accountCode = typeof connection.server === 'string' ? connection.server : state?.account.worldId || '';
  useEffect(() => {
    let active = true;
    void CitadelAPI.getGameServers().then(catalog => { if (active) setServers(catalog.servers); }).catch(() => undefined);
    return () => { active = false; };
  }, []);
  return (value: string) => serverLabel(value, servers, accountCode);
}
