# Auto Buyer specialist price bounds

Validated 2026-09-15 against the official Goodgame Empire HTML5 client:

- Entry: <https://empire-html5.goodgamestudios.com/default/index.html>
- `Game.bundle.ffc3a3f14892d62ec53c.js`: <https://empire-html5.goodgamestudios.com/default/Game.bundle.ffc3a3f14892d62ec53c.js>, SHA-256 `8c7ab04fa98725ffce3f7d62b83b95e7d4ac7d5fb7d1b3c305cbedce732e1288`
- `ggs.dll.593dc7f854f7e3d96d4d.js`: <https://empire-html5.goodgamestudios.com/default/dll/ggs.dll.593dc7f854f7e3d96d4d.js>, SHA-256 `e9b0ca1440595f20c106051f28f9651bda5acdfb62a925808629bd6b7a0d42d2`

The official `BoosterConst` values and constructor mapping establish conservative ruby maximums: 625 for booster IDs 0–4, 4,900 for ID 5, 990 for IDs 6 and 10, and 750 for ID 8. `CastleHeroBoosterShopVO.finalCostsC2` applies supported rebuy or sale reductions through `CastleCostsData.getFinalCostsC2`; those discounts only reduce the base value. Auto Buyer therefore uses the mapped base as a validated maximum, never as a claim about the exact charge.

The normal `PO:-1` purchase route is capture-backed. If a future client removes a mapping, introduces an additive uplift, or otherwise invalidates the bound, that specialist must become unavailable for automatic spending until the maximum is revalidated.
