export interface CastleResourcesAmount {
  wood_amount: number;
  stone_amount: number;
  food_amount: number;
  coal_amount: number;
  oil_amount: number;
  glass_amount: number;
  iron_amount: number;
  honey_amount: number;
  mead_amount: number;
  beef_amount: number;
}

export interface CastleProductionTotal {
  wood_prod: number;
  stone_prod: number;
  food_prod: number;
  coal_prod: number;
  oil_prod: number;
  glass_prod: number;
  iron_prod: number;
  honey_prod: number;
  mead_prod: number;
  beef_prod: number;
}

export interface CastleStorageMax {
  wood_max: number;
  stone_max: number;
  food_max: number;
  coal_max: number;
  oil_max: number;
  glass_max: number;
  iron_max: number;
  honey_max: number;
  mead_max: number;
  beef_max: number;
}

export interface PlayerCastleInfo {
  main_castle_name: string;
  outpost_1_name: string;
  outpost_2_name: string;
  outpost_3_name: string;
  ice_castle_name: string;
  desert_castle_name: string;
  dungeon_castle_name: string;
  storm_castle_name: string;
  main_castle_aid: number;
  outpost_1_aid: number;
  outpost_2_aid: number;
  outpost_3_aid: number;
  ice_castle_aid: number;
  desert_castle_aid: number;
  dungeon_castle_aid: number;
  storm_castle_aid: number;
  main_castle_amount: CastleResourcesAmount;
  main_castle_production: CastleProductionTotal;
  main_castle_storage: CastleStorageMax;
  outpost_1_amount: CastleResourcesAmount;
  outpost_1_production: CastleProductionTotal;
  outpost_1_storage: CastleStorageMax;
  outpost_2_amount: CastleResourcesAmount;
  outpost_2_production: CastleProductionTotal;
  outpost_2_storage: CastleStorageMax;
  outpost_3_amount: CastleResourcesAmount;
  outpost_3_production: CastleProductionTotal;
  outpost_3_storage: CastleStorageMax;
  ice_castle_amount: CastleResourcesAmount;
  ice_castle_production: CastleProductionTotal;
  ice_castle_storage: CastleStorageMax;
  desert_castle_amount: CastleResourcesAmount;
  desert_castle_production: CastleProductionTotal;
  desert_castle_storage: CastleStorageMax;
  dungeon_castle_amount: CastleResourcesAmount;
  dungeon_castle_production: CastleProductionTotal;
  dungeon_castle_storage: CastleStorageMax;
  storm_castle_amount: CastleResourcesAmount;
  storm_castle_production: CastleProductionTotal;
  storm_castle_storage: CastleStorageMax;
}
