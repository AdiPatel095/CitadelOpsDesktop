# CIT-118 catalog replay

`cit118-unit-513.json` and `cit118-currency-dga.json` are byte-preserving raw rows from `amos/Data/GameData/Items/Items-v786.03.json`, selected by wodID 513 and currencyID 69. Cost fields retain the source's numeric strings, including `costDragonGlassArrows="2"` and the absence of `costC1`.

`ProductionCost_test.go` supplies invented account/castle/operation identifiers and feeds sender-shaped currency rows through the SCE ingest pipeline. No customer payload is retained.
