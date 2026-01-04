package Models

import (
	"log"
)

// CastleTroops represents troop counts for a single castle location
type CastleTroops struct {
	KingdomID int         `json:"kingdomID"`
	CastleID  int         `json:"castleID"` // Added CastleID
	X         int         `json:"x"`
	Y         int         `json:"y"`
	Troops    map[int]int `json:"troops"` // unitID -> count (troops only, no tools)
}

// SaveInCastleTroops represents the troops to keep in castle per castle ID
// This is loaded from BirdIgnoreList.json and persists across restarts
type SaveInCastleTroops struct {
	Troops map[int]map[int]int // CastleID -> (unitID -> count)
}

// BirdIgnoreListFile represents the JSON file structure
type BirdIgnoreListFile struct {
	Description string             `json:"description"`
	Castles     map[string][][]int `json:"castles"` // CastleID (string) -> [[troopID, count], ...]
}

// Global instance of SaveInCastleTroops loaded from file
var BirdIgnoreList SaveInCastleTroops

// AutoBirdDelayConfig holds the min and max delay hours
type AutoBirdDelayConfig struct {
	MinDelay int
	MaxDelay int
}

var AutoBirdDelay = AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12}

// GetSaveAmount returns the amount of troops to save for a specific castle and unit
func (s *SaveInCastleTroops) GetSaveAmount(castleID, unitID int) int {
	if s.Troops == nil {
		return 0
	}

	// Check specific castle config
	if castleConfig, ok := s.Troops[castleID]; ok {
		if amount, ok := castleConfig[unitID]; ok {
			return amount
		}
	}

	return 0
}

// UpdateBirdIgnoreList updates the in-memory bird ignore list from the given map
func UpdateBirdIgnoreList(data map[int]map[int]int) {
	BirdIgnoreList.Troops = data
	log.Printf("[BirdIgnoreList] Updated in-memory config for %d castles", len(BirdIgnoreList.Troops))
}

// ClearBirdIgnoreList clears the BirdIgnoreList from memory to save space
func ClearBirdIgnoreList() {
	BirdIgnoreList.Troops = nil
	log.Println("[BirdIgnoreList] Cleared from memory")
}

// TroopIDs contains all valid troop unit IDs and their names
var TroopIDs = map[int]string{
	5:   "Veteran Saber Cleaver",
	6:   "Veteran Slingshot",
	7:   "Cave Smasher",
	8:   "Cave Hunter",
	9:   "Veteran Demon Horror",
	10:  "Veteran Deathly Horror",
	11:  "Veteran Flame Bearer",
	12:  "Veteran Composite Bowman",
	13:  "Muscle Man",
	14:  "Marksman",
	15:  "Fruit Pirate",
	18:  "Imperial Guardsman",
	19:  "Imperial Bowman",
	20:  "Imperial Knight",
	21:  "Imperial Marksman",
	22:  "Berserker",
	23:  "Spear Woman",
	34:  "Renegade Sai Warrior",
	35:  "Renegade Kunai Thrower",
	36:  "Renegade Katana Warrior",
	37:  "Renegade Bow Master",
	38:  "Katana Warrior",
	39:  "Bow Master",
	40:  "Skeleton Warrior",
	41:  "Skeleton Bowman",
	42:  "Pumpkin Butcher",
	43:  "Raven Chomper",
	48:  "Bone Huntress",
	50:  "Frost Bowman",
	51:  "Master Bone Huntress",
	52:  "Master Frost Bowman",
	58:  "Star-Spangled Knight",
	59:  "Star-Spangled Crossbowman",
	68:  "Cave Smasher",
	74:  "Cave Hunter",
	75:  "Knight of the Elite Guard",
	76:  "Crossbowman of the Elite Guard",
	78:  "Renegade Cave Smasher",
	79:  "Renegade Cave Hunter",
	83:  "Forest Warrior",
	84:  "Forest Hunter",
	85:  "Forest Guardian",
	86:  "Forest Bowman",
	92:  "Demon Shadow",
	93:  "Deathly Shadow",
	100: "Shamrock Huntsman",
	101: "Shamrock Assassin",
	102: "Spring Huntress",
	103: "Spring Footsoldier",
	117: "Fire Witch",
	118: "Bone Rattler",
	119: "Skeletal Hunter",
	120: "Skeletal Scytheman",
	126: "Shapeshifter Legionnaire",
	127: "Shapeshifter Sharpshooter",
	132: "Gnomad Warrior",
	133: "Gnomad Archer",
	134: "Gingerbread Brawler",
	135: "Gingerbread Sniper",
	146: "Knight of the Throne-Watcher",
	147: "Marksman of the Throne-Watcher",
	148: "Relic Axeman",
	149: "Relic Shortbowman",
	150: "Relic Hammerman",
	151: "Relic Longbowman",
	183: "Cultist Brawler",
	184: "Cultist Slingshot",
	185: "Cultist Warrior",
	186: "Cultist Hunter",
	187: "Wilderness Brawler",
	188: "Wilderness Slingshot",
	189: "Wilderness Warrior",
	190: "Wilderness Hunter",
	191: "Corrupted Assassin",
	192: "Corrupted Crossbowman",
	193: "Corrupted Veteran Halberdier",
	194: "Corrupted Veteran Longbowman",
	195: "Shield-Maiden",
	205: "Valkyrie Ranger",
	217: "Protector of the North",
	228: "Valkyrie Sniper",
	277: "Direwolf",
	288: "Easter Championess",
	299: "Valkyrie Huntress",
	308: "Veteran Halberdier",
	309: "Veteran Two-Handed Swordsman",
	311: "Veteran Longbowman",
	312: "Veteran Heavy Crossbowman",
	336: "Guardian of Spring",
	347: "Celestial Marksman",
	358: "Summer Huntress",
	369: "Fruit Breaker",
	409: "Star-Spangled Veteran Demon Horror",
	410: "Star-Spangled Veteran Deathly Horror",
	461: "Summer Marksman",
	479: "Stein Smasher",
	480: "Cask Marksmann",
	481: "Pretzel Guardian",
	482: "Bavarian Brewer",
	485: "Glacial Amazon Warrior",
	486: "Glacial Amazon Archer",
	487: "Glacial Amazon Guardian",
	488: "Glacial Amazon Huntress",
	494: "Christmas Warrior",
	495: "Christmas Archer",
	496: "Christmas Guardian",
	497: "Christmas Huntress",
	498: "Festive Warrior",
	499: "Festive Archer",
	500: "Festive Guardian",
	501: "Festive Huntress",
	502: "Forsaken Maiden",
	503: "Forlorn Ranger",
	513: "Glasswing Archer",
	524: "Flamebreath Berserker",
	535: "Scalebound Guardian",
	546: "Scaleshard Marksman",
	601: "Swordsman",
	602: "Spearman",
	603: "Maceman",
	604: "Halberdier",
	605: "Two-Handed Swordsman",
	606: "Archer",
	607: "Crossbowman",
	608: "Bowman",
	609: "Heavy Crossbowman",
	610: "Longbowman",
	612: "Shadow Maceman",
	613: "Shadow Crossbowman",
	615: "Shadow Ram",
	616: "Shadow Ladder",
	618: "Shadow Bundles",
	619: "Shadow Shields",
	628: "Veteran Spearman",
	630: "Veteran Maceman",
	631: "Veteran Bowman",
	636: "Veteran Crossbowman",
	652: "Armed Citizen",
	655: "Traveling Knight",
	656: "Traveling Crossbowman",
	657: "Shark Tooth Warrior",
	658: "Stone Smasher",
	659: "Prince",
	662: "Skeleton Warrior",
	663: "Skeleton Bowman",
	664: "Crossbowman of the Kingsguard",
	667: "Shadow Rogue",
	668: "Shadow Felon",
	670: "Norseman With Axe",
	671: "Norseman With Bow",
	672: "Knight of the Kingsguard",
	673: "Dragon Claws",
	674: "Dragon Fire",
	675: "Saber Warrior",
	676: "Desert Bowman",
	677: "Norseman With Axe",
	678: "Norseman With Bow",
	679: "Cultist Fanatic",
	680: "Cultist Bowman",
	684: "Marauder",
	685: "Pyromaniac",
	686: "Sentinel of the Kingsguard",
	687: "Scout of the Kingsguard",
	688: "Renegade Saber Warrior",
	689: "Renegade Desert Bowman",
	690: "Renegade Norseman Warrior",
	691: "Renegade Norseman Bowman",
	692: "Renegade Cultist Warrior",
	693: "Renegade Cultist Bowman",
	698: "Cow Berdier",
	699: "Longbow Ox",
	710: "Bear Warrior",
	711: "Bear Bowman",
	712: "Lion Warrior",
	713: "Lion Bowman",
	714: "Demon Horror",
	715: "Deathly Horror",
	716: "Swashbuckler",
	717: "Sail Ripper",
	718: "Tentacle",
	719: "Kraken Head",
	720: "Wolfhound",
	721: "Barbarian",
	722: "Composite Bowman",
	723: "Flame Bearer",
	724: "Veteran Swordsman",
	725: "Khan Guard",
	726: "Saber Cleaver",
	727: "Slingshot",
	728: "Renegade Lancer",
	729: "Renegade Spear Thrower",
	743: "Lancer",
	744: "Spear Thrower",
	746: "Militia",
	747: "Shadow Scoundrel",
	748: "Shadow Wretch",
	749: "Shadow Battering Ram",
	750: "Shadow Siege Tower",
	751: "Shadow Assault Bridge",
	752: "Shadow Mantlet",
	753: "Demon Slayer",
	754: "Assassin",
	759: "Renegade Swashbuckler",
	760: "Renegade Sail Ripper",
	765: "Veteran Marauder",
	766: "Veteran Pyromaniac",
	767: "Renegade Shark Tooth Warrior",
	768: "Renegade Stone Smasher",
	781: "Master Swordsman",
	782: "Master Archer",
	960: "Forest Warrior",
	961: "Forest Hunter",
	962: "Renegade Swashbuckler",
	963: "Renegade Sail Ripper",
	//Leveled units - Jolly Gingerbread Sniper
	478: "Jolly Gingerbread Sniper",
	//Leveled units - Candy Cane Protector
	477: "Candy Cane Protector",
	// Leveled units - Shield-Maiden
	196: "Shield-Maiden", 197: "Shield-Maiden", 198: "Shield-Maiden", 199: "Shield-Maiden",
	200: "Shield-Maiden", 201: "Shield-Maiden", 202: "Shield-Maiden", 203: "Shield-Maiden",
	204: "Shield-Maiden", 215: "Shield-Maiden",
	// Leveled units - Valkyrie Ranger
	206: "Valkyrie Ranger", 207: "Valkyrie Ranger", 208: "Valkyrie Ranger", 209: "Valkyrie Ranger",
	210: "Valkyrie Ranger", 211: "Valkyrie Ranger", 212: "Valkyrie Ranger", 213: "Valkyrie Ranger",
	214: "Valkyrie Ranger", 216: "Valkyrie Ranger",
	// Leveled units - Protector of the North
	218: "Protector of the North", 219: "Protector of the North", 220: "Protector of the North",
	221: "Protector of the North", 222: "Protector of the North", 223: "Protector of the North",
	224: "Protector of the North", 225: "Protector of the North", 226: "Protector of the North",
	227: "Protector of the North", 489: "Protector of the North",
	// Leveled units - Valkyrie Sniper
	229: "Valkyrie Sniper", 230: "Valkyrie Sniper", 231: "Valkyrie Sniper", 232: "Valkyrie Sniper",
	233: "Valkyrie Sniper", 234: "Valkyrie Sniper", 235: "Valkyrie Sniper", 236: "Valkyrie Sniper",
	237: "Valkyrie Sniper", 238: "Valkyrie Sniper", 493: "Valkyrie Sniper",
	// Leveled units - Forsaken Maiden
	472: "Forsaken Maiden", 483: "Forsaken Maiden",
	// Leveled units - Forlorn Ranger
	473: "Forlorn Ranger", 484: "Forlorn Ranger",
	// Leveled units - Glasswing Archer
	514: "Glasswing Archer", 515: "Glasswing Archer", 516: "Glasswing Archer", 517: "Glasswing Archer",
	518: "Glasswing Archer", 519: "Glasswing Archer", 520: "Glasswing Archer", 521: "Glasswing Archer",
	522: "Glasswing Archer", 523: "Glasswing Archer",
	// Leveled units - Flamebreath Berserker
	525: "Flamebreath Berserker", 526: "Flamebreath Berserker", 527: "Flamebreath Berserker",
	528: "Flamebreath Berserker", 529: "Flamebreath Berserker", 530: "Flamebreath Berserker",
	531: "Flamebreath Berserker", 532: "Flamebreath Berserker", 533: "Flamebreath Berserker",
	534: "Flamebreath Berserker",
	// Leveled units - Scalebound Guardian
	536: "Scalebound Guardian", 537: "Scalebound Guardian", 538: "Scalebound Guardian",
	539: "Scalebound Guardian", 540: "Scalebound Guardian", 541: "Scalebound Guardian",
	542: "Scalebound Guardian", 543: "Scalebound Guardian", 544: "Scalebound Guardian",
	545: "Scalebound Guardian",
	// Leveled units - Scaleshard Marksman
	547: "Scaleshard Marksman", 548: "Scaleshard Marksman", 549: "Scaleshard Marksman",
	550: "Scaleshard Marksman", 551: "Scaleshard Marksman", 552: "Scaleshard Marksman",
	553: "Scaleshard Marksman", 554: "Scaleshard Marksman", 555: "Scaleshard Marksman",
	556: "Scaleshard Marksman",
	// Leveled units - Swashbuckler
	701: "Swashbuckler", 702: "Swashbuckler",
	// Leveled units - Sail Ripper
	703: "Sail Ripper", 704: "Sail Ripper",
	// Leveled units - Renegade Swashbuckler (levels)
	705: "Renegade Swashbuckler", 706: "Renegade Swashbuckler",
	// Leveled units - Renegade Sail Ripper (levels)
	707: "Renegade Sail Ripper", 708: "Renegade Sail Ripper",
	// Leveled units - Katana Warrior
	820: "Katana Warrior", 821: "Katana Warrior", 822: "Katana Warrior", 823: "Katana Warrior",
	824: "Katana Warrior", 825: "Katana Warrior", 826: "Katana Warrior", 827: "Katana Warrior",
	828: "Katana Warrior", 829: "Katana Warrior",
	// Leveled units - Bow Master
	830: "Bow Master", 831: "Bow Master", 832: "Bow Master", 833: "Bow Master",
	834: "Bow Master", 835: "Bow Master", 836: "Bow Master", 837: "Bow Master",
	838: "Bow Master", 839: "Bow Master",
	// Leveled units - Sai Warrior
	861: "Sai Warrior", 862: "Sai Warrior", 863: "Sai Warrior", 864: "Sai Warrior",
	865: "Sai Warrior", 866: "Sai Warrior", 867: "Sai Warrior", 868: "Sai Warrior",
	869: "Sai Warrior",
	// Leveled units - Kunai Thrower
	871: "Kunai Thrower", 872: "Kunai Thrower", 873: "Kunai Thrower", 874: "Kunai Thrower",
	875: "Kunai Thrower", 876: "Kunai Thrower", 877: "Kunai Thrower", 878: "Kunai Thrower",
	879: "Kunai Thrower",
	// Leveled units - Lancer
	900: "Lancer", 901: "Lancer", 902: "Lancer", 903: "Lancer", 904: "Lancer",
	905: "Lancer", 906: "Lancer", 907: "Lancer", 908: "Lancer", 909: "Lancer",
	// Leveled units - Spear Thrower
	910: "Spear Thrower", 911: "Spear Thrower", 912: "Spear Thrower", 913: "Spear Thrower",
	914: "Spear Thrower", 915: "Spear Thrower", 916: "Spear Thrower", 917: "Spear Thrower",
	918: "Spear Thrower", 919: "Spear Thrower",
	// Leveled units - Veteran Saber Cleaver
	940: "Veteran Saber Cleaver", 941: "Veteran Saber Cleaver", 942: "Veteran Saber Cleaver",
	943: "Veteran Saber Cleaver", 944: "Veteran Saber Cleaver", 945: "Veteran Saber Cleaver",
	946: "Veteran Saber Cleaver", 947: "Veteran Saber Cleaver", 948: "Veteran Saber Cleaver",
	949: "Veteran Saber Cleaver",
	// Leveled units - Veteran Slingshot
	950: "Veteran Slingshot", 951: "Veteran Slingshot", 952: "Veteran Slingshot",
	953: "Veteran Slingshot", 954: "Veteran Slingshot", 955: "Veteran Slingshot",
	956: "Veteran Slingshot", 957: "Veteran Slingshot", 958: "Veteran Slingshot",
	959: "Veteran Slingshot",
	// Leveled units - Veteran Halberdier
	2000: "Veteran Halberdier", 2001: "Veteran Halberdier", 2002: "Veteran Halberdier",
	2003: "Veteran Halberdier", 2004: "Veteran Halberdier", 2005: "Veteran Halberdier",
	2006: "Veteran Halberdier", 2007: "Veteran Halberdier", 2008: "Veteran Halberdier",
	2009: "Veteran Halberdier",
	// Leveled units - Veteran Two-Handed Swordsman
	2010: "Veteran Two-Handed Swordsman", 2011: "Veteran Two-Handed Swordsman",
	2012: "Veteran Two-Handed Swordsman", 2013: "Veteran Two-Handed Swordsman",
	2014: "Veteran Two-Handed Swordsman", 2015: "Veteran Two-Handed Swordsman",
	2016: "Veteran Two-Handed Swordsman", 2017: "Veteran Two-Handed Swordsman",
	2018: "Veteran Two-Handed Swordsman", 2019: "Veteran Two-Handed Swordsman",
	// Leveled units - Relic Axeman
	2020: "Relic Axeman", 2021: "Relic Axeman", 2022: "Relic Axeman", 2023: "Relic Axeman",
	2024: "Relic Axeman", 2025: "Relic Axeman", 2026: "Relic Axeman", 2027: "Relic Axeman",
	2028: "Relic Axeman", 2029: "Relic Axeman",
	// Leveled units - Relic Hammerman
	2030: "Relic Hammerman", 2031: "Relic Hammerman", 2032: "Relic Hammerman",
	2033: "Relic Hammerman", 2034: "Relic Hammerman", 2035: "Relic Hammerman",
	2036: "Relic Hammerman", 2037: "Relic Hammerman", 2038: "Relic Hammerman",
	2039: "Relic Hammerman",
	// Leveled units - Veteran Longbowman
	2040: "Veteran Longbowman", 2041: "Veteran Longbowman", 2042: "Veteran Longbowman",
	2043: "Veteran Longbowman", 2044: "Veteran Longbowman", 2045: "Veteran Longbowman",
	2046: "Veteran Longbowman", 2047: "Veteran Longbowman", 2048: "Veteran Longbowman",
	2049: "Veteran Longbowman",
	// Leveled units - Veteran Heavy Crossbowman
	2050: "Veteran Heavy Crossbowman", 2051: "Veteran Heavy Crossbowman",
	2052: "Veteran Heavy Crossbowman", 2053: "Veteran Heavy Crossbowman",
	2054: "Veteran Heavy Crossbowman", 2055: "Veteran Heavy Crossbowman",
	2056: "Veteran Heavy Crossbowman", 2057: "Veteran Heavy Crossbowman",
	2058: "Veteran Heavy Crossbowman", 2059: "Veteran Heavy Crossbowman",
	// Leveled units - Relic Shortbowman
	2060: "Relic Shortbowman", 2061: "Relic Shortbowman", 2062: "Relic Shortbowman",
	2063: "Relic Shortbowman", 2064: "Relic Shortbowman", 2065: "Relic Shortbowman",
	2066: "Relic Shortbowman", 2067: "Relic Shortbowman", 2068: "Relic Shortbowman",
	2069: "Relic Shortbowman",
	// Leveled units - Relic Longbowman
	2070: "Relic Longbowman", 2071: "Relic Longbowman", 2072: "Relic Longbowman",
	2073: "Relic Longbowman", 2074: "Relic Longbowman", 2075: "Relic Longbowman",
	2076: "Relic Longbowman", 2077: "Relic Longbowman", 2078: "Relic Longbowman",
	2079: "Relic Longbowman",
}

// ToolIDs contains all tool unit IDs and their names
var ToolIDs = map[int]string{
	// No tools were defined in the data file
}

// IsTroop checks if a unit ID is a troop (not a tool)
func IsTroop(unitID int) bool {
	_, exists := TroopIDs[unitID]
	return exists
}

// IsTool checks if a unit ID is a tool (not a troop)
func IsTool(unitID int) bool {
	_, exists := ToolIDs[unitID]
	return exists
}
