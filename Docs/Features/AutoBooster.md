# Auto Booster

Auto Booster owns the optional 2,500-ruby purchase for global effect `2`, the
daily fortress travel-speed boost. Auto Fortress does not purchase this effect.

## Authoritative protocol inputs

The official Empire HTML5 client requests basic account data with `gbd` and an
empty argument list. `BasicSmartfoxClient.sendCommand` serializes that request
as a bare SmartFox extension command:

```text
%xt%<namespace>%gbd%<room>%
```

The inspected official artifact is
[`ggs.dll.593dc7f854f7e3d96d4d.js`](https://empire-html5.goodgamestudios.com/default/dll/ggs.dll.593dc7f854f7e3d96d4d.js).
Its SHA-256 digest at inspection was
`e9b0ca1440595f20c106051f28f9651bda5acdfb62a925808629bd6b7a0d42d2`.
The official GBD handler reads scalable-event inventory (`sei`), boosted global
effects (`bie`), and account resources (`gcu`). The official AGB success branch
accepts response code `0` without requiring a JSON body.

The repository fixture
[`Server/Ingest/testdata/global_effect_gbd_sanitized.json`](../../Server/Ingest/testdata/global_effect_gbd_sanitized.json)
retains only the fields needed by Auto Booster. Its identifiers and balances
are synthetic; it contains no account capture or raw log data. It documents
that availability strength (`GE[2] = 10`) and the paid booster value (`BV = 50`)
are independent fields.

## Purchase contract

Auto Booster sends `agb` with exactly `{"GEID":2}` only when one complete,
current-session GBD snapshot proves all of these conditions:

- effect `2` is active for more than 30 seconds;
- BIE explicitly contains a valid `GE` array and says the occurrence is not
  boosted;
- the live GEB quote is exactly 2,500 rubies and has a positive `BV`;
- the current authoritative ruby balance remains above the saved reserve after
  the purchase;
- Auto Booster remains enabled, Bot Lock is off, and the session, settings,
  quote, balance, and occurrence still match at the transport boundary.

Missing, null, malformed, stale, future, or prior-session evidence cannot
authorize a purchase. A successful GBD read with an unavailable effect, offer,
or valid booster section waits for the configured interval instead of creating
an immediate refresh loop.

## Durable outcomes

Before dispatch, Auto Booster synchronously records the occurrence, quote,
balance, operation identity, and an unresolved outcome. A code-zero AGB result
advances it to accepted; BIE activation advances it to confirmed. Either state
prevents another purchase for the same occurrence across reconnects and
restarts. A terminal ambiguous operation can become rejected only after a later
complete GBD snapshot proves the same occurrence inactive. A durable code-zero
AGB exchange always restores accepted state before that reconciliation.

Ruby changes observed in GBD/GCU are shown separately from debit attribution.
An equal 2,500-ruby change is useful evidence, but it does not by itself prove
that AGB caused the debit.

GBD response bodies are not copied into operation receipts because they are
large private account snapshots. Only the scoped booster observations and
purchase record are persisted and projected to the client.
