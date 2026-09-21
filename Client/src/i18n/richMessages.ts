/** Whole-sentence templates; only application-owned React callbacks render tags. */
export const richMessages = {
  "ui.rich.equipment.components.equipmentOptimizer.max.stat.groups.receive.the.strongest.position.583645f6": "<span0>Max Stat</span0> groups receive the strongest position-decayed score. <span1>Have in Random Slots</span1> groups receive a presence bonus and lower weighted score.",
  "ui.rich.settings.components.autoBeriWorldSettingsModal.citadelops.sends.the.exact.codetext0.capacity.with.9d6cf133": "CitadelOps sends the exact <span0>{codeText0}</span0> capacity with <span1>{codeText1}</span1>. When skipping is enabled, it applies the selected <span2>{codeText2}</span2> immediately, then checks a still-travelling transfer once per minute. The selection stays saved while skipping is off.",
  "ui.rich.settings.components.autoBeriWorldSettingsModal.the.game.normally.expects.codetext0.c76ffaa1": "The game normally expects <span0>{codeText0}</span0>.",
  "ui.rich.settings.components.autoBoosterSettingsModal.independent.by.design.auto.booster.only.buys.5ea3f461": "<strong0>Independent by design.</strong0> Auto Booster only buys this one daily global effect. Auto Fortress can run without it, while enabling both is recommended for the fastest fortress marches.",
  "ui.rich.settings.components.autoFortressSettingsModal.recommended.enable.the.separate.auto.booster.feature.13180338": "<strong0>Recommended:</strong0> enable the separate Auto Booster feature to buy the {cost, number}-ruby daily global fortress-speed boost. Auto Fortress does not require, purchase, or spend rubies on that boost.",
  "ui.rich.views.attackPresetsView.the.cra.codetext0.formation.codetext1.courtyard.troops.96c662c9": "The {protocolCode} <span0>{codeText0}</span0> formation, <span1>{codeText1}</span1> courtyard troops, and <span2>{codeText2}</span2> Sceat tools are imported. Commander, source, target, travel, and other account-specific fields are ignored.",
  "ui.rich.views.automationView.right.click.a.toggle.for.temporary.activation.c34579ee": "<strong0>Right-click</strong0> a toggle for temporary activation",
  "ui.rich.views.settingsView.local.desktop.mode.applies.this.policy.directly.d237d204": "Local desktop mode applies this policy directly to <code0>{codeText0}</code0>. Feature Stats aggregates remain in <code1>{codeText1}</code1>.  Neither dataset is published to the hosted private-metrics backend; World Intelligence and report sharing remain separate features.",
  "ui.rich.views.settingsView.my.stats.uses.this.profile.s.codetext0.6d76bd61": "My Stats uses this profile's <code0>{codeText0}</code0> file, not SQLite. Estimates use saved rows or the current sample shape; troop and currency counts can change the actual size.  Finite-window maintenance rewrites the file safely and can briefly require roughly twice the displayed retained size.  Choosing a more frequent cadence affects future recordings; choosing a less-frequent cadence permanently compacts existing intermediate points.  Turning storage off removes saved history but keeps current live values available while CitadelOps is running.  Reducing the window permanently removes older points. Increasing it later cannot restore points already deleted.  This setting does not affect logs, reports, World Intelligence, or live game state."
} as const;
export const richContracts = {
  "ui.rich.equipment.components.equipmentOptimizer.max.stat.groups.receive.the.strongest.position.583645f6": {
    "arguments": [],
    "tags": [
      "span0",
      "span1"
    ]
  },
  "ui.rich.settings.components.autoBeriWorldSettingsModal.citadelops.sends.the.exact.codetext0.capacity.with.9d6cf133": {
    "arguments": [
      "codeText0",
      "codeText1",
      "codeText2"
    ],
    "tags": [
      "span0",
      "span1",
      "span2"
    ]
  },
  "ui.rich.settings.components.autoBeriWorldSettingsModal.the.game.normally.expects.codetext0.c76ffaa1": {
    "arguments": [
      "codeText0"
    ],
    "tags": [
      "span0"
    ]
  },
  "ui.rich.settings.components.autoBoosterSettingsModal.independent.by.design.auto.booster.only.buys.5ea3f461": {
    "arguments": [],
    "tags": [
      "strong0"
    ]
  },
  "ui.rich.settings.components.autoFortressSettingsModal.recommended.enable.the.separate.auto.booster.feature.13180338": {
    "arguments": [
      "cost"
    ],
    "tags": [
      "strong0"
    ]
  },
  "ui.rich.views.attackPresetsView.the.cra.codetext0.formation.codetext1.courtyard.troops.96c662c9": {
    "arguments": [
      "codeText0",
      "codeText1",
      "codeText2",
      "protocolCode"
    ],
    "tags": [
      "span0",
      "span1",
      "span2"
    ]
  },
  "ui.rich.views.automationView.right.click.a.toggle.for.temporary.activation.c34579ee": {
    "arguments": [],
    "tags": [
      "strong0"
    ]
  },
  "ui.rich.views.settingsView.local.desktop.mode.applies.this.policy.directly.d237d204": {
    "arguments": [
      "codeText0",
      "codeText1"
    ],
    "tags": [
      "code0",
      "code1"
    ]
  },
  "ui.rich.views.settingsView.my.stats.uses.this.profile.s.codetext0.6d76bd61": {
    "arguments": [
      "codeText0"
    ],
    "tags": [
      "code0"
    ]
  }
} as const;
