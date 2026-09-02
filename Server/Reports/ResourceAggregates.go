package Reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/State"
)

const (
	ResourceAggregateSourceSeconds int64 = 60
	resourceAggregateSchemaVersion       = "resource-aggregates.minute.v2"
	resourceAggregateSchemaKey           = "resource_aggregate_schema"
	resourceAggregateWatermarkKey        = "resource_aggregate_raw_watermark"
	resourceAggregateQueryLimit          = 100_000
	resourceAggregateOutboxLimit         = 1_000
)

// ResourceViewKey is the durable stat-view identity assigned while a battle
// report is parsed. It intentionally does not expose the automation's internal
// feature name to storage consumers.
type ResourceViewKey string

const (
	ResourceViewTower      ResourceViewKey = "tower"
	ResourceViewStorm      ResourceViewKey = "storm"
	ResourceViewInvasion   ResourceViewKey = "invasion"
	ResourceViewNomad      ResourceViewKey = "nomad"
	ResourceViewAdvisor    ResourceViewKey = "advisor"
	ResourceViewKhan       ResourceViewKey = "khan"
	ResourceViewBerimond   ResourceViewKey = "berimond"
	ResourceViewRift       ResourceViewKey = "rift"
	ResourceViewRiftReplay ResourceViewKey = "rift-replay"
)

// ResourceAggregate is one additive account/view/time bucket. Incoming battle
// reports always produce 60-second source rows. Hosted retention may later
// merge those rows into hour/day buckets without losing any totals.
type ResourceAggregate struct {
	ViewKey         ResourceViewKey  `json:"viewKey"`
	BucketStart     time.Time        `json:"bucketStart"`
	BucketSeconds   int64            `json:"bucketSeconds"`
	ReportCount     int64            `json:"reportCount"`
	Victories       int64            `json:"victories"`
	Defeats         int64            `json:"defeats"`
	TroopsSent      int64            `json:"troopsSent"`
	TroopLosses     int64            `json:"troopLosses"`
	ToolsUsed       int64            `json:"toolsUsed"`
	GallantryPoints int64            `json:"gallantryPoints"`
	LootTotal       int64            `json:"lootTotal"`
	Resources       map[string]int64 `json:"resources"`
	FirstOccurredAt time.Time        `json:"firstOccurredAt"`
	LastOccurredAt  time.Time        `json:"lastOccurredAt"`
	Revision        int64            `json:"revision"`
	Deleted         bool             `json:"deleted,omitempty"`
}

type ResourceAggregateQuery struct {
	AccountUID int64
	WorldID    string
	PlayerID   int64
	ViewKey    ResourceViewKey
	Since      time.Time
	Before     time.Time
	Limit      int
}

// PendingResourceAggregate is an outbox item. Revision is part of the public
// aggregate contract and also fences acknowledgement against a newer local
// rewrite of the same minute.
type PendingResourceAggregate struct {
	IdentityKey string
	Aggregate   ResourceAggregate
	// Rollups are absolute hour/day reconciliations derived from the current
	// minute table. They let hosted retention replace an already-compacted
	// bucket after a late report correction without retaining every cloud
	// source minute indefinitely. Acknowledgement remains fenced by Aggregate.
	Rollups []ResourceAggregate
}

type ResourceAggregateOutboxQuery struct {
	AccountUID int64
	WorldID    string
	PlayerID   int64
	Limit      int
}

// ResourceAggregateMigrationStatus is the durable receipt for the historical
// report-to-minute migration. SourceReports and SourceBuckets describe every
// exact, feature-attributed report still recoverable on this runtime; pending
// buckets have not yet been acknowledged by the hosted backend.
type ResourceAggregateMigrationStatus struct {
	SourceReports       int64
	SourceBuckets       int64
	PendingBuckets      int64
	OldestOccurredAt    time.Time
	NewestOccurredAt    time.Time
	OldestPendingBucket time.Time
	NewestPendingBucket time.Time
}

type resourceAggregateTuple struct {
	identityKey string
	viewKey     ResourceViewKey
	bucketStart int64
}

func resourceAggregateStorageKey(value ResourceAggregate) string {
	return string(value.ViewKey) + "\x00" + strconv.FormatInt(value.BucketStart.UTC().Unix(), 10) +
		"\x00" + strconv.FormatInt(value.BucketSeconds, 10)
}

type rawResourceContribution struct {
	accountUID      int64
	worldID         string
	playerID        int64
	reportKey       string
	featureID       string
	occurredAt      string
	result          string
	role            string
	troopsSent      int64
	troopLosses     int64
	toolsUsed       int64
	gallantryPoints int64
	lootTotal       int64
	lootPayload     []byte
	updatedAt       string
}

func ResourceViewKeyForFeature(featureID string) (ResourceViewKey, bool) {
	switch State.AttackFeatureID(strings.TrimSpace(featureID)) {
	case State.AttackFeatureAutoTowers:
		return ResourceViewTower, true
	case State.AttackFeatureAutoStorm:
		return ResourceViewStorm, true
	case State.AttackFeatureAutoInvasion:
		return ResourceViewInvasion, true
	case State.AttackFeatureAutoNomad:
		return ResourceViewNomad, true
	case State.AttackFeatureAutoAdvisor:
		return ResourceViewAdvisor, true
	case State.AttackFeatureAutoKhan:
		return ResourceViewKhan, true
	case State.AttackFeatureAutoBeriWorld:
		return ResourceViewBerimond, true
	case State.AttackFeatureRiftMaiden:
		return ResourceViewRift, true
	case State.AttackFeatureRiftReplay:
		return ResourceViewRiftReplay, true
	default:
		return "", false
	}
}

func ValidResourceViewKey(value ResourceViewKey) bool {
	switch value {
	case ResourceViewTower, ResourceViewStorm, ResourceViewInvasion,
		ResourceViewNomad, ResourceViewAdvisor, ResourceViewKhan,
		ResourceViewBerimond, ResourceViewRift, ResourceViewRiftReplay:
		return true
	default:
		return false
	}
}

func ResourceViewKeys() []ResourceViewKey {
	return []ResourceViewKey{
		ResourceViewTower, ResourceViewStorm, ResourceViewInvasion,
		ResourceViewNomad, ResourceViewAdvisor, ResourceViewKhan,
		ResourceViewBerimond, ResourceViewRift, ResourceViewRiftReplay,
	}
}

func FeatureForResourceViewKey(viewKey ResourceViewKey) (State.AttackFeatureID, bool) {
	switch viewKey {
	case ResourceViewTower:
		return State.AttackFeatureAutoTowers, true
	case ResourceViewStorm:
		return State.AttackFeatureAutoStorm, true
	case ResourceViewInvasion:
		return State.AttackFeatureAutoInvasion, true
	case ResourceViewNomad:
		return State.AttackFeatureAutoNomad, true
	case ResourceViewAdvisor:
		return State.AttackFeatureAutoAdvisor, true
	case ResourceViewKhan:
		return State.AttackFeatureAutoKhan, true
	case ResourceViewBerimond:
		return State.AttackFeatureAutoBeriWorld, true
	case ResourceViewRift:
		return State.AttackFeatureRiftMaiden, true
	case ResourceViewRiftReplay:
		return State.AttackFeatureRiftReplay, true
	default:
		return "", false
	}
}

func createResourceAggregateTables(ctx context.Context, execer sqlReportExecer) error {
	_, err := execer.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS feature_resource_aggregates (
			identity_key TEXT NOT NULL,
			view_key TEXT NOT NULL,
			bucket_start INTEGER NOT NULL,
			bucket_seconds INTEGER NOT NULL,
			report_count INTEGER NOT NULL,
			victories INTEGER NOT NULL,
			defeats INTEGER NOT NULL,
			troops_sent INTEGER NOT NULL,
			troop_losses INTEGER NOT NULL,
			tools_used INTEGER NOT NULL,
			gallantry_points INTEGER NOT NULL,
			loot_total INTEGER NOT NULL,
			resources_json BLOB NOT NULL,
			first_occurred_at TEXT NOT NULL,
			last_occurred_at TEXT NOT NULL,
			revision INTEGER NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (identity_key, view_key, bucket_start)
		);
		CREATE INDEX IF NOT EXISTS feature_resource_aggregates_history
			ON feature_resource_aggregates(identity_key, view_key, bucket_start);
		CREATE TABLE IF NOT EXISTS feature_resource_aggregate_outbox (
			identity_key TEXT NOT NULL,
			view_key TEXT NOT NULL,
			bucket_start INTEGER NOT NULL,
			revision INTEGER NOT NULL,
			payload_json BLOB NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (identity_key, view_key, bucket_start)
		);
		CREATE INDEX IF NOT EXISTS feature_resource_aggregate_outbox_updated
			ON feature_resource_aggregate_outbox(updated_at, identity_key, view_key, bucket_start);
	`)
	if err != nil {
		return fmt.Errorf("initialize feature resource aggregates: %w", err)
	}
	return nil
}

func ensureResourceAggregateColumns(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(feature_resource_aggregates)`)
	if err != nil {
		return fmt.Errorf("inspect feature resource aggregate columns: %w", err)
	}
	columns := map[string]bool{}
	for rows.Next() {
		var sequence, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("scan feature resource aggregate column: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close feature resource aggregate columns: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect feature resource aggregate columns: %w", err)
	}
	for _, column := range []string{"first_occurred_at", "last_occurred_at"} {
		if columns[column] {
			continue
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE feature_resource_aggregates ADD COLUMN `+column+` TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add feature resource aggregate %s: %w", column, err)
		}
	}
	return nil
}

func (store *SQLiteStore) ensureResourceAggregatesCurrent(ctx context.Context) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	var version, recordedWatermark string
	_ = store.db.QueryRowContext(ctx, `SELECT value FROM battle_report_storage_metadata WHERE key = ?`, resourceAggregateSchemaKey).Scan(&version)
	_ = store.db.QueryRowContext(ctx, `SELECT value FROM battle_report_storage_metadata WHERE key = ?`, resourceAggregateWatermarkKey).Scan(&recordedWatermark)
	var rawWatermark string
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(updated_at), '') FROM battle_report_analytics`).Scan(&rawWatermark); err != nil {
		return fmt.Errorf("read feature resource aggregate watermark: %w", err)
	}
	if version == resourceAggregateSchemaVersion && recordedWatermark == rawWatermark {
		return nil
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin feature resource aggregate rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := rebuildResourceAggregates(ctx, tx); err != nil {
		return err
	}
	if err := recordResourceAggregateWatermark(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit feature resource aggregate rebuild: %w", err)
	}
	return nil
}

func rebuildResourceAggregates(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT account_uid, world_id, player_id, report_key, automation_feature,
			occurred_at, result, role, troops_sent, own_troop_losses, tools_used,
			gallantry_points, loot_total, loot_json, updated_at
		FROM battle_report_analytics
		ORDER BY updated_at ASC, account_key ASC, report_key ASC
	`)
	if err != nil {
		return fmt.Errorf("read reports for feature resource aggregate rebuild: %w", err)
	}
	latest := map[string]rawResourceContribution{}
	for rows.Next() {
		contribution, scanErr := scanRawResourceContribution(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		identityKey := resourceAggregateIdentity(contribution.accountUID, contribution.worldID, contribution.playerID)
		if identityKey == "" {
			continue
		}
		latest[identityKey+"\x00"+contribution.reportKey] = contribution
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close feature resource aggregate rebuild rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read reports for feature resource aggregate rebuild: %w", err)
	}

	aggregates := map[resourceAggregateTuple]*ResourceAggregate{}
	for key, contribution := range latest {
		identityKey := key[:strings.IndexByte(key, 0)]
		tuple, valid := resourceTupleForContribution(identityKey, contribution)
		if !valid {
			continue
		}
		aggregate := aggregates[tuple]
		if aggregate == nil {
			created := newResourceAggregate(tuple)
			aggregate = &created
			aggregates[tuple] = aggregate
		}
		addRawContribution(aggregate, contribution)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM feature_resource_aggregates`); err != nil {
		return fmt.Errorf("reset feature resource aggregates: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM feature_resource_aggregate_outbox`); err != nil {
		return fmt.Errorf("reset feature resource aggregate outbox: %w", err)
	}
	tuples := make([]resourceAggregateTuple, 0, len(aggregates))
	for tuple := range aggregates {
		tuples = append(tuples, tuple)
	}
	sort.Slice(tuples, func(left, right int) bool {
		if tuples[left].identityKey != tuples[right].identityKey {
			return tuples[left].identityKey < tuples[right].identityKey
		}
		if tuples[left].viewKey != tuples[right].viewKey {
			return tuples[left].viewKey < tuples[right].viewKey
		}
		return tuples[left].bucketStart < tuples[right].bucketStart
	})
	for _, tuple := range tuples {
		if err := writeResourceAggregate(ctx, tx, tuple, *aggregates[tuple]); err != nil {
			return err
		}
	}
	return nil
}

func recordResourceAggregateWatermark(ctx context.Context, tx *sql.Tx) error {
	var watermark string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(updated_at), '') FROM battle_report_analytics`).Scan(&watermark); err != nil {
		return fmt.Errorf("read feature resource aggregate watermark: %w", err)
	}
	for _, value := range []struct{ key, content string }{
		{resourceAggregateSchemaKey, resourceAggregateSchemaVersion},
		{resourceAggregateWatermarkKey, watermark},
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO battle_report_storage_metadata (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, value.key, value.content); err != nil {
			return fmt.Errorf("record feature resource aggregate metadata: %w", err)
		}
	}
	return nil
}

func resourceAggregateIdentity(uid int64, worldID string, playerID int64) string {
	worldID = State.CanonicalWorldID(worldID)
	if worldID != "" && playerID > 0 {
		return "world:" + worldID + ":player:" + strconv.FormatInt(playerID, 10)
	}
	if uid > 0 {
		return "uid:" + strconv.FormatInt(uid, 10)
	}
	return ""
}

func resourceAggregateIdentityCandidates(uid int64, worldID string, playerID int64) []string {
	result := make([]string, 0, 2)
	if stable := resourceAggregateIdentity(0, worldID, playerID); stable != "" {
		result = append(result, stable)
	}
	if uid > 0 {
		uidKey := resourceAggregateIdentity(uid, "", 0)
		if len(result) == 0 || result[0] != uidKey {
			result = append(result, uidKey)
		}
	}
	return result
}

func resourceTupleForReport(report BattleReport) (resourceAggregateTuple, bool) {
	identityKey := resourceAggregateIdentity(report.AccountUID, report.WorldID, report.PlayerID)
	if identityKey == "" {
		return resourceAggregateTuple{}, false
	}
	contribution := rawResourceContribution{
		accountUID: report.AccountUID, worldID: report.WorldID, playerID: report.PlayerID,
		reportKey: report.ID, featureID: report.AutomationFeature, occurredAt: report.OccurredAt,
		result: report.Result, role: report.Role,
	}
	return resourceTupleForContribution(identityKey, contribution)
}

func resourceTupleForContribution(identityKey string, contribution rawResourceContribution) (resourceAggregateTuple, bool) {
	if !strings.EqualFold(strings.TrimSpace(contribution.role), "attacker") {
		return resourceAggregateTuple{}, false
	}
	viewKey, valid := ResourceViewKeyForFeature(contribution.featureID)
	if !valid {
		return resourceAggregateTuple{}, false
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(contribution.occurredAt))
	if err != nil {
		return resourceAggregateTuple{}, false
	}
	return resourceAggregateTuple{
		identityKey: identityKey,
		viewKey:     viewKey,
		bucketStart: occurredAt.UTC().Truncate(time.Minute).Unix(),
	}, true
}

func recomputeResourceAggregate(ctx context.Context, tx *sql.Tx, tuple resourceAggregateTuple) error {
	start := time.Unix(tuple.bucketStart, 0).UTC()
	end := start.Add(time.Minute)
	rows, err := tx.QueryContext(ctx, `
		SELECT account_uid, world_id, player_id, report_key, automation_feature,
			occurred_at, result, role, troops_sent, own_troop_losses, tools_used,
			gallantry_points, loot_total, loot_json, updated_at
		FROM battle_report_analytics
		WHERE occurred_at >= ? AND occurred_at < ? AND lower(role) = 'attacker'
		ORDER BY updated_at ASC, account_key ASC, report_key ASC
	`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("read reports for feature resource minute: %w", err)
	}
	latest := map[string]rawResourceContribution{}
	for rows.Next() {
		contribution, scanErr := scanRawResourceContribution(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		if resourceAggregateIdentity(contribution.accountUID, contribution.worldID, contribution.playerID) != tuple.identityKey {
			continue
		}
		candidate, valid := resourceTupleForContribution(tuple.identityKey, contribution)
		if !valid || candidate != tuple {
			continue
		}
		latest[contribution.reportKey] = contribution
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close feature resource minute rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read reports for feature resource minute: %w", err)
	}
	if len(latest) == 0 {
		return deleteResourceAggregate(ctx, tx, tuple)
	}
	aggregate := newResourceAggregate(tuple)
	for _, contribution := range latest {
		addRawContribution(&aggregate, contribution)
	}
	return writeResourceAggregate(ctx, tx, tuple, aggregate)
}

func newResourceAggregate(tuple resourceAggregateTuple) ResourceAggregate {
	return ResourceAggregate{
		ViewKey: tuple.viewKey, BucketStart: time.Unix(tuple.bucketStart, 0).UTC(),
		BucketSeconds: ResourceAggregateSourceSeconds, Resources: map[string]int64{},
	}
}

func addRawContribution(aggregate *ResourceAggregate, contribution rawResourceContribution) {
	if aggregate == nil {
		return
	}
	aggregate.ReportCount++
	switch strings.ToLower(strings.TrimSpace(contribution.result)) {
	case "victory", "won", "win":
		aggregate.Victories++
	case "defeat", "lost", "loss":
		aggregate.Defeats++
	}
	aggregate.TroopsSent += max(int64(0), contribution.troopsSent)
	aggregate.TroopLosses += max(int64(0), contribution.troopLosses)
	aggregate.ToolsUsed += max(int64(0), contribution.toolsUsed)
	aggregate.GallantryPoints += max(int64(0), contribution.gallantryPoints)
	aggregate.LootTotal += max(int64(0), contribution.lootTotal)
	if occurredAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(contribution.occurredAt)); err == nil {
		occurredAt = occurredAt.UTC()
		if aggregate.FirstOccurredAt.IsZero() || occurredAt.Before(aggregate.FirstOccurredAt) {
			aggregate.FirstOccurredAt = occurredAt
		}
		if aggregate.LastOccurredAt.IsZero() || occurredAt.After(aggregate.LastOccurredAt) {
			aggregate.LastOccurredAt = occurredAt
		}
	}
	var resources map[string]int64
	if len(contribution.lootPayload) > 0 && json.Unmarshal(contribution.lootPayload, &resources) == nil {
		for key, amount := range resources {
			key = strings.TrimSpace(key)
			if key != "" && amount > 0 {
				aggregate.Resources[key] += amount
			}
		}
	}
}

type resourceContributionScanner interface {
	Scan(...any) error
}

func scanRawResourceContribution(scanner resourceContributionScanner) (rawResourceContribution, error) {
	var contribution rawResourceContribution
	if err := scanner.Scan(
		&contribution.accountUID, &contribution.worldID, &contribution.playerID,
		&contribution.reportKey, &contribution.featureID, &contribution.occurredAt,
		&contribution.result, &contribution.role, &contribution.troopsSent,
		&contribution.troopLosses, &contribution.toolsUsed, &contribution.gallantryPoints,
		&contribution.lootTotal, &contribution.lootPayload, &contribution.updatedAt,
	); err != nil {
		return rawResourceContribution{}, fmt.Errorf("scan report for feature resource aggregate: %w", err)
	}
	return contribution, nil
}

func writeResourceAggregate(ctx context.Context, tx *sql.Tx, tuple resourceAggregateTuple, aggregate ResourceAggregate) error {
	resourcesPayload, err := json.Marshal(aggregate.Resources)
	if err != nil {
		return fmt.Errorf("encode feature resource aggregate: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = tx.QueryRowContext(ctx, `
		INSERT INTO feature_resource_aggregates (
			identity_key, view_key, bucket_start, bucket_seconds,
			report_count, victories, defeats, troops_sent, troop_losses,
			tools_used, gallantry_points, loot_total, resources_json,
			first_occurred_at, last_occurred_at,
			revision, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(identity_key, view_key, bucket_start) DO UPDATE SET
			bucket_seconds = excluded.bucket_seconds,
			report_count = excluded.report_count,
			victories = excluded.victories,
			defeats = excluded.defeats,
			troops_sent = excluded.troops_sent,
			troop_losses = excluded.troop_losses,
			tools_used = excluded.tools_used,
			gallantry_points = excluded.gallantry_points,
			loot_total = excluded.loot_total,
			resources_json = excluded.resources_json,
			first_occurred_at = excluded.first_occurred_at,
			last_occurred_at = excluded.last_occurred_at,
			revision = feature_resource_aggregates.revision + 1,
			updated_at = excluded.updated_at
		RETURNING revision
	`, tuple.identityKey, tuple.viewKey, tuple.bucketStart, ResourceAggregateSourceSeconds,
		aggregate.ReportCount, aggregate.Victories, aggregate.Defeats, aggregate.TroopsSent,
		aggregate.TroopLosses, aggregate.ToolsUsed, aggregate.GallantryPoints,
		aggregate.LootTotal, resourcesPayload,
		aggregate.FirstOccurredAt.UTC().Format(time.RFC3339Nano),
		aggregate.LastOccurredAt.UTC().Format(time.RFC3339Nano), now).Scan(&aggregate.Revision)
	if err != nil {
		return fmt.Errorf("write feature resource aggregate: %w", err)
	}
	aggregate.BucketStart = time.Unix(tuple.bucketStart, 0).UTC()
	aggregate.BucketSeconds = ResourceAggregateSourceSeconds
	aggregate.ViewKey = tuple.viewKey
	return queueResourceAggregate(ctx, tx, tuple.identityKey, aggregate, now)
}

func deleteResourceAggregate(ctx context.Context, tx *sql.Tx, tuple resourceAggregateTuple) error {
	var revision int64
	err := tx.QueryRowContext(ctx, `
		DELETE FROM feature_resource_aggregates
		WHERE identity_key = ? AND view_key = ? AND bucket_start = ?
		RETURNING revision + 1
	`, tuple.identityKey, tuple.viewKey, tuple.bucketStart).Scan(&revision)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete feature resource aggregate: %w", err)
	}
	aggregate := newResourceAggregate(tuple)
	aggregate.Revision = revision
	aggregate.Deleted = true
	return queueResourceAggregate(ctx, tx, tuple.identityKey, aggregate, time.Now().UTC().Format(time.RFC3339Nano))
}

func queueResourceAggregate(ctx context.Context, tx *sql.Tx, identityKey string, aggregate ResourceAggregate, updatedAt string) error {
	payload, err := json.Marshal(aggregate)
	if err != nil {
		return fmt.Errorf("encode feature resource aggregate outbox: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO feature_resource_aggregate_outbox (
			identity_key, view_key, bucket_start, revision, payload_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(identity_key, view_key, bucket_start) DO UPDATE SET
			revision = excluded.revision,
			payload_json = excluded.payload_json,
			updated_at = excluded.updated_at
	`, identityKey, aggregate.ViewKey, aggregate.BucketStart.Unix(), aggregate.Revision, payload, updatedAt)
	if err != nil {
		return fmt.Errorf("queue feature resource aggregate: %w", err)
	}
	return nil
}

func affectedResourceTuples(ctx context.Context, tx *sql.Tx, report BattleReport) (map[resourceAggregateTuple]struct{}, error) {
	result := map[resourceAggregateTuple]struct{}{}
	accountKey := reportAccountKey(report.AccountUID, report.WorldID, report.PlayerID)
	rows, err := tx.QueryContext(ctx, `
		SELECT account_uid, world_id, player_id, report_key, automation_feature,
			occurred_at, result, role, troops_sent, own_troop_losses, tools_used,
			gallantry_points, loot_total, loot_json, updated_at
		FROM battle_report_analytics
		WHERE report_key = ? AND (
			account_key = ? OR (world_id = ? AND player_id = ?)
		)
	`, report.ID, accountKey, strings.TrimSpace(report.WorldID), report.PlayerID)
	if err != nil {
		return nil, fmt.Errorf("read replaced feature resource report: %w", err)
	}
	for rows.Next() {
		contribution, scanErr := scanRawResourceContribution(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		identityKey := resourceAggregateIdentity(contribution.accountUID, contribution.worldID, contribution.playerID)
		if tuple, valid := resourceTupleForContribution(identityKey, contribution); valid {
			result[tuple] = struct{}{}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close replaced feature resource reports: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read replaced feature resource reports: %w", err)
	}
	if tuple, valid := resourceTupleForReport(report); valid {
		result[tuple] = struct{}{}
	}
	return result, nil
}

func (store *SQLiteStore) ResourceAggregates(ctx context.Context, query ResourceAggregateQuery) ([]ResourceAggregate, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	identities := resourceAggregateIdentityCandidates(query.AccountUID, query.WorldID, query.PlayerID)
	if len(identities) == 0 || !ValidResourceViewKey(query.ViewKey) {
		return []ResourceAggregate{}, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > resourceAggregateQueryLimit {
		limit = resourceAggregateQueryLimit
	}
	identityPlaceholders := strings.TrimRight(strings.Repeat("?,", len(identities)), ",")
	clauses := []string{"identity_key IN (" + identityPlaceholders + ")", "view_key = ?"}
	arguments := make([]any, 0, len(identities)+5)
	for _, identity := range identities {
		arguments = append(arguments, identity)
	}
	arguments = append(arguments, query.ViewKey)
	if !query.Since.IsZero() {
		clauses = append(clauses, "bucket_start >= ?")
		arguments = append(arguments, query.Since.UTC().Truncate(time.Minute).Unix())
	}
	if !query.Before.IsZero() {
		clauses = append(clauses, "bucket_start < ?")
		arguments = append(arguments, query.Before.UTC().Unix())
	}
	rawLimit := limit * len(identities)
	arguments = append(arguments, rawLimit)
	rows, err := store.db.QueryContext(ctx, `
		SELECT view_key, bucket_start, bucket_seconds, report_count,
			victories, defeats, troops_sent, troop_losses, tools_used,
			gallantry_points, loot_total, resources_json,
			first_occurred_at, last_occurred_at, revision
		FROM feature_resource_aggregates
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY bucket_start DESC
		LIMIT ?
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list feature resource aggregates: %w", err)
	}
	defer rows.Close()
	merged := make(map[string]*ResourceAggregate, min(rawLimit, 2048))
	for rows.Next() {
		var aggregate ResourceAggregate
		var bucketStart int64
		var resourcesPayload []byte
		var firstOccurredAt, lastOccurredAt string
		if err := rows.Scan(
			&aggregate.ViewKey, &bucketStart, &aggregate.BucketSeconds, &aggregate.ReportCount,
			&aggregate.Victories, &aggregate.Defeats, &aggregate.TroopsSent, &aggregate.TroopLosses,
			&aggregate.ToolsUsed, &aggregate.GallantryPoints, &aggregate.LootTotal,
			&resourcesPayload, &firstOccurredAt, &lastOccurredAt, &aggregate.Revision,
		); err != nil {
			return nil, fmt.Errorf("scan feature resource aggregate: %w", err)
		}
		aggregate.BucketStart = time.Unix(bucketStart, 0).UTC()
		aggregate.FirstOccurredAt, _ = time.Parse(time.RFC3339Nano, firstOccurredAt)
		aggregate.LastOccurredAt, _ = time.Parse(time.RFC3339Nano, lastOccurredAt)
		aggregate.Resources = map[string]int64{}
		if len(resourcesPayload) > 0 {
			if err := json.Unmarshal(resourcesPayload, &aggregate.Resources); err != nil {
				return nil, fmt.Errorf("decode feature resource aggregate resources: %w", err)
			}
		}
		key := resourceAggregateStorageKey(aggregate)
		if existing := merged[key]; existing != nil {
			addResourceAggregate(existing, aggregate)
			continue
		}
		copy := aggregate
		merged[key] = &copy
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list feature resource aggregates: %w", err)
	}
	result := make([]ResourceAggregate, 0, len(merged))
	for _, aggregate := range merged {
		result = append(result, *aggregate)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].BucketStart.Equal(result[right].BucketStart) {
			return result[left].BucketSeconds < result[right].BucketSeconds
		}
		return result[left].BucketStart.After(result[right].BucketStart)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (store *SQLiteStore) PendingResourceAggregates(ctx context.Context, query ResourceAggregateOutboxQuery) ([]PendingResourceAggregate, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	identities := resourceAggregateIdentityCandidates(query.AccountUID, query.WorldID, query.PlayerID)
	if len(identities) == 0 {
		return []PendingResourceAggregate{}, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > resourceAggregateOutboxLimit {
		limit = resourceAggregateOutboxLimit
	}
	identityPlaceholders := strings.TrimRight(strings.Repeat("?,", len(identities)), ",")
	arguments := make([]any, 0, len(identities)+1)
	for _, identity := range identities {
		arguments = append(arguments, identity)
	}
	arguments = append(arguments, limit)
	rows, err := store.db.QueryContext(ctx, `
		SELECT identity_key, payload_json
		FROM feature_resource_aggregate_outbox
		WHERE identity_key IN (`+identityPlaceholders+`)
		ORDER BY updated_at ASC, identity_key ASC, view_key ASC, bucket_start ASC
		LIMIT ?
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list feature resource aggregate outbox: %w", err)
	}
	result := make([]PendingResourceAggregate, 0, limit)
	for rows.Next() {
		var item PendingResourceAggregate
		var payload []byte
		if err := rows.Scan(&item.IdentityKey, &payload); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan feature resource aggregate outbox: %w", err)
		}
		if err := json.Unmarshal(payload, &item.Aggregate); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode feature resource aggregate outbox: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("list feature resource aggregate outbox: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close feature resource aggregate outbox: %w", err)
	}
	for index := range result {
		merged, err := store.resourceAggregateRollup(
			ctx, identities, result[index].Aggregate.ViewKey,
			result[index].Aggregate.BucketStart, ResourceAggregateSourceSeconds,
			result[index].Aggregate.Revision,
		)
		if err != nil {
			return nil, err
		}
		result[index].Aggregate = merged
		rollups, err := store.resourceAggregateRollups(ctx, identities, result[index])
		if err != nil {
			return nil, err
		}
		result[index].Rollups = rollups
	}
	return result, nil
}

// ResourceAggregateMigrationStatus reports exact local coverage and delivery
// progress for one account binding without reading or decoding raw reports.
func (store *SQLiteStore) ResourceAggregateMigrationStatus(
	ctx context.Context,
	query ResourceAggregateOutboxQuery,
) (ResourceAggregateMigrationStatus, error) {
	if store == nil || store.db == nil {
		return ResourceAggregateMigrationStatus{}, fmt.Errorf("report analytics database is unavailable")
	}
	identities := resourceAggregateIdentityCandidates(query.AccountUID, query.WorldID, query.PlayerID)
	if len(identities) == 0 {
		return ResourceAggregateMigrationStatus{}, nil
	}
	identityPlaceholders := strings.TrimRight(strings.Repeat("?,", len(identities)), ",")
	arguments := make([]any, 0, len(identities))
	for _, identity := range identities {
		arguments = append(arguments, identity)
	}
	var status ResourceAggregateMigrationStatus
	var oldestOccurredAt, newestOccurredAt string
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(report_count), 0),
			COALESCE(MIN(first_occurred_at), ''), COALESCE(MAX(last_occurred_at), '')
		FROM feature_resource_aggregates
		WHERE identity_key IN (`+identityPlaceholders+`)
	`, arguments...).Scan(
		&status.SourceBuckets, &status.SourceReports, &oldestOccurredAt, &newestOccurredAt,
	); err != nil {
		return ResourceAggregateMigrationStatus{}, fmt.Errorf("read feature resource aggregate migration coverage: %w", err)
	}
	status.OldestOccurredAt, _ = time.Parse(time.RFC3339Nano, oldestOccurredAt)
	status.NewestOccurredAt, _ = time.Parse(time.RFC3339Nano, newestOccurredAt)
	var oldestPendingBucket, newestPendingBucket int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MIN(bucket_start), 0), COALESCE(MAX(bucket_start), 0)
		FROM feature_resource_aggregate_outbox
		WHERE identity_key IN (`+identityPlaceholders+`)
	`, arguments...).Scan(
		&status.PendingBuckets, &oldestPendingBucket, &newestPendingBucket,
	); err != nil {
		return ResourceAggregateMigrationStatus{}, fmt.Errorf("read feature resource aggregate migration outbox: %w", err)
	}
	if oldestPendingBucket > 0 {
		status.OldestPendingBucket = time.Unix(oldestPendingBucket, 0).UTC()
	}
	if newestPendingBucket > 0 {
		status.NewestPendingBucket = time.Unix(newestPendingBucket, 0).UTC()
	}
	return status, nil
}

func (store *SQLiteStore) resourceAggregateRollups(
	ctx context.Context,
	identities []string,
	pending PendingResourceAggregate,
) ([]ResourceAggregate, error) {
	rollups := make([]ResourceAggregate, 0, 2)
	for _, bucketSeconds := range []int64{60 * 60, 24 * 60 * 60} {
		start := pending.Aggregate.BucketStart.UTC()
		if bucketSeconds == 60*60 {
			start = start.Truncate(time.Hour)
		} else {
			start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		}
		rollup, err := store.resourceAggregateRollup(
			ctx, identities, pending.Aggregate.ViewKey, start, bucketSeconds,
			pending.Aggregate.Revision,
		)
		if err != nil {
			return nil, err
		}
		rollups = append(rollups, rollup)
	}
	return rollups, nil
}

// resourceAggregateRollup reconciles one absolute bucket across every known
// local identity for the account. Older profiles may contain UID-keyed reports
// while newer reports use the stable world/player key; publishing either
// identity independently would make one correct partial bucket overwrite the
// other in the account-scoped hosted table.
func (store *SQLiteStore) resourceAggregateRollup(
	ctx context.Context,
	identities []string,
	viewKey ResourceViewKey,
	start time.Time,
	bucketSeconds int64,
	revision int64,
) (ResourceAggregate, error) {
	start = start.UTC()
	rollup := ResourceAggregate{
		ViewKey: viewKey, BucketStart: start, BucketSeconds: bucketSeconds,
		Resources: map[string]int64{}, Revision: revision,
	}
	if len(identities) == 0 {
		rollup.Deleted = true
		return rollup, nil
	}
	identityPlaceholders := strings.TrimRight(strings.Repeat("?,", len(identities)), ",")
	arguments := make([]any, 0, len(identities)+3)
	for _, identity := range identities {
		arguments = append(arguments, identity)
	}
	arguments = append(arguments, viewKey, start.Unix(), start.Add(time.Duration(bucketSeconds)*time.Second).Unix())
	rows, err := store.db.QueryContext(ctx, `
		SELECT view_key, bucket_start, bucket_seconds, report_count,
			victories, defeats, troops_sent, troop_losses, tools_used,
			gallantry_points, loot_total, resources_json,
			first_occurred_at, last_occurred_at, revision
		FROM feature_resource_aggregates
		WHERE identity_key IN (`+identityPlaceholders+`)
			AND view_key = ? AND bucket_start >= ? AND bucket_start < ?
		ORDER BY bucket_start
	`, arguments...)
	if err != nil {
		return ResourceAggregate{}, fmt.Errorf("read feature resource aggregate rollup: %w", err)
	}
	rowCount := 0
	for rows.Next() {
		var source ResourceAggregate
		var sourceStart int64
		var resourcesPayload []byte
		var firstOccurredAt, lastOccurredAt string
		if err := rows.Scan(
			&source.ViewKey, &sourceStart, &source.BucketSeconds, &source.ReportCount,
			&source.Victories, &source.Defeats, &source.TroopsSent, &source.TroopLosses,
			&source.ToolsUsed, &source.GallantryPoints, &source.LootTotal,
			&resourcesPayload, &firstOccurredAt, &lastOccurredAt, &source.Revision,
		); err != nil {
			rows.Close()
			return ResourceAggregate{}, fmt.Errorf("scan feature resource aggregate rollup: %w", err)
		}
		source.BucketStart = time.Unix(sourceStart, 0).UTC()
		source.FirstOccurredAt, _ = time.Parse(time.RFC3339Nano, firstOccurredAt)
		source.LastOccurredAt, _ = time.Parse(time.RFC3339Nano, lastOccurredAt)
		source.Resources = map[string]int64{}
		if len(resourcesPayload) > 0 {
			if err := json.Unmarshal(resourcesPayload, &source.Resources); err != nil {
				rows.Close()
				return ResourceAggregate{}, fmt.Errorf("decode feature resource aggregate rollup resources: %w", err)
			}
		}
		addResourceAggregate(&rollup, source)
		rowCount++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ResourceAggregate{}, fmt.Errorf("read feature resource aggregate rollup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return ResourceAggregate{}, fmt.Errorf("close feature resource aggregate rollup: %w", err)
	}
	if rowCount == 0 {
		rollup.Deleted = true
		rollup.Resources = map[string]int64{}
	}
	return rollup, nil
}

func addResourceAggregate(target *ResourceAggregate, source ResourceAggregate) {
	if target == nil {
		return
	}
	target.ReportCount += source.ReportCount
	target.Victories += source.Victories
	target.Defeats += source.Defeats
	target.TroopsSent += source.TroopsSent
	target.TroopLosses += source.TroopLosses
	target.ToolsUsed += source.ToolsUsed
	target.GallantryPoints += source.GallantryPoints
	target.LootTotal += source.LootTotal
	if target.Resources == nil {
		target.Resources = map[string]int64{}
	}
	for key, value := range source.Resources {
		target.Resources[key] += value
	}
	if target.FirstOccurredAt.IsZero() || (!source.FirstOccurredAt.IsZero() && source.FirstOccurredAt.Before(target.FirstOccurredAt)) {
		target.FirstOccurredAt = source.FirstOccurredAt
	}
	if source.LastOccurredAt.After(target.LastOccurredAt) {
		target.LastOccurredAt = source.LastOccurredAt
	}
	if source.Revision > target.Revision {
		target.Revision = source.Revision
	}
}

func (store *SQLiteStore) AcknowledgeResourceAggregates(ctx context.Context, items []PendingResourceAggregate) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	if len(items) == 0 {
		return nil
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin feature resource aggregate acknowledgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			DELETE FROM feature_resource_aggregate_outbox
			WHERE identity_key = ? AND view_key = ? AND bucket_start = ? AND revision = ?
		`, item.IdentityKey, item.Aggregate.ViewKey, item.Aggregate.BucketStart.Unix(), item.Aggregate.Revision)
		if err != nil {
			return fmt.Errorf("acknowledge feature resource aggregate: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit feature resource aggregate acknowledgement: %w", err)
	}
	return nil
}
