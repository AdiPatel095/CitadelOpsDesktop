export interface UnitAtlasDefinition {
  width: number;
  height: number;
  frames: readonly (readonly [x: number, y: number, width: number, height: number])[];
}

// Older official Barracks art packs bottom-aligned BMP layers into one texture.
// Frame order follows the game's BMP_0, BMP_1, ... animation metadata.
const UNIT_ATLASES: Record<string, UnitAtlasDefinition> = {
  Archer: { width: 94, height: 308, frames: [[1, 175, 92, 132], [1, 47, 71, 126], [46, 1, 42, 44], [1, 1, 43, 42]] },
  Bowman: { width: 110, height: 329, frames: [[1, 157, 92, 124], [1, 1, 108, 154], [1, 283, 43, 45], [46, 283, 43, 43]] },
  Crossbowman: { width: 169, height: 128, frames: [[74, 1, 53, 126], [1, 1, 71, 126], [129, 1, 39, 47], [129, 50, 39, 46]] },
  Elitebowman: { width: 174, height: 198, frames: [[92, 1, 81, 125], [1, 1, 89, 132], [92, 128, 61, 65], [1, 135, 63, 62]] },
  Elitecrossbowman: { width: 169, height: 252, frames: [[1, 1, 89, 128], [92, 1, 76, 132], [56, 135, 59, 114], [1, 131, 53, 120]] },
  Elitehalberd: { width: 209, height: 134, frames: [[127, 1, 81, 132], [80, 1, 45, 130], [1, 1, 39, 119], [42, 1, 36, 119]] },
  Eliteheavycrossbowman: { width: 348, height: 126, frames: [[75, 1, 48, 124], [125, 1, 55, 122], [1, 1, 72, 124], [310, 1, 37, 66], [225, 1, 39, 94], [266, 93, 23, 31], [182, 1, 41, 95], [266, 1, 42, 90]] },
  Elitekingscrossbowman: { width: 241, height: 129, frames: [[156, 1, 84, 127], [54, 1, 51, 122], [1, 1, 51, 118], [107, 1, 47, 122]] },
  Elitekingsmace: { width: 119, height: 393, frames: [[1, 1, 81, 123], [52, 126, 66, 132], [1, 260, 67, 132], [1, 126, 49, 128]] },
  Elitelongbowman: { width: 163, height: 231, frames: [[1, 1, 85, 132], [88, 1, 63, 117], [123, 120, 39, 68], [1, 135, 38, 95], [123, 190, 12, 36], [81, 135, 40, 92], [41, 135, 38, 95]] },
  Elitemace: { width: 154, height: 223, frames: [[1, 1, 93, 132], [1, 135, 60, 87], [63, 135, 58, 82], [96, 1, 57, 87]] },
  Elitespeerman: { width: 163, height: 187, frames: [[1, 1, 95, 132], [98, 1, 64, 116], [98, 119, 64, 67], [1, 135, 55, 48]] },
  Eliteswordman: { width: 238, height: 132, frames: [[53, 1, 86, 128], [193, 80, 6, 7], [141, 1, 50, 125], [1, 1, 50, 130], [193, 1, 44, 77]] },
  Elitetwohandedsword: { width: 91, height: 322, frames: [[1, 84, 63, 103], [1, 189, 89, 132], [42, 1, 37, 81], [1, 1, 39, 75]] },
  Halberd: { width: 126, height: 248, frames: [[1, 1, 82, 132], [85, 1, 40, 126], [85, 129, 36, 118], [1, 135, 35, 112]] },
  Heavycrossbowman: { width: 122, height: 226, frames: [[73, 1, 48, 124], [1, 1, 70, 132], [73, 127, 41, 95], [1, 135, 42, 90]] },
  Kingsbowman: { width: 96, height: 218, frames: [[1, 1, 94, 132], [1, 135, 46, 42], [49, 135, 40, 42], [1, 179, 46, 38]] },
  Kingscrossbowman: { width: 89, height: 240, frames: [[1, 1, 87, 132], [1, 135, 54, 51], [1, 188, 53, 51], [56, 188, 32, 48]] },
  Kingsmace: { width: 140, height: 214, frames: [[1, 1, 87, 132], [1, 135, 53, 78], [90, 1, 49, 78], [56, 135, 58, 68]] },
  Kingsspeerman: { width: 95, height: 318, frames: [[1, 1, 93, 132], [1, 135, 70, 72], [1, 264, 55, 53], [1, 209, 58, 53]] },
  Longbowman: { width: 84, height: 360, frames: [[1, 227, 82, 132], [1, 98, 64, 127], [43, 1, 38, 95], [1, 1, 40, 90]] },
  Mace: { width: 89, height: 211, frames: [[1, 1, 87, 132], [1, 135, 38, 67], [41, 135, 27, 38], [41, 175, 27, 35]] },
  Speerman: { width: 115, height: 238, frames: [[1, 1, 80, 132], [1, 135, 56, 61], [59, 135, 54, 56], [59, 193, 55, 44]] },
  Swordman: { width: 99, height: 250, frames: [[1, 1, 86, 132], [1, 135, 58, 61], [1, 198, 44, 51], [47, 198, 51, 44]] },
  Twohandedsword: { width: 243, height: 134, frames: [[160, 1, 82, 132], [106, 1, 52, 119], [1, 1, 49, 106], [52, 1, 52, 113]] },
};

export function unitAtlasDefinition(type: unknown): UnitAtlasDefinition | undefined {
  if (typeof type !== 'string') return undefined;
  return UNIT_ATLASES[type];
}
