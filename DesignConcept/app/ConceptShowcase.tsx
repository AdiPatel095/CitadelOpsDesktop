"use client";

import { useMemo, useState } from "react";

type ScreenId =
  | "public"
  | "account"
  | "overview"
  | "automations"
  | "editor"
  | "execution"
  | "activity"
  | "authorize"
  | "license";

type Tone = "brand" | "success" | "warning" | "danger" | "info" | "neutral";

const screenOptions: Array<{ id: ScreenId; label: string; group: string }> = [
  { id: "public", label: "Public home", group: "Web" },
  { id: "account", label: "Account", group: "Web" },
  { id: "overview", label: "Overview", group: "Desktop" },
  { id: "automations", label: "Automations", group: "Desktop" },
  { id: "editor", label: "Editor", group: "Desktop" },
  { id: "execution", label: "Execution", group: "Desktop" },
  { id: "activity", label: "Activity", group: "Desktop" },
  { id: "authorize", label: "Authorization", group: "Account" },
  { id: "license", label: "License states", group: "Account" },
];

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div
      className={`brand-lockup ${compact ? "is-compact" : ""}`}
      aria-label="CitadelOps concept"
    >
      <span className="brand-mark" aria-hidden="true">
        <span className="mark-node" />
      </span>
      {!compact && (
        <span className="brand-type">
          <strong>Citadel</strong>
          <span>Ops</span>
        </span>
      )}
    </div>
  );
}

function Status({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return (
    <span className={`status status-${tone}`}>
      <span className="status-symbol" aria-hidden="true" />
      {children}
    </span>
  );
}

function Icon({ children }: { children: React.ReactNode }) {
  return (
    <span className="icon" aria-hidden="true">
      {children}
    </span>
  );
}

function ShellNav({ active }: { active: string }) {
  const groups = [
    {
      label: "Operations",
      items: [
        ["Overview", "⌂"],
        ["Castles", "◇"],
        ["Automations", "↻"],
      ],
    },
    {
      label: "Workspaces",
      items: [
        ["Combat", "↗"],
        ["Intelligence", "◎"],
        ["Library", "▤"],
      ],
    },
    {
      label: "System",
      items: [
        ["Activity", "≋"],
        ["Settings", "⚙"],
      ],
    },
  ];

  return (
    <aside className="app-sidebar" aria-label="Desktop navigation">
      <div className="sidebar-brand">
        <Brand />
        <span className="concept-version">2.0 concept</span>
      </div>
      <nav>
        {groups.map((group) => (
          <div className="nav-group" key={group.label}>
            <p>{group.label}</p>
            {group.items.map(([label, glyph]) => (
              <button
                className={active === label ? "active" : ""}
                type="button"
                key={label}
              >
                <Icon>{glyph}</Icon>
                <span>{label}</span>
                {label === "Activity" && <span className="nav-count">2</span>}
              </button>
            ))}
          </div>
        ))}
      </nav>
      <div className="sidebar-foot">
        <div className="mini-user">NB</div>
        <div>
          <strong>Nebula</strong>
          <span>Command plan</span>
        </div>
        <button type="button" aria-label="Account menu">
          ···
        </button>
      </div>
    </aside>
  );
}

function AppHeader({ onPause }: { onPause?: () => void }) {
  return (
    <header className="app-header">
      <button className="scope-switch" type="button">
        <span className="castle-glyph" aria-hidden="true" />
        <span>
          <small>Viewing castle</small>
          <strong>GhostTown</strong>
        </span>
        <span aria-hidden="true">⌄</span>
      </button>
      <div className="health-summary">
        <Status tone="success">Connected</Status>
        <span>Live 8s ago</span>
        <button type="button">Details</button>
      </div>
      <div className="header-actions">
        <button className="quiet-button" type="button" aria-label="Search">
          <Icon>⌕</Icon>
          <span className="desktop-only">Search</span>
          <kbd>⌘K</kbd>
        </button>
        <button className="pause-button" type="button" onClick={onPause}>
          <span aria-hidden="true">Ⅱ</span>
          Pause all
        </button>
      </div>
    </header>
  );
}

function AppFrame({
  active,
  children,
  onPause,
  dock,
}: {
  active: string;
  children: React.ReactNode;
  onPause?: () => void;
  dock?: React.ReactNode;
}) {
  return (
    <div className="desktop-frame">
      <ShellNav active={active} />
      <div className="app-stage">
        <AppHeader onPause={onPause} />
        <div className="app-content">{children}</div>
      </div>
      {dock}
    </div>
  );
}

function PublicHome({ navigate }: { navigate: (screen: ScreenId) => void }) {
  return (
    <div className="marketing-screen">
      <header className="marketing-header">
        <Brand />
        <nav aria-label="Public site">
          <a href="#product">Product</a>
          <a href="#automations">Automations</a>
          <a href="#pricing">Pricing</a>
          <a href="#docs">Docs</a>
        </nav>
        <div className="marketing-actions">
          <button type="button" onClick={() => navigate("authorize")}>
            Sign in
          </button>
          <button className="primary-button" type="button">
            Download app
          </button>
        </div>
      </header>

      <main>
        <section className="hero" id="product">
          <div className="hero-copy">
            <div className="eyebrow">
              <span /> Calm tactical control
            </div>
            <h1>
              Command complexity.
              <br />
              <em>Keep the judgment.</em>
            </h1>
            <p>
              Supervise every castle, automate within your rules, and understand
              every decision from plan to confirmed outcome.
            </p>
            <div className="hero-actions">
              <button className="primary-button large" type="button">
                Download for Windows <span aria-hidden="true">→</span>
              </button>
              <button
                className="secondary-button large"
                type="button"
                onClick={() => navigate("overview")}
              >
                Explore the command center
              </button>
            </div>
            <div className="trust-line">
              <span>✓ Scoped dry runs</span>
              <span>✓ Resource guardrails</span>
              <span>✓ Traceable outcomes</span>
            </div>
          </div>

          <div className="hero-product" aria-label="Product overview preview">
            <div className="product-window-bar">
              <span className="window-mark">
                <Brand compact />
              </span>
              <span>Empire overview</span>
              <Status tone="success">Live</Status>
            </div>
            <div className="hero-product-body">
              <div className="hero-side-rail">
                <span className="active" />
                <span />
                <span />
                <span />
              </div>
              <div className="hero-dashboard">
                <div className="preview-heading">
                  <div>
                    <small>Tuesday, July 14</small>
                    <h2>Good evening, Nebula.</h2>
                  </div>
                  <button type="button">Ⅱ Pause all</button>
                </div>
                <div className="preview-alert">
                  <div className="alert-icon">!</div>
                  <div>
                    <strong>One decision needs you</strong>
                    <span>Auto TCI is blocked at Winter Keep</span>
                  </div>
                  <button type="button" onClick={() => navigate("execution")}>
                    Review →
                  </button>
                </div>
                <div className="preview-castles">
                  <article>
                    <div className="castle-art castle-one" />
                    <span>GhostTown</span>
                    <strong>Recruiting</strong>
                    <small>Next action in 2m</small>
                  </article>
                  <article>
                    <div className="castle-art castle-two" />
                    <span>Winter Keep</span>
                    <strong className="warning-text">Blocked</strong>
                    <small>Construction item missing</small>
                  </article>
                  <article>
                    <div className="castle-art castle-three" />
                    <span>Stonewatch</span>
                    <strong>Waiting</strong>
                    <small>Slot free in 18m</small>
                  </article>
                </div>
                <div className="preview-timeline">
                  <span />
                  <span />
                  <span />
                  <span />
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className="proof-grid" id="automations">
          <article>
            <span className="proof-number">01</span>
            <Icon>◉</Icon>
            <h3>See what is true</h3>
            <p>
              Live, delayed, stale, and unknown are always distinct—and always
              timestamped.
            </p>
          </article>
          <article>
            <span className="proof-number">02</span>
            <Icon>↻</Icon>
            <h3>Automate within rules</h3>
            <p>
              Scope, reserves, shared resources, and stop conditions stay
              visible before action.
            </p>
          </article>
          <article>
            <span className="proof-number">03</span>
            <Icon>✓</Icon>
            <h3>Recover with evidence</h3>
            <p>
              Readable traces and receipts show what completed and what can
              safely resume.
            </p>
          </article>
        </section>

        <section className="how-it-works">
          <div>
            <span className="eyebrow">One accountable path</span>
            <h2>From live state to confirmed result.</h2>
          </div>
          <div className="process-line">
            {[
              ["01", "Observe", "Normalize current game state"],
              ["02", "Evaluate", "Apply policy and resource claims"],
              ["03", "Preview", "Show scope, cost, and next action"],
              ["04", "Confirm", "Record outcome and receipt"],
            ].map(([number, title, copy]) => (
              <article key={number}>
                <span>{number}</span>
                <strong>{title}</strong>
                <p>{copy}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="marketing-cta" id="pricing">
          <div>
            <span className="eyebrow">Working concept</span>
            <h2>Built for leverage without mystery.</h2>
          </div>
          <button
            className="primary-button large"
            type="button"
            onClick={() => navigate("automations")}
          >
            Tour automations →
          </button>
        </section>
      </main>

      <footer className="marketing-footer">
        <Brand />
        <p>Working name and symbol pending professional clearance.</p>
        <nav>
          <a href="#security">Security</a>
          <a href="#status">Status</a>
          <a href="#support">Support</a>
          <a href="#privacy">Privacy</a>
          <a href="#legal">Legal</a>
        </nav>
      </footer>
    </div>
  );
}

function AccountScreen({ navigate }: { navigate: (screen: ScreenId) => void }) {
  return (
    <div className="portal-screen">
      <header className="portal-header">
        <Brand />
        <div className="portal-product-switch">
          Account portal <span>⌄</span>
        </div>
        <div className="portal-header-actions">
          <button type="button">Help</button>
          <div className="mini-user">NB</div>
        </div>
      </header>
      <div className="portal-layout">
        <aside className="portal-nav">
          <p>Account</p>
          {[
            ["Overview", "⌂"],
            ["Devices", "▣"],
            ["Plan & billing", "▤"],
            ["Security & sessions", "⌾"],
            ["Downloads", "↓"],
            ["Support", "?"],
            ["Data & privacy", "◇"],
          ].map(([label, icon], index) => (
            <button
              className={index === 0 ? "active" : ""}
              type="button"
              key={label}
            >
              <Icon>{icon}</Icon>
              {label}
            </button>
          ))}
          <button
            className="back-to-app"
            type="button"
            onClick={() => navigate("overview")}
          >
            ← Open desktop concept
          </button>
        </aside>
        <main className="portal-main">
          <div className="page-title-row">
            <div>
              <span className="eyebrow">Account</span>
              <h1>Good evening, Nebula.</h1>
              <p>Your plan, devices, and security at a glance.</p>
            </div>
            <button className="primary-button" type="button">
              Download latest
            </button>
          </div>

          <section className="account-grid">
            <article className="plan-card featured-card">
              <div className="card-topline">
                <span>Current plan</span>
                <Status tone="success">Active</Status>
              </div>
              <h2>Command</h2>
              <p>All current desktop automation and intelligence features.</p>
              <dl className="plan-facts">
                <div>
                  <dt>Renews</dt>
                  <dd>Aug 15, 2026</dd>
                </div>
                <div>
                  <dt>Next charge</dt>
                  <dd>$14.00 total</dd>
                </div>
                <div>
                  <dt>Devices</dt>
                  <dd>2 of 3 used</dd>
                </div>
              </dl>
              <div className="card-actions">
                <button className="secondary-button" type="button">
                  Manage plan
                </button>
                <button
                  className="text-button"
                  type="button"
                  onClick={() => navigate("license")}
                >
                  View state examples
                </button>
              </div>
            </article>

            <article className="account-status-card">
              <div className="card-topline">
                <span>Account health</span>
                <span className="score-ring">A</span>
              </div>
              <h3>No action needed</h3>
              <ul className="check-list">
                <li>
                  <span>✓</span> Passkey configured
                </li>
                <li>
                  <span>✓</span> Recovery codes saved
                </li>
                <li>
                  <span>✓</span> Payment current
                </li>
              </ul>
              <button className="text-button" type="button">
                Review security →
              </button>
            </article>
          </section>

          <section className="portal-section">
            <div className="section-heading-row">
              <div>
                <h2>Licensed devices</h2>
                <p>Two of three device slots are in use.</p>
              </div>
              <button className="secondary-button" type="button">
                Manage devices
              </button>
            </div>
            <div
              className="device-table"
              role="table"
              aria-label="Licensed devices"
            >
              <div className="device-row device-head" role="row">
                <span>Device</span>
                <span>Version</span>
                <span>Last active</span>
                <span>Status</span>
                <span />
              </div>
              <div className="device-row" role="row">
                <div className="device-name">
                  <span className="device-icon">▣</span>
                  <div>
                    <strong>Nebula-PC</strong>
                    <small>Windows 11 · Current device</small>
                  </div>
                </div>
                <span>2.0.0</span>
                <span>Now</span>
                <Status tone="success">Current</Status>
                <button type="button">···</button>
              </div>
              <div className="device-row" role="row">
                <div className="device-name">
                  <span className="device-icon">▣</span>
                  <div>
                    <strong>Travel laptop</strong>
                    <small>Windows 11</small>
                  </div>
                </div>
                <span>1.9.8</span>
                <span>31 days ago</span>
                <Status tone="warning">Update</Status>
                <button type="button">Revoke</button>
              </div>
            </div>
          </section>

          <section className="portal-section release-strip">
            <div className="release-icon">↓</div>
            <div>
              <span>Latest stable release</span>
              <strong>CitadelOps 2.0.0</strong>
              <small>Published July 11 · Signed for Windows</small>
            </div>
            <button className="secondary-button" type="button">
              Release notes
            </button>
          </section>
        </main>
      </div>
    </div>
  );
}

function OverviewScreen({
  navigate,
}: {
  navigate: (screen: ScreenId) => void;
}) {
  const [paused, setPaused] = useState(false);

  return (
    <AppFrame active="Overview" onPause={() => setPaused((value) => !value)}>
      <div className="desktop-page overview-page">
        {paused && (
          <div className="pause-banner" role="status">
            <span aria-hidden="true">Ⅱ</span>
            <div>
              <strong>New automation work is paused</strong>
              <small>
                One active operation will stop after its current safe step.
              </small>
            </div>
            <button type="button" onClick={() => setPaused(false)}>
              Resume all
            </button>
          </div>
        )}
        <div className="page-title-row compact-title">
          <div>
            <span className="eyebrow">Tuesday, July 14</span>
            <h1>Empire overview</h1>
            <p>
              Everything that needs judgment, then everything already under
              control.
            </p>
          </div>
          <div className="overview-stat">
            <small>Automations</small>
            <strong>
              9 <span>enabled</span>
            </strong>
            <Status tone="warning">2 need attention</Status>
          </div>
        </div>

        <section className="attention-panel">
          <div className="section-heading-row">
            <div>
              <span className="eyebrow">Attention</span>
              <h2>Two decisions need you</h2>
            </div>
            <button className="text-button" type="button">
              View attention queue →
            </button>
          </div>
          <div className="attention-list">
            <article className="attention-item danger-line">
              <div className="attention-icon">!</div>
              <div className="attention-copy">
                <div>
                  <strong>Auto TCI cannot continue at Winter Keep</strong>
                  <Status tone="danger">Blocked</Status>
                </div>
                <p>
                  The required construction item is no longer available. No
                  currency was spent.
                </p>
                <small>Winter Keep · Auto TCI · first seen 6m ago</small>
              </div>
              <button
                className="secondary-button"
                type="button"
                onClick={() => navigate("execution")}
              >
                Review recovery
              </button>
            </article>
            <article className="attention-item warning-line">
              <div className="attention-icon">◷</div>
              <div className="attention-copy">
                <div>
                  <strong>Food reserve may be crossed in 34 minutes</strong>
                  <Status tone="warning">Review</Status>
                </div>
                <p>
                  Auto Recruit is waiting for three inbound movements before it
                  reevaluates.
                </p>
                <small>Stonewatch · Auto Recruit · updated 18s ago</small>
              </div>
              <button className="secondary-button" type="button">
                Review projection
              </button>
            </article>
          </div>
        </section>

        <section className="castle-section">
          <div className="section-heading-row">
            <div>
              <h2>Castles</h2>
              <p>Current work, constraint, and next expected transition.</p>
            </div>
            <button className="text-button" type="button">
              View all castles →
            </button>
          </div>
          <div className="castle-grid">
            {[
              {
                name: "GhostTown",
                status: "Healthy",
                tone: "success" as Tone,
                mode: "Auto Recruit",
                detail: "Training defensive veterans",
                next: "Confirms in about 2m",
                art: "castle-one",
                value: "82%",
                label: "Food reserve",
              },
              {
                name: "Winter Keep",
                status: "Blocked",
                tone: "danger" as Tone,
                mode: "Auto TCI",
                detail: "Construction item unavailable",
                next: "Needs your decision",
                art: "castle-two",
                value: "1 / 3",
                label: "Build slots free",
              },
              {
                name: "Stonewatch",
                status: "Waiting",
                tone: "neutral" as Tone,
                mode: "Auto Food",
                detail: "Watching inbound movement",
                next: "Rechecks in 3m 42s",
                art: "castle-three",
                value: "18m",
                label: "Next slot",
              },
            ].map((castle) => (
              <article className="castle-card" key={castle.name}>
                <div className={`castle-card-art ${castle.art}`}>
                  <Status tone={castle.tone}>{castle.status}</Status>
                </div>
                <div className="castle-card-body">
                  <div>
                    <span>{castle.name}</span>
                    <button type="button" aria-label={`${castle.name} menu`}>
                      ···
                    </button>
                  </div>
                  <strong>{castle.mode}</strong>
                  <p>{castle.detail}</p>
                  <div className="next-line">
                    <span aria-hidden="true">→</span>
                    <small>{castle.next}</small>
                  </div>
                  <div className="castle-metric">
                    <span>{castle.label}</span>
                    <strong>{castle.value}</strong>
                  </div>
                </div>
              </article>
            ))}
          </div>
        </section>

        <section className="overview-bottom-grid">
          <article className="upcoming-card">
            <div className="section-heading-row">
              <div>
                <h2>Upcoming</h2>
                <p>Next 45 minutes</p>
              </div>
              <button type="button">Timeline</button>
            </div>
            <div className="upcoming-list">
              <div>
                <time>Now</time>
                <span className="timeline-node success-node" />
                <div>
                  <strong>Recruitment confirming</strong>
                  <small>GhostTown · Auto Recruit</small>
                </div>
                <Status tone="success">Running</Status>
              </div>
              <div>
                <time>18m</time>
                <span className="timeline-node" />
                <div>
                  <strong>Construction slot opens</strong>
                  <small>Stonewatch · Auto TCI</small>
                </div>
                <Status tone="neutral">Expected</Status>
              </div>
              <div>
                <time>34m</time>
                <span className="timeline-node warning-node" />
                <div>
                  <strong>Food reserve threshold</strong>
                  <small>Stonewatch · Auto Food</small>
                </div>
                <Status tone="warning">Projected</Status>
              </div>
            </div>
          </article>
          <article className="recent-card">
            <div className="section-heading-row">
              <div>
                <h2>Recent outcomes</h2>
                <p>Significant activity only</p>
              </div>
              <button
                className="text-button"
                type="button"
                onClick={() => navigate("activity")}
              >
                All activity →
              </button>
            </div>
            <div className="receipt-list">
              <div>
                <span className="receipt-icon">✓</span>
                <div>
                  <strong>Workshop upgraded to level 7</strong>
                  <small>GhostTown · 12,500 silver · 3m ago</small>
                </div>
              </div>
              <div>
                <span className="receipt-icon">↻</span>
                <div>
                  <strong>Three movements reconciled</strong>
                  <small>Empire · confirmed · 11m ago</small>
                </div>
              </div>
              <div>
                <span className="receipt-icon">◇</span>
                <div>
                  <strong>Attack preset revised</strong>
                  <small>Nebula · “Nomad balanced” · 28m ago</small>
                </div>
              </div>
            </div>
          </article>
        </section>
      </div>
      <button
        className="dock-handle"
        type="button"
        onClick={() => navigate("activity")}
      >
        <span>2</span> Activity
      </button>
    </AppFrame>
  );
}

const automationRows = [
  {
    name: "Auto TCI",
    glyph: "▥",
    tone: "warning" as Tone,
    state: "Waiting",
    scope: "All castles · 4 enabled",
    detail: "Next: upgrade Bakery when slot 2 is free",
    next: "about 18m",
    issue: "1 blocked dependency",
    outcome: "Workshop upgraded · 3m ago",
  },
  {
    name: "Auto Recruit",
    glyph: "♙",
    tone: "success" as Tone,
    state: "Running",
    scope: "GhostTown, Stonewatch",
    detail: "Training defensive veterans · step 2 of 4",
    next: "confirms in ~8s",
    issue: "",
    outcome: "2,840 units trained today",
  },
  {
    name: "Auto Food Balance",
    glyph: "◒",
    tone: "neutral" as Tone,
    state: "Waiting",
    scope: "3 selected castles",
    detail: "Watching reserves and inbound movements",
    next: "rechecks in 3m 42s",
    issue: "",
    outcome: "No transfers needed · 12m ago",
  },
  {
    name: "Auto Hospital",
    glyph: "+",
    tone: "neutral" as Tone,
    state: "Scheduled",
    scope: "All castles",
    detail: "Next recovery window begins at 11:00 PM",
    next: "in 41m",
    issue: "",
    outcome: "312 units recovered · 1h ago",
  },
  {
    name: "Equipment Cleanup",
    glyph: "◇",
    tone: "brand" as Tone,
    state: "Paused",
    scope: "Account inventory",
    detail: "Paused by Nebula after last safe step",
    next: "manual resume",
    issue: "",
    outcome: "18 items reviewed · yesterday",
  },
];

function AutomationsScreen({
  navigate,
}: {
  navigate: (screen: ScreenId) => void;
}) {
  const [filter, setFilter] = useState("All");
  const filteredRows = useMemo(() => {
    if (filter === "All") return automationRows;
    if (filter === "Attention")
      return automationRows.filter((row) => row.issue);
    return automationRows.filter((row) => row.state === filter);
  }, [filter]);

  return (
    <AppFrame active="Automations">
      <div className="desktop-page automations-page">
        <div className="page-title-row compact-title">
          <div>
            <span className="eyebrow">Control center</span>
            <h1>Automations</h1>
            <p>
              Eligibility, current work, constraints, and the next expected
              action.
            </p>
          </div>
          <button
            className="primary-button"
            type="button"
            onClick={() => navigate("editor")}
          >
            + Create automation
          </button>
        </div>

        <section className="automation-summary">
          <div>
            <span className="summary-icon brand-summary">↻</span>
            <strong>9</strong>
            <small>Enabled</small>
          </div>
          <div>
            <span className="summary-icon success-summary">▶</span>
            <strong>1</strong>
            <small>Running</small>
          </div>
          <div>
            <span className="summary-icon neutral-summary">◷</span>
            <strong>6</strong>
            <small>Waiting</small>
          </div>
          <div>
            <span className="summary-icon danger-summary">!</span>
            <strong>2</strong>
            <small>Need attention</small>
          </div>
          <div className="summary-note">
            <span>Shared resources</span>
            <strong>No commander conflicts</strong>
            <button type="button">View allocation →</button>
          </div>
        </section>

        <section className="automation-table-card">
          <div className="automation-toolbar">
            <div className="filter-tabs" aria-label="Automation filter">
              {["All", "Attention", "Running", "Waiting"].map((item) => (
                <button
                  className={filter === item ? "active" : ""}
                  type="button"
                  onClick={() => setFilter(item)}
                  key={item}
                >
                  {item}
                  {item === "Attention" && <span>2</span>}
                </button>
              ))}
            </div>
            <div className="toolbar-actions">
              <button type="button">
                <Icon>⌕</Icon> Search
              </button>
              <button type="button">Status & scope ⌄</button>
              <button type="button">Compact ⌄</button>
            </div>
          </div>
          <div className="automation-column-head">
            <span>Automation</span>
            <span>Current state and next action</span>
            <span>Last outcome</span>
            <span>Controls</span>
          </div>
          <div className="automation-rows">
            {filteredRows.map((row) => (
              <article
                className={`automation-row ${row.issue ? "has-issue" : ""}`}
                key={row.name}
              >
                <div className="automation-identity">
                  <span className="automation-icon">{row.glyph}</span>
                  <div>
                    <strong>{row.name}</strong>
                    <small>{row.scope}</small>
                  </div>
                </div>
                <div className="automation-state">
                  <div>
                    <Status tone={row.tone}>{row.state}</Status>
                    <strong>{row.detail}</strong>
                  </div>
                  <p>
                    <span aria-hidden="true">→</span> {row.next}
                    {row.issue && (
                      <button
                        type="button"
                        onClick={() => navigate("execution")}
                      >
                        {row.issue} →
                      </button>
                    )}
                  </p>
                </div>
                <div className="automation-outcome">
                  <strong>{row.outcome}</strong>
                  <small>Confirmed by game</small>
                </div>
                <div className="automation-controls">
                  <button type="button" aria-label={`Pause ${row.name}`}>
                    Ⅱ
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      row.name === "Auto TCI"
                        ? navigate("execution")
                        : undefined
                    }
                  >
                    Inspect
                  </button>
                  <button type="button" aria-label={`${row.name} settings`}>
                    ⚙
                  </button>
                </div>
              </article>
            ))}
          </div>
        </section>
      </div>
      <button
        className="dock-handle"
        type="button"
        onClick={() => navigate("activity")}
      >
        <span>2</span> Activity
      </button>
    </AppFrame>
  );
}

function EditorScreen({ navigate }: { navigate: (screen: ScreenId) => void }) {
  const [step, setStep] = useState(4);
  const [tested, setTested] = useState(false);
  const steps = [
    "When",
    "If",
    "Then",
    "Scope & resources",
    "Limits & stop",
    "Failure behavior",
  ];
  return (
    <AppFrame active="Automations">
      <div className="editor-page">
        <div className="editor-topbar">
          <button type="button" onClick={() => navigate("automations")}>
            ← Back
          </button>
          <div>
            <span className="eyebrow">Automation editor</span>
            <h1>
              Auto TCI <span>Draft v9</span>
            </h1>
          </div>
          <div className="editor-actions">
            <span>Live v8</span>
            <button className="secondary-button" type="button">
              Save draft
            </button>
            <button className="primary-button" type="button">
              Review & deploy
            </button>
          </div>
        </div>
        <div className="editor-layout">
          <aside className="editor-steps">
            <p>Definition</p>
            {steps.map((label, index) => (
              <button
                className={step === index + 1 ? "active" : ""}
                type="button"
                onClick={() => setStep(index + 1)}
                key={label}
              >
                <span>{index + 1}</span>
                <div>
                  <strong>{label}</strong>
                  <small>
                    {index < 3
                      ? "Configured"
                      : index === 3
                        ? "3 castles"
                        : "Review"}
                  </small>
                </div>
                {index < 4 && <b>✓</b>}
              </button>
            ))}
            <div className="editor-help">
              <Icon>?</Icon>
              <div>
                <strong>Why these steps?</strong>
                <small>See automation concepts</small>
              </div>
            </div>
          </aside>
          <main className="editor-form">
            <div className="form-heading">
              <span>Step {step} of 6</span>
              <h2>{steps[step - 1]}</h2>
              <p>
                Choose exactly where this policy can act and which resources it
                may claim.
              </p>
            </div>
            <section className="form-section">
              <div className="field-heading">
                <div>
                  <h3>Castle scope</h3>
                  <p>Only selected castles will be evaluated.</p>
                </div>
                <button type="button">Select all</button>
              </div>
              <div className="scope-list">
                {["GhostTown", "Winter Keep", "Stonewatch"].map(
                  (castle, index) => (
                    <label key={castle}>
                      <input type="checkbox" defaultChecked />
                      <span className={`castle-thumb castle-${index + 1}`} />
                      <div>
                        <strong>{castle}</strong>
                        <small>
                          {index === 1
                            ? "1 blocked dependency"
                            : "Live state available"}
                        </small>
                      </div>
                      <Status tone={index === 1 ? "warning" : "success"}>
                        {index === 1 ? "Review" : "Ready"}
                      </Status>
                    </label>
                  ),
                )}
              </div>
            </section>
            <section className="form-section resource-fields">
              <div className="field-heading">
                <div>
                  <h3>Resource boundaries</h3>
                  <p>Hard limits are enforced before every action.</p>
                </div>
              </div>
              <label>
                <span>Silver budget per hour</span>
                <div className="number-input">
                  <span>≤</span>
                  <input
                    defaultValue="25,000"
                    aria-label="Silver budget per hour"
                  />
                  <b>silver</b>
                </div>
                <small>Current dry run uses up to 12,500.</small>
              </label>
              <label>
                <span>Premium currency</span>
                <button className="prohibited-select" type="button">
                  <Status tone="neutral">Prohibited</Status>
                  <span>⌄</span>
                </button>
                <small>No automation purchase can use premium currency.</small>
              </label>
            </section>
          </main>
          <aside className="live-preview">
            <div className="preview-status">
              <span className="eyebrow">Live preview</span>
              <Status tone="success">State live 6s ago</Status>
            </div>
            <h2>Likely next hour</h2>
            <div className="preview-stat-grid">
              <div>
                <strong>3</strong>
                <span>Castles in scope</span>
              </div>
              <div>
                <strong>1</strong>
                <span>Expected action</span>
              </div>
              <div>
                <strong>1</strong>
                <span>Blocked item</span>
              </div>
              <div>
                <strong>12.5k</strong>
                <span>Maximum silver</span>
              </div>
            </div>
            <div className="dry-run-list">
              <div>
                <span>1</span>
                <p>
                  <strong>GhostTown</strong>Equip TCI 418 in Bakery slot 2
                </p>
                <Status tone="success">Ready</Status>
              </div>
              <div>
                <span>2</span>
                <p>
                  <strong>Winter Keep</strong>Wait; required item is unavailable
                </p>
                <Status tone="warning">Blocked</Status>
              </div>
              <div>
                <span>3</span>
                <p>
                  <strong>Stonewatch</strong>No action; reserve would be crossed
                </p>
                <Status tone="neutral">Guarded</Status>
              </div>
            </div>
            {tested ? (
              <div className="test-result" role="status">
                <span>✓</span>
                <div>
                  <strong>Dry test passed with one blocker</strong>
                  <small>No game commands were sent.</small>
                </div>
              </div>
            ) : (
              <button
                className="primary-button full"
                type="button"
                onClick={() => setTested(true)}
              >
                Run dry test
              </button>
            )}
            <button className="text-button centered" type="button">
              View exact intent plan →
            </button>
          </aside>
        </div>
      </div>
    </AppFrame>
  );
}

function ExecutionScreen({
  navigate,
}: {
  navigate: (screen: ScreenId) => void;
}) {
  const [technical, setTechnical] = useState(false);
  return (
    <AppFrame active="Activity">
      <div className="desktop-page execution-page">
        <button
          className="back-link"
          type="button"
          onClick={() => navigate("automations")}
        >
          ← Automations
        </button>
        <div className="execution-heading">
          <div className="execution-title-icon">▥</div>
          <div>
            <span className="eyebrow">Execution 8F2A · Auto TCI v8</span>
            <h1>Upgrade at Winter Keep</h1>
            <p>Started 10:41:22 PM · based on live state revision 18,294</p>
          </div>
          <Status tone="danger">Failed safely</Status>
        </div>
        <div className="execution-summary-strip">
          <div>
            <span>Outcome</span>
            <strong>No currency spent</strong>
          </div>
          <div>
            <span>Scope</span>
            <strong>Winter Keep only</strong>
          </div>
          <div>
            <span>Duration</span>
            <strong>18.4 seconds</strong>
          </div>
          <div>
            <span>Other automations</span>
            <strong>Still active</strong>
          </div>
        </div>

        <div className="execution-grid">
          <section className="trace-card">
            <div className="section-heading-row">
              <div>
                <span className="eyebrow">Execution trace</span>
                <h2>Four evaluated steps</h2>
              </div>
              <button
                type="button"
                onClick={() => setTechnical((value) => !value)}
              >
                {technical ? "Hide" : "Show"} technical IDs
              </button>
            </div>
            <div className="trace-list">
              {[
                {
                  state: "success",
                  title: "Reserve condition passed",
                  copy: "Silver remains above the configured 18,000 reserve.",
                  time: "10:41:22",
                  id: "condition.reserve.01",
                },
                {
                  state: "success",
                  title: "Construction slot confirmed free",
                  copy: "Slot 2 was available at state revision 18,294.",
                  time: "10:41:24",
                  id: "claim.build-slot.02",
                },
                {
                  state: "success",
                  title: "Construction item equipped",
                  copy: "TCI 418 equipped and acknowledged by the game.",
                  time: "10:41:29",
                  id: "operation.equip.03",
                },
                {
                  state: "danger",
                  title: "Upgrade rejected by game",
                  copy: "The item was no longer eligible when confirmation arrived.",
                  time: "10:41:40",
                  id: "operation.upgrade.04",
                },
              ].map((item, index) => (
                <article
                  className={`trace-step trace-${item.state}`}
                  key={item.title}
                >
                  <div className="trace-marker">
                    {item.state === "success" ? "✓" : "×"}
                  </div>
                  <div>
                    <div className="trace-title">
                      <span>Step {index + 1}</span>
                      <strong>{item.title}</strong>
                      <time>{item.time}</time>
                    </div>
                    <p>{item.copy}</p>
                    {technical && <code>{item.id}</code>}
                  </div>
                </article>
              ))}
            </div>
            <div className="trace-footer">
              <button
                className="secondary-button"
                type="button"
                onClick={() => navigate("activity")}
              >
                View related activity
              </button>
              <button
                className="text-button"
                type="button"
                onClick={() => setTechnical(true)}
              >
                Technical details →
              </button>
            </div>
          </section>

          <aside className="recovery-card">
            <div className="recovery-icon">↻</div>
            <span className="eyebrow">Recommended recovery</span>
            <h2>Refresh state, then retry only the failed safe step.</h2>
            <p>
              The equipped item and all completed checks will not repeat. The
              plan will be recalculated against fresh state before anything is
              sent.
            </p>
            <div className="recovery-will">
              <div>
                <span>Will repeat</span>
                <strong>Eligibility check · upgrade request</strong>
              </div>
              <div>
                <span>Will not repeat</span>
                <strong>Equip item · reserve claim</strong>
              </div>
              <div>
                <span>Maximum cost</span>
                <strong>12,500 silver · 0 premium</strong>
              </div>
            </div>
            <button className="primary-button full" type="button">
              Preview recovery plan
            </button>
            <button className="secondary-button full" type="button">
              Stop and review manually
            </button>
            <small>
              Retry is unavailable until a fresh game state is confirmed.
            </small>
          </aside>
        </div>
      </div>
    </AppFrame>
  );
}

function ActivityScreen() {
  const [tab, setTab] = useState<"activity" | "logs">("activity");
  const dock = (
    <aside className="activity-dock" aria-label="Activity and diagnostics">
      <div className="dock-heading">
        <div>
          <span className="eyebrow">Evidence</span>
          <h2>Activity</h2>
        </div>
        <button type="button" aria-label="Close activity">
          ×
        </button>
      </div>
      <div className="dock-tabs">
        <button
          className={tab === "activity" ? "active" : ""}
          type="button"
          onClick={() => setTab("activity")}
        >
          Readable activity
        </button>
        <button
          className={tab === "logs" ? "active" : ""}
          type="button"
          onClick={() => setTab("logs")}
        >
          Diagnostics
        </button>
      </div>
      <div className="dock-filter">
        <button type="button">
          <Icon>⌕</Icon> Search evidence
        </button>
        <button type="button">Auto TCI ⌄</button>
        <button type="button">Last hour ⌄</button>
      </div>
      {tab === "activity" ? (
        <div className="dock-events">
          <div className="event-date">Today · July 14</div>
          {[
            {
              time: "10:42",
              tone: "danger",
              icon: "!",
              title: "Upgrade rejected at Winter Keep",
              copy: "Auto TCI stopped safely. No currency was spent.",
              meta: "Operation 8F2A · Auto TCI",
            },
            {
              time: "10:41",
              tone: "success",
              icon: "✓",
              title: "Construction item equipped",
              copy: "TCI 418 was equipped in slot 2 and confirmed by the game.",
              meta: "Winter Keep · Auto TCI",
            },
            {
              time: "10:39",
              tone: "warning",
              icon: "◷",
              title: "Auto Recruit is waiting",
              copy: "Food reserve would fall below 18,000 at Stonewatch.",
              meta: "Next check in 3m 42s",
            },
            {
              time: "10:31",
              tone: "brand",
              icon: "◇",
              title: "Attack preset revised",
              copy: "Nebula saved version 12 of “Nomad balanced.”",
              meta: "User action · Preset library",
            },
          ].map((event) => (
            <article
              className={`dock-event event-${event.tone}`}
              key={event.title}
            >
              <time>{event.time}</time>
              <span className="event-icon">{event.icon}</span>
              <div>
                <strong>{event.title}</strong>
                <p>{event.copy}</p>
                <small>{event.meta}</small>
              </div>
              <button type="button">···</button>
            </article>
          ))}
          <div className="new-events">
            <span>12 new events available</span>
            <button type="button">Jump to latest ↓</button>
          </div>
        </div>
      ) : (
        <div className="diagnostic-list">
          <div className="diagnostic-note">
            <Icon>◇</Icon>
            <p>
              Technical evidence is prefiltered to Auto TCI. Secrets are
              redacted by default.
            </p>
          </div>
          {[
            [
              "22:42:01.883",
              "ERROR",
              "autotci",
              "upgrade rejected · operation=8F2A · castle=winter-keep",
            ],
            [
              "22:41:59.124",
              "RECV",
              "game",
              "response status=17 · command=ubc · correlation=8F2A",
            ],
            [
              "22:41:58.447",
              "SEND",
              "autotci",
              "upgrade request · cid=418 · slot=2 · payload redacted",
            ],
            [
              "22:41:49.210",
              "INFO",
              "intent",
              "equip confirmed · revision=18294 · claim released",
            ],
            [
              "22:41:46.008",
              "DEBUG",
              "state",
              "construction snapshot updated · 4 fields changed",
            ],
          ].map(([time, level, source, message]) => (
            <article className="diagnostic-row" key={`${time}-${level}`}>
              <time>{time}</time>
              <span className={`level level-${level.toLowerCase()}`}>
                {level}
              </span>
              <strong>{source}</strong>
              <code>{message}</code>
              <button type="button">Copy</button>
            </article>
          ))}
        </div>
      )}
      <div className="dock-footer">
        <span>800 retained · live tail paused while reading</span>
        <button type="button">Export redacted bundle</button>
      </div>
    </aside>
  );

  return (
    <AppFrame active="Activity" dock={dock}>
      <div className="desktop-page activity-page">
        <div className="page-title-row compact-title">
          <div>
            <span className="eyebrow">Operations</span>
            <h1>Activity & evidence</h1>
            <p>
              Human-readable outcomes first. Technical records remain one level
              deeper.
            </p>
          </div>
          <button className="secondary-button" type="button">
            Create support bundle
          </button>
        </div>
        <section className="activity-main-card">
          <div className="section-heading-row">
            <div>
              <h2>Selected operation</h2>
              <p>Auto TCI · Winter Keep · 8F2A</p>
            </div>
            <Status tone="danger">Failed safely</Status>
          </div>
          <div className="operation-map">
            <div className="map-step done">
              <span>✓</span>
              <strong>Conditions</strong>
              <small>Passed</small>
            </div>
            <i />
            <div className="map-step done">
              <span>✓</span>
              <strong>Equip</strong>
              <small>Confirmed</small>
            </div>
            <i />
            <div className="map-step failed">
              <span>×</span>
              <strong>Upgrade</strong>
              <small>Rejected</small>
            </div>
            <i />
            <div className="map-step">
              <span>4</span>
              <strong>Receipt</strong>
              <small>Recorded</small>
            </div>
          </div>
          <div className="operation-summary">
            <div>
              <span>Cause</span>
              <strong>Item eligibility changed before confirmation.</strong>
            </div>
            <div>
              <span>Consequence</span>
              <strong>No currency spent. Other castles remain active.</strong>
            </div>
            <div>
              <span>Next action</span>
              <strong>Refresh state and preview failed-step recovery.</strong>
            </div>
          </div>
        </section>
        <section className="activity-main-card muted-card">
          <h2>The evidence dock remembers context</h2>
          <p>
            Open Activity from an automation, attention item, or notification
            and it carries the actor, castle, operation, and absolute incident
            time automatically.
          </p>
          <div className="context-chips">
            <span>Auto TCI</span>
            <span>Winter Keep</span>
            <span>Operation 8F2A</span>
            <span>10:41–10:42 PM EDT</span>
          </div>
        </section>
      </div>
    </AppFrame>
  );
}

function AuthorizationScreen({
  navigate,
}: {
  navigate: (screen: ScreenId) => void;
}) {
  const [authorized, setAuthorized] = useState(false);
  return (
    <div className="auth-screen">
      <div className="auth-backdrop-grid" aria-hidden="true" />
      <header className="auth-header">
        <Brand />
        <button type="button">Help</button>
      </header>
      <main className="auth-center">
        <section className="auth-card">
          {!authorized ? (
            <>
              <div className="device-connection">
                <span className="web-node">◎</span>
                <i />
                <span className="desktop-node">
                  <Brand compact />
                </span>
              </div>
              <span className="eyebrow">Desktop authorization</span>
              <h1>Sign in on Nebula-PC</h1>
              <p>CitadelOps Desktop 2.0.0 · Windows 11</p>
              <div className="permission-box">
                <Icon>◇</Icon>
                <div>
                  <strong>This allows the desktop app to:</strong>
                  <ul>
                    <li>
                      Access your Citadel account and current entitlements
                    </li>
                    <li>Register Nebula-PC as one licensed device</li>
                    <li>Open the account portal in your browser</li>
                  </ul>
                  <small>Your game password is not sent to this website.</small>
                </div>
              </div>
              <div className="account-choice">
                <div className="mini-user">NB</div>
                <div>
                  <strong>Nebula</strong>
                  <small>account@example.com · 1 slot available</small>
                </div>
                <Status tone="success">Signed in</Status>
              </div>
              <button
                className="primary-button full large"
                type="button"
                onClick={() => setAuthorized(true)}
              >
                Authorize this device
              </button>
              <div className="auth-alternatives">
                <button type="button">Use another account</button>
                <span>·</span>
                <button type="button">Cancel</button>
              </div>
              <a href="#one-time-code">Having trouble? Use a one-time code</a>
            </>
          ) : (
            <div className="auth-success" role="status">
              <div className="success-orbit">
                <span>✓</span>
              </div>
              <span className="eyebrow">Device authorized</span>
              <h1>Nebula-PC is ready.</h1>
              <p>
                The desktop app now has your current Command entitlement. You
                can close this page or return to the concept.
              </p>
              <button
                className="primary-button full large"
                type="button"
                onClick={() => navigate("overview")}
              >
                Open CitadelOps concept
              </button>
              <button className="secondary-button full" type="button">
                Manage licensed devices
              </button>
            </div>
          )}
        </section>
        <div className="auth-trust">
          <span>◈ Encrypted browser handoff</span>
          <span>◇ Passkey protected</span>
          <span>✓ 1 device slot available</span>
        </div>
      </main>
      <footer className="auth-footer">
        <span>Security</span>
        <span>Privacy</span>
        <span>Support</span>
        <span>Status</span>
      </footer>
    </div>
  );
}

function LicenseScreen() {
  const [state, setState] = useState<"past-due" | "unavailable">("past-due");
  return (
    <div className="license-screen">
      <header className="portal-header">
        <Brand />
        <div className="portal-product-switch">
          Account portal <span>⌄</span>
        </div>
        <div className="portal-header-actions">
          <button type="button">Status</button>
          <div className="mini-user">NB</div>
        </div>
      </header>
      <div className="license-demo-bar">
        <div>
          <span className="eyebrow">State prototype</span>
          <strong>Compare two conditions that must never look identical</strong>
        </div>
        <div className="filter-tabs">
          <button
            className={state === "past-due" ? "active" : ""}
            type="button"
            onClick={() => setState("past-due")}
          >
            Payment past due
          </button>
          <button
            className={state === "unavailable" ? "active" : ""}
            type="button"
            onClick={() => setState("unavailable")}
          >
            Service unavailable
          </button>
        </div>
      </div>
      <main className="license-state-wrap">
        {state === "past-due" ? (
          <section className="license-state-card past-due-card">
            <div className="license-state-icon">!</div>
            <Status tone="warning">Payment needs attention</Status>
            <h1>Update payment before July 21.</h1>
            <p>
              Your Command plan is in grace. This is a billing issue—not a game
              or desktop connection failure.
            </p>
            <div className="license-facts">
              <div>
                <span>Amount due</span>
                <strong>$14.00 total</strong>
              </div>
              <div>
                <span>Grace ends</span>
                <strong>Jul 21 · 11:59 PM EDT</strong>
              </div>
              <div>
                <span>Device access</span>
                <strong>2 devices remain active</strong>
              </div>
            </div>
            <div className="access-policy">
              <h2>What happens next</h2>
              <div>
                <span className="policy-now">Now</span>
                <p>
                  <strong>Everything remains available.</strong>Your saved data,
                  history, and active automation stay visible.
                </p>
              </div>
              <div>
                <span className="policy-later">Jul 21</span>
                <p>
                  <strong>New automation launches pause.</strong>Read, export,
                  diagnostics, pause, and support remain available.
                </p>
              </div>
            </div>
            <div className="license-actions">
              <button className="primary-button large" type="button">
                Update payment
              </button>
              <button className="secondary-button large" type="button">
                View invoice
              </button>
            </div>
            <small>
              We will retry the saved payment method on July 17. You can cancel
              renewal from Plan & billing.
            </small>
          </section>
        ) : (
          <section className="license-state-card unavailable-card">
            <div className="license-state-icon info-state-icon">↻</div>
            <Status tone="info">Service unavailable</Status>
            <h1>Your entitlement has not been marked invalid.</h1>
            <p>
              CitadelOps could not reach the entitlement service. Your last
              validated access remains in offline grace while we retry.
            </p>
            <div className="license-facts">
              <div>
                <span>Last validated</span>
                <strong>Jul 14 · 9:42 PM EDT</strong>
              </div>
              <div>
                <span>Offline grace</span>
                <strong>2d 11h remaining</strong>
              </div>
              <div>
                <span>Automatic retry</span>
                <strong>In 2 minutes</strong>
              </div>
            </div>
            <div className="service-chain">
              <div className="chain-ok">
                <span>✓</span>
                <strong>Desktop service</strong>
                <small>Healthy</small>
              </div>
              <i />
              <div className="chain-down">
                <span>!</span>
                <strong>Entitlement service</strong>
                <small>Unavailable</small>
              </div>
              <i />
              <div className="chain-ok">
                <span>✓</span>
                <strong>Game connection</strong>
                <small>Healthy</small>
              </div>
            </div>
            <div className="access-policy compact-policy">
              <h2>Safe offline behavior</h2>
              <ul>
                <li>
                  <span>✓</span>Existing state, activity, and diagnostics remain
                  available
                </li>
                <li>
                  <span>✓</span>Current safe operations can reach their next
                  boundary
                </li>
                <li>
                  <span>Ⅱ</span>New high-value writes wait for validation
                </li>
              </ul>
            </div>
            <div className="license-actions">
              <button className="primary-button large" type="button">
                Retry now
              </button>
              <button className="secondary-button large" type="button">
                View service status
              </button>
              <button className="text-button" type="button">
                Work offline
              </button>
            </div>
          </section>
        )}
        <aside className="license-principles">
          <span className="eyebrow">Design rule</span>
          <h2>Cause, consequence, and remedy stay distinct.</h2>
          <ul>
            <li>
              <span>1</span>
              <div>
                <strong>Name the real condition</strong>
                <small>Billing failure is not invalid entitlement.</small>
              </div>
            </li>
            <li>
              <span>2</span>
              <div>
                <strong>Preserve safe access</strong>
                <small>Never blank the app during service failure.</small>
              </div>
            </li>
            <li>
              <span>3</span>
              <div>
                <strong>Show exact time and policy</strong>
                <small>Grace and access changes are never vague.</small>
              </div>
            </li>
          </ul>
        </aside>
      </main>
    </div>
  );
}

export function ConceptShowcase() {
  const [screen, setScreen] = useState<ScreenId>("public");
  const [theme, setTheme] = useState<"dark" | "light">("dark");

  const activeLabel =
    screenOptions.find((option) => option.id === screen)?.label ?? "Concept";

  return (
    <main className="concept-root" data-theme={theme}>
      <div className="concept-bar">
        <div className="concept-note">
          <span className="concept-dot" />
          <strong>CitadelOps redesign concept</strong>
          <span>Working name and icon pending clearance</span>
        </div>
        <div
          className="screen-picker"
          role="toolbar"
          aria-label="Concept screens"
        >
          {screenOptions.map((option) => (
            <button
              className={screen === option.id ? "active" : ""}
              type="button"
              aria-pressed={screen === option.id}
              onClick={() => setScreen(option.id)}
              key={option.id}
            >
              <small>{option.group}</small>
              {option.label}
            </button>
          ))}
        </div>
        <div className="theme-picker" aria-label="Theme">
          <button
            className={theme === "dark" ? "active" : ""}
            type="button"
            aria-pressed={theme === "dark"}
            onClick={() => setTheme("dark")}
            aria-label="Use dark theme"
          >
            ◐
          </button>
          <button
            className={theme === "light" ? "active" : ""}
            type="button"
            aria-pressed={theme === "light"}
            onClick={() => setTheme("light")}
            aria-label="Use light theme"
          >
            ○
          </button>
        </div>
      </div>
      <div
        className="concept-viewport"
        aria-label={`${activeLabel} concept screen`}
      >
        {screen === "public" && <PublicHome navigate={setScreen} />}
        {screen === "account" && <AccountScreen navigate={setScreen} />}
        {screen === "overview" && <OverviewScreen navigate={setScreen} />}
        {screen === "automations" && <AutomationsScreen navigate={setScreen} />}
        {screen === "editor" && <EditorScreen navigate={setScreen} />}
        {screen === "execution" && <ExecutionScreen navigate={setScreen} />}
        {screen === "activity" && <ActivityScreen />}
        {screen === "authorize" && <AuthorizationScreen navigate={setScreen} />}
        {screen === "license" && <LicenseScreen />}
      </div>
    </main>
  );
}
