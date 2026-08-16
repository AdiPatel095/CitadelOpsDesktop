package State

import "sort"

const reportShardCount = 256

type reportRecord struct {
	Notice    ReportNotice
	Spy       SpyReportCapture
	Battle    BattleReportCapture
	HasNotice bool
	HasSpy    bool
	HasBattle bool
}

type reportGeneration struct {
	shards [reportShardCount]map[int64]reportRecord
}

func reportShardIndex(messageID int64) uint8 {
	value := uint64(messageID)
	value ^= value >> 33
	value *= 0xff51afd7ed558ccd
	value ^= value >> 33
	return uint8(value)
}

func reportGenerationFromState(source ReportState) *reportGeneration {
	generation := &reportGeneration{}
	set := func(messageID int64, update func(*reportRecord)) {
		if messageID <= 0 {
			return
		}
		shard := reportShardIndex(messageID)
		if generation.shards[shard] == nil {
			generation.shards[shard] = map[int64]reportRecord{}
		}
		record := generation.shards[shard][messageID]
		update(&record)
		generation.shards[shard][messageID] = record
	}
	for id, notice := range source.Notices {
		value := notice
		set(id, func(record *reportRecord) { record.Notice, record.HasNotice = value, true })
	}
	for id, capture := range source.SpyCaptures {
		value := capture
		set(id, func(record *reportRecord) { record.Spy, record.HasSpy = value, true })
	}
	for id, capture := range source.BattleCaptures {
		value := capture
		set(id, func(record *reportRecord) { record.Battle, record.HasBattle = value, true })
	}
	return generation
}

func (state *GameState) initializeReports() {
	if state == nil || state.reportRecords != nil {
		return
	}
	state.reportRecords = reportGenerationFromState(state.Reports)
	state.Reports.Notices = nil
	state.Reports.SpyCaptures = nil
	state.Reports.BattleCaptures = nil
}

func (state *GameState) prepareReportMutation(source GameState) {
	state.Reports = source.Reports
	base := source.reportRecords
	if base == nil {
		base = reportGenerationFromState(source.Reports)
	}
	generation := *base
	state.reportRecords = &generation
	state.Reports.Notices = nil
	state.Reports.SpyCaptures = nil
	state.Reports.BattleCaptures = nil
	state.reportMutationCOW = true
	state.mutableReportShards = [4]uint64{}
	state.pendingReportMessages = map[int64]struct{}{}
	state.replaceReports = false
}

func (state GameState) reportRecord(messageID int64) (reportRecord, bool) {
	if messageID <= 0 {
		return reportRecord{}, false
	}
	if state.reportRecords != nil {
		record, found := state.reportRecords.shards[reportShardIndex(messageID)][messageID]
		return record, found
	}
	record := reportRecord{}
	if notice, found := state.Reports.Notices[messageID]; found {
		record.Notice, record.HasNotice = notice, true
	}
	if capture, found := state.Reports.SpyCaptures[messageID]; found {
		record.Spy, record.HasSpy = capture, true
	}
	if capture, found := state.Reports.BattleCaptures[messageID]; found {
		record.Battle, record.HasBattle = capture, true
	}
	return record, record.HasNotice || record.HasSpy || record.HasBattle
}

func (state *GameState) mutableReportRecord(messageID int64) reportRecord {
	if state.reportRecords == nil {
		state.reportRecords = reportGenerationFromState(state.Reports)
		state.Reports.Notices = nil
		state.Reports.SpyCaptures = nil
		state.Reports.BattleCaptures = nil
	}
	shard := reportShardIndex(messageID)
	word, bit := shard/64, uint(shard%64)
	if !state.reportMutationCOW || state.mutableReportShards[word]&(uint64(1)<<bit) == 0 {
		state.reportRecords.shards[shard] = cloneMap(state.reportRecords.shards[shard])
		if state.reportMutationCOW {
			state.mutableReportShards[word] |= uint64(1) << bit
		}
	}
	if state.reportRecords.shards[shard] == nil {
		state.reportRecords.shards[shard] = map[int64]reportRecord{}
	}
	return state.reportRecords.shards[shard][messageID]
}

func (state *GameState) storeReportRecord(messageID int64, record reportRecord) {
	shard := reportShardIndex(messageID)
	if !record.HasNotice && !record.HasSpy && !record.HasBattle {
		delete(state.reportRecords.shards[shard], messageID)
	} else {
		state.reportRecords.shards[shard][messageID] = record
	}
	state.markReportMessage(messageID)
}

func (state GameState) LookupReportNotice(messageID int64) (ReportNotice, bool) {
	record, found := state.reportRecord(messageID)
	return record.Notice, found && record.HasNotice
}

func (state GameState) LookupSpyReportCapture(messageID int64) (SpyReportCapture, bool) {
	record, found := state.reportRecord(messageID)
	return record.Spy, found && record.HasSpy
}

func (state GameState) LookupBattleReportCapture(messageID int64) (BattleReportCapture, bool) {
	record, found := state.reportRecord(messageID)
	return record.Battle, found && record.HasBattle
}

func (state GameState) RangeReportNotices(visit func(int64, ReportNotice) bool) {
	state.rangeReportRecords(func(id int64, record reportRecord) bool {
		return !record.HasNotice || visit(id, record.Notice)
	})
}

func (state GameState) RangeSpyReportCaptures(visit func(int64, SpyReportCapture) bool) {
	state.rangeReportRecords(func(id int64, record reportRecord) bool {
		return !record.HasSpy || visit(id, record.Spy)
	})
}

func (state GameState) RangeBattleReportCaptures(visit func(int64, BattleReportCapture) bool) {
	state.rangeReportRecords(func(id int64, record reportRecord) bool {
		return !record.HasBattle || visit(id, record.Battle)
	})
}

func (state GameState) rangeReportRecords(visit func(int64, reportRecord) bool) {
	if visit == nil {
		return
	}
	if state.reportRecords == nil {
		generation := reportGenerationFromState(state.Reports)
		for _, shard := range generation.shards {
			for id, record := range shard {
				if !visit(id, record) {
					return
				}
			}
		}
		return
	}
	for _, shard := range state.reportRecords.shards {
		for id, record := range shard {
			if !visit(id, record) {
				return
			}
		}
	}
}

func (state *GameState) markReportMessage(messageID int64) {
	if state != nil && state.reportMutationCOW && !state.replaceReports && messageID > 0 {
		state.pendingReportMessages[messageID] = struct{}{}
	}
}

func (state *GameState) SetReportNotice(messageID int64, notice ReportNotice) {
	if state == nil || messageID <= 0 {
		return
	}
	if !state.reportMutationCOW && state.reportRecords == nil {
		if state.Reports.Notices == nil {
			state.Reports.Notices = map[int64]ReportNotice{}
		}
		state.Reports.Notices[messageID] = notice
		return
	}
	record := state.mutableReportRecord(messageID)
	record.Notice, record.HasNotice = notice, true
	state.storeReportRecord(messageID, record)
}

func (state *GameState) DeleteReportNotice(messageID int64) {
	if state == nil || messageID <= 0 {
		return
	}
	if !state.reportMutationCOW && state.reportRecords == nil {
		delete(state.Reports.Notices, messageID)
		return
	}
	record := state.mutableReportRecord(messageID)
	record.Notice, record.HasNotice = ReportNotice{}, false
	state.storeReportRecord(messageID, record)
}

func (state *GameState) SetSpyReportCapture(messageID int64, capture SpyReportCapture) {
	if state == nil || messageID <= 0 {
		return
	}
	if !state.reportMutationCOW && state.reportRecords == nil {
		if state.Reports.SpyCaptures == nil {
			state.Reports.SpyCaptures = map[int64]SpyReportCapture{}
		}
		state.Reports.SpyCaptures[messageID] = capture
		return
	}
	record := state.mutableReportRecord(messageID)
	record.Spy, record.HasSpy = capture, true
	state.storeReportRecord(messageID, record)
}

func (state *GameState) DeleteSpyReportCapture(messageID int64) {
	if state == nil || messageID <= 0 {
		return
	}
	if !state.reportMutationCOW && state.reportRecords == nil {
		delete(state.Reports.SpyCaptures, messageID)
		return
	}
	record := state.mutableReportRecord(messageID)
	record.Spy, record.HasSpy = SpyReportCapture{}, false
	state.storeReportRecord(messageID, record)
}

func (state *GameState) SetBattleReportCapture(messageID int64, capture BattleReportCapture) {
	if state == nil || messageID <= 0 {
		return
	}
	if !state.reportMutationCOW && state.reportRecords == nil {
		if state.Reports.BattleCaptures == nil {
			state.Reports.BattleCaptures = map[int64]BattleReportCapture{}
		}
		state.Reports.BattleCaptures[messageID] = capture
		return
	}
	record := state.mutableReportRecord(messageID)
	record.Battle, record.HasBattle = capture, true
	state.storeReportRecord(messageID, record)
}

func (state *GameState) DeleteBattleReportCapture(messageID int64) {
	if state == nil || messageID <= 0 {
		return
	}
	if !state.reportMutationCOW && state.reportRecords == nil {
		delete(state.Reports.BattleCaptures, messageID)
		return
	}
	record := state.mutableReportRecord(messageID)
	record.Battle, record.HasBattle = BattleReportCapture{}, false
	state.storeReportRecord(messageID, record)
}

func (state *GameState) ReplaceReports(value ReportState) {
	if state == nil {
		return
	}
	state.Reports = value
	state.reportRecords = reportGenerationFromState(value)
	state.Reports.Notices, state.Reports.SpyCaptures, state.Reports.BattleCaptures = nil, nil, nil
	if state.reportMutationCOW {
		state.replaceReports = true
		state.pendingReportMessages = map[int64]struct{}{}
		state.mutableReportShards = [4]uint64{}
	}
}

func (state GameState) materializedReports() ReportState {
	reports := ReportState{
		Notices: map[int64]ReportNotice{}, SpyCaptures: map[int64]SpyReportCapture{},
		BattleCaptures: map[int64]BattleReportCapture{}, ActiveBattleReport: state.Reports.ActiveBattleReport,
	}
	state.rangeReportRecords(func(id int64, record reportRecord) bool {
		if record.HasNotice {
			reports.Notices[id] = record.Notice
		}
		if record.HasSpy {
			capture := record.Spy
			capture.Payload = append([]byte(nil), capture.Payload...)
			reports.SpyCaptures[id] = capture
		}
		if record.HasBattle {
			capture := record.Battle
			capture.Summary = append([]byte(nil), capture.Summary...)
			capture.Waves = append([]byte(nil), capture.Waves...)
			capture.Details = append([]byte(nil), capture.Details...)
			reports.BattleCaptures[id] = capture
		}
		return true
	})
	return reports
}

func (state GameState) reportMessageChangeIDs() []int64 {
	ids := make([]int64, 0, len(state.pendingReportMessages))
	for id := range state.pendingReportMessages {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state GameState) reportMessageIDs() []int64 {
	ids := []int64{}
	state.rangeReportRecords(func(id int64, _ reportRecord) bool {
		ids = append(ids, id)
		return true
	})
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}
