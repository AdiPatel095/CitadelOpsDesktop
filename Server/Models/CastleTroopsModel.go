package Models

// CastleTroops represents troop counts for a single castle location
type CastleTroops struct {
	KingdomID   int            `json:"kingdomID"`
	CastleID    int            `json:"castleID"`
	X           int            `json:"x"`
	Y           int            `json:"y"`
	Troops      map[int]int    `json:"troops"`      // unitID -> count (troops only, no tools) - Legacy/I
	TroopsI     map[int]int    `json:"troopsI"`     // Units currently in castle
	TroopsTU    map[int]int    `json:"troopsTU"`    // Units travelling
	TroopsHI    map[int]int    `json:"troopsHI"`    // Units in hospital
	TroopsSHI   map[int]int    `json:"troopsSHI"`   // Units in special hospital
	TroopsMixed map[int]int    `json:"troopsMixed"` // Combined I + TU
	Buildings   []BuildingData `json:"buildings"`   // New: building information
}

// TroopIDs contains all valid troop unit IDs and their names
type TroopInfo struct {
	Name              string
	ConsumptionType   string // "food", "mead", "beef"
	ConsumptionAmount int
}

var TroopIDs = map[int]TroopInfo{
	5: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	6: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	7: {
		Name:              "Cave Smasher",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	8: {
		Name:              "Cave Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	9: {
		Name:              "Veteran Demon Horror",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	10: {
		Name:              "Veteran Deathly Horror",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	11: {
		Name:              "Veteran Flame Bearer",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	12: {
		Name:              "Veteran Composite Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	13: {
		Name:              "Muscle Man",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	14: {
		Name:              "Marksman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	15: {
		Name:              "Fruit Pirate",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	18: {
		Name:              "Imperial Guardsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	19: {
		Name:              "Imperial Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	20: {
		Name:              "Imperial Knight",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	21: {
		Name:              "Imperial Marksman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	22: {
		Name:              "Berserker",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	23: {
		Name:              "Spear Woman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	34: {
		Name:              "Renegade Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	35: {
		Name:              "Renegade Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	36: {
		Name:              "Renegade Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	37: {
		Name:              "Renegade Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	38: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	39: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	40: {
		Name:              "Skeleton Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	41: {
		Name:              "Skeleton Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	42: {
		Name:              "Pumpkin Butcher",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	43: {
		Name:              "Raven Chomper",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	48: {
		Name:              "Bone Huntress",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	50: {
		Name:              "Frost Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	51: {
		Name:              "Master Bone Huntress",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	52: {
		Name:              "Master Frost Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	58: {
		Name:              "Star-Spangled Knight",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	59: {
		Name:              "Star-Spangled Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	68: {
		Name:              "Cave Smasher",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	74: {
		Name:              "Cave Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	75: {
		Name:              "Knight of the Elite Guard",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	76: {
		Name:              "Crossbowman of the Elite Guard",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	78: {
		Name:              "Renegade Cave Smasher",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	79: {
		Name:              "Renegade Cave Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	83: {
		Name:              "Forest Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	84: {
		Name:              "Forest Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	85: {
		Name:              "Forest Guardian",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	86: {
		Name:              "Forest Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	92: {
		Name:              "Demon Shadow",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	93: {
		Name:              "Deathly Shadow",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	100: {
		Name:              "Shamrock Huntsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	101: {
		Name:              "Shamrock Assassin",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	102: {
		Name:              "Spring Huntress",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	103: {
		Name:              "Spring Footsoldier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	117: {
		Name:              "Fire Witch",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	118: {
		Name:              "Bone Rattler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	119: {
		Name:              "Skeletal Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	120: {
		Name:              "Skeletal Scytheman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	126: {
		Name:              "Shapeshifter Legionnaire",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	127: {
		Name:              "Shapeshifter Sharpshooter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	132: {
		Name:              "Gnomad Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	133: {
		Name:              "Gnomad Archer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	134: {
		Name:              "Gingerbread Brawler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	135: {
		Name:              "Gingerbread Sniper",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	146: {
		Name:              "Knight of the Throne-Watcher",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	147: {
		Name:              "Marksman of the Throne-Watcher",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	148: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	149: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	150: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	151: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	183: {
		Name:              "Cultist Brawler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	184: {
		Name:              "Cultist Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	185: {
		Name:              "Cultist Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	186: {
		Name:              "Cultist Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	187: {
		Name:              "Wilderness Brawler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	188: {
		Name:              "Wilderness Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	189: {
		Name:              "Wilderness Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	190: {
		Name:              "Wilderness Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	191: {
		Name:              "Corrupted Assassin",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	192: {
		Name:              "Corrupted Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	193: {
		Name:              "Corrupted Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	194: {
		Name:              "Corrupted Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	195: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	205: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	217: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	228: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	277: {
		Name:              "Direwolf",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	288: {
		Name:              "Easter Championess",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	299: {
		Name:              "Valkyrie Huntress",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	308: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	309: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	311: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	312: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	336: {
		Name:              "Guardian of Spring",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	347: {
		Name:              "Celestial Marksman",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	358: {
		Name:              "Summer Huntress",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	369: {
		Name:              "Fruit Breaker",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	409: {
		Name:              "Star-Spangled Veteran Demon Horror",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	410: {
		Name:              "Star-Spangled Veteran Deathly Horror",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	461: {
		Name:              "Summer Marksman",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	479: {
		Name:              "Stein Smasher",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	480: {
		Name:              "Cask Marksmann",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	481: {
		Name:              "Pretzel Guardian",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	482: {
		Name:              "Bavarian Brewer",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	485: {
		Name:              "Glacial Amazon Warrior",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	486: {
		Name:              "Glacial Amazon Archer",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	487: {
		Name:              "Glacial Amazon Guardian",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	488: {
		Name:              "Glacial Amazon Huntress",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	494: {
		Name:              "Christmas Warrior",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	495: {
		Name:              "Christmas Archer",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	496: {
		Name:              "Christmas Guardian",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	497: {
		Name:              "Christmas Huntress",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	498: {
		Name:              "Festive Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	499: {
		Name:              "Festive Archer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	500: {
		Name:              "Festive Guardian",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	501: {
		Name:              "Festive Huntress",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	502: {
		Name:              "Forsaken Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	503: {
		Name:              "Forlorn Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	513: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	524: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	535: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	546: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	601: {
		Name:              "Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	602: {
		Name:              "Spearman",
		ConsumptionType:   "food",
		ConsumptionAmount: 2,
	},
	603: {
		Name:              "Maceman",
		ConsumptionType:   "food",
		ConsumptionAmount: 2,
	},
	604: {
		Name:              "Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	605: {
		Name:              "Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	606: {
		Name:              "Archer",
		ConsumptionType:   "food",
		ConsumptionAmount: 2,
	},
	607: {
		Name:              "Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 2,
	},
	608: {
		Name:              "Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 2,
	},
	609: {
		Name:              "Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	610: {
		Name:              "Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	612: {
		Name:              "Shadow Maceman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	613: {
		Name:              "Shadow Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	615: {
		Name:              "Shadow Ram",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	616: {
		Name:              "Shadow Ladder",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	618: {
		Name:              "Shadow Bundles",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	619: {
		Name:              "Shadow Shields",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	628: {
		Name:              "Veteran Spearman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	630: {
		Name:              "Veteran Maceman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	631: {
		Name:              "Veteran Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	636: {
		Name:              "Veteran Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	652: {
		Name:              "Armed Citizen",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	655: {
		Name:              "Traveling Knight",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	656: {
		Name:              "Traveling Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	657: {
		Name:              "Shark Tooth Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	658: {
		Name:              "Stone Smasher",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	659: {
		Name:              "Prince",
		ConsumptionType:   "food",
		ConsumptionAmount: 2,
	},
	662: {
		Name:              "Skeleton Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	663: {
		Name:              "Skeleton Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	664: {
		Name:              "Crossbowman of the Kingsguard",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	667: {
		Name:              "Shadow Rogue",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	668: {
		Name:              "Shadow Felon",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	670: {
		Name:              "Norseman With Axe",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	671: {
		Name:              "Norseman With Bow",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	672: {
		Name:              "Knight of the Kingsguard",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	673: {
		Name:              "Dragon Claws",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	674: {
		Name:              "Dragon Fire",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	675: {
		Name:              "Saber Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	676: {
		Name:              "Desert Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	677: {
		Name:              "Norseman With Axe",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	678: {
		Name:              "Norseman With Bow",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	679: {
		Name:              "Cultist Fanatic",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	680: {
		Name:              "Cultist Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	684: {
		Name:              "Marauder",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	685: {
		Name:              "Pyromaniac",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	686: {
		Name:              "Sentinel of the Kingsguard",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	687: {
		Name:              "Scout of the Kingsguard",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	688: {
		Name:              "Renegade Saber Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	689: {
		Name:              "Renegade Desert Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	690: {
		Name:              "Renegade Norseman Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	691: {
		Name:              "Renegade Norseman Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	692: {
		Name:              "Renegade Cultist Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	693: {
		Name:              "Renegade Cultist Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	698: {
		Name:              "Cow Berdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	699: {
		Name:              "Longbow Ox",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	710: {
		Name:              "Bear Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	711: {
		Name:              "Bear Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	712: {
		Name:              "Lion Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	713: {
		Name:              "Lion Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	714: {
		Name:              "Demon Horror",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	715: {
		Name:              "Deathly Horror",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	716: {
		Name:              "Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	717: {
		Name:              "Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	718: {
		Name:              "Tentacle",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	719: {
		Name:              "Kraken Head",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	720: {
		Name:              "Wolfhound",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	721: {
		Name:              "Barbarian",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	722: {
		Name:              "Composite Bowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	723: {
		Name:              "Flame Bearer",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	724: {
		Name:              "Veteran Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	725: {
		Name:              "Khan Guard",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	726: {
		Name:              "Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	727: {
		Name:              "Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	728: {
		Name:              "Renegade Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	729: {
		Name:              "Renegade Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	743: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	744: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	746: {
		Name:              "Militia",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	747: {
		Name:              "Shadow Scoundrel",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	748: {
		Name:              "Shadow Wretch",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	749: {
		Name:              "Shadow Battering Ram",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	750: {
		Name:              "Shadow Siege Tower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	751: {
		Name:              "Shadow Assault Bridge",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	752: {
		Name:              "Shadow Mantlet",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	753: {
		Name:              "Demon Slayer",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	754: {
		Name:              "Assassin",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	759: {
		Name:              "Renegade Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	760: {
		Name:              "Renegade Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	765: {
		Name:              "Veteran Marauder",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	766: {
		Name:              "Veteran Pyromaniac",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	767: {
		Name:              "Renegade Shark Tooth Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	768: {
		Name:              "Renegade Stone Smasher",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	781: {
		Name:              "Master Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	782: {
		Name:              "Master Archer",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	960: {
		Name:              "Forest Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	961: {
		Name:              "Forest Hunter",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	962: {
		Name:              "Renegade Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	963: {
		Name:              "Renegade Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	// Shield-Maiden (melee/attack) - MEAD
	196: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	197: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	198: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	199: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	200: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	201: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	202: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	203: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	204: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	215: {
		Name:              "Shield-Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	// Valkyrie Ranger (range/attack) - MEAD
	206: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	207: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	208: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	209: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	210: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	211: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	212: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	213: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	214: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	216: {
		Name:              "Valkyrie Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	// Protector of the North (melee/defense) - MEAD
	218: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	219: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	220: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	221: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	222: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	223: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	224: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	225: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	226: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	227: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	489: {
		Name:              "Protector of the North",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	// Valkyrie Sniper (range/defense) - MEAD
	229: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	230: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	231: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	232: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	233: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	234: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	235: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	236: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	237: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	238: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	493: {
		Name:              "Valkyrie Sniper",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	// Forsaken Maiden - MEAD
	472: {
		Name:              "Forsaken Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	483: {
		Name:              "Forsaken Maiden",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	// Forlorn Ranger - MEAD
	473: {
		Name:              "Forlorn Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	484: {
		Name:              "Forlorn Ranger",
		ConsumptionType:   "mead",
		ConsumptionAmount: 2,
	},
	// Glasswing Archer (range/attack) - BEEF
	514: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	515: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	516: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	517: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	518: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	519: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	520: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	521: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	522: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	523: {
		Name:              "Glasswing Archer",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	// Flamebreath Berserker (melee/attack) - BEEF
	525: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	526: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	527: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	528: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	529: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	530: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	531: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	532: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	533: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	534: {
		Name:              "Flamebreath Berserker",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	// Scalebound Guardian (melee/attack) - BEEF
	536: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	537: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	538: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	539: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	540: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	541: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	542: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	543: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	544: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	545: {
		Name:              "Scalebound Guardian",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	// Scaleshard Marksman (range/attack) - BEEF
	547: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	548: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	549: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	550: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	551: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	552: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	553: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	554: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	555: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	556: {
		Name:              "Scaleshard Marksman",
		ConsumptionType:   "beef",
		ConsumptionAmount: 2,
	},
	// Swashbuckler (melee/attack) - FOOD
	701: {
		Name:              "Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	702: {
		Name:              "Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Sail Ripper (range/attack) - FOOD
	703: {
		Name:              "Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	704: {
		Name:              "Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Renegade Swashbuckler (melee/attack) - FOOD
	705: {
		Name:              "Renegade Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	706: {
		Name:              "Renegade Swashbuckler",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	// Renegade Sail Ripper (range/attack) - FOOD
	707: {
		Name:              "Renegade Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	708: {
		Name:              "Renegade Sail Ripper",
		ConsumptionType:   "food",
		ConsumptionAmount: 6,
	},
	// Katana Warrior (melee/defense) - FOOD
	820: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	821: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	822: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	823: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	824: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	825: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	826: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	827: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	828: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	829: {
		Name:              "Katana Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Bow Master (range/defense) - FOOD
	830: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	831: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	832: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	833: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	834: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	835: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	836: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	837: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	838: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	839: {
		Name:              "Bow Master",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	// Sai Warrior (melee/attack) - FOOD
	861: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	862: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	863: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	864: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	865: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	866: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	867: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	868: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	869: {
		Name:              "Sai Warrior",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Kunai Thrower (range/attack) - FOOD
	871: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	872: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	873: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	874: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	875: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	876: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	877: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	878: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	879: {
		Name:              "Kunai Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Lancer (melee/defense) - FOOD
	900: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	901: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	902: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	903: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	904: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	905: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	906: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	907: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	908: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	909: {
		Name:              "Lancer",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Spear Thrower (range/defense) - FOOD
	910: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	911: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	912: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	913: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	914: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	915: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	916: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	917: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	918: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	919: {
		Name:              "Spear Thrower",
		ConsumptionType:   "food",
		ConsumptionAmount: 0,
	},
	// Veteran Saber Cleaver (melee/attack) - FOOD
	940: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	941: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	942: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	943: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	944: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	945: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	946: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	947: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	948: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	949: {
		Name:              "Veteran Saber Cleaver",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	// Veteran Slingshot (range/attack) - FOOD
	950: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	951: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	952: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	953: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	954: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	955: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	956: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	957: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	958: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	959: {
		Name:              "Veteran Slingshot",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	// Veteran Halberdier (melee/defense) - FOOD
	2000: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2001: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2002: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2003: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2004: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2005: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2006: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2007: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2008: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2009: {
		Name:              "Veteran Halberdier",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	// Veteran Two-Handed Swordsman (melee/attack) - FOOD
	2010: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2011: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2012: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2013: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2014: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2015: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2016: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2017: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2018: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2019: {
		Name:              "Veteran Two-Handed Swordsman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	// Relic Axeman (melee/attack) - FOOD
	2020: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2021: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2022: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2023: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2024: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2025: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2026: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2027: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2028: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2029: {
		Name:              "Relic Axeman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	// Relic Hammerman (melee/defense) - FOOD
	2030: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2031: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2032: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2033: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2034: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2035: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2036: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2037: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2038: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2039: {
		Name:              "Relic Hammerman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	// Veteran Longbowman (range/defense) - FOOD
	2040: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2041: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2042: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2043: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2044: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2045: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2046: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2047: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2048: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	2049: {
		Name:              "Veteran Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 3,
	},
	// Veteran Heavy Crossbowman (range/attack) - FOOD
	2050: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2051: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2052: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2053: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2054: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2055: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2056: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2057: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2058: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2059: {
		Name:              "Veteran Heavy Crossbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	// Relic Shortbowman (range/attack) - FOOD
	2060: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2061: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2062: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2063: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2064: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2065: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2066: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2067: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2068: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	2069: {
		Name:              "Relic Shortbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 5,
	},
	// Relic Longbowman (range/defense) - FOOD
	2070: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2071: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2072: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2073: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2074: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2075: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2076: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2077: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2078: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
	2079: {
		Name:              "Relic Longbowman",
		ConsumptionType:   "food",
		ConsumptionAmount: 4,
	},
}

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
