package decoration

import _ "embed"

// Embedded copies of client-side decoration metadata (sync from Server/Data/decorations when updating).
//
//go:embed DecoEmbed/index.json
var embeddedDecorationIndexJSON []byte

//go:embed DecoEmbed/items.json
var embeddedDecorationItemsJSON []byte
