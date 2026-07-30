package State

const ConstructionItemInventoryLimit int64 = 1000

func ConstructionItemInventoryCount(items map[ConstructionItemID]int64) int64 {
	var total int64
	for _, amount := range items {
		if amount > 0 {
			total += amount
		}
	}
	return total
}
