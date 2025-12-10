export interface CastleResourcesAmount {
  wood_amount: number;  // json:"wood_amount"
  stone_amount: number; // json:"stone_amount"
  food_amount: number;  // json:"food_amount"
  coal_amount: number;  // json:"coal_amount"
  oil_amount: number;   // json:"oil_amount"
  glass_amount: number; // json:"glass_amount"
  iron_amount: number;  // json:"iron_amount"
  honey_amount: number; // json:"honey_amount"
  mead_amount: number;  // json:"mead_amount"
  beef_amount: number;  // json:"beef_amount"
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
  castleName: string;
  aid: number;
  amount: CastleResourcesAmount;
  production: CastleProductionTotal;
  storage: CastleStorageMax;
}
