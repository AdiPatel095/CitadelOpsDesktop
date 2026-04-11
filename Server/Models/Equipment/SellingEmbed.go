package equipment

import _ "embed"

// Embedded ID lists for safe-selling rules (sync from Server/Data when updating).
//
//go:embed SellingEmbed/old_red_gear.json
var embeddedOldRedGearJSON []byte

//go:embed SellingEmbed/old_red_gems.json
var embeddedOldRedGemsJSON []byte

//go:embed SellingEmbed/post_2026_gear.json
var embeddedPost2026GearJSON []byte

//go:embed SellingEmbed/post_2026_gems.json
var embeddedPost2026GemsJSON []byte
