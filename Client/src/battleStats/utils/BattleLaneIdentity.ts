// Exact producer identities: Server/Reports/BattleDetailParser.go battleLaneNames.
const laneKeys={'Left flank':'battle.leftFlank','Middle front':'battle.middleFront','Right flank':'battle.rightFlank'} as const;
const positionKeys=['battle.leftFlank','battle.middleFront','battle.rightFlank'] as const;
export function battleLaneMessageKey(label:string,index:number){
 if(label)return Object.hasOwn(laneKeys,label) ? laneKeys[label as keyof typeof laneKeys] : undefined;
 return positionKeys[index];
}
