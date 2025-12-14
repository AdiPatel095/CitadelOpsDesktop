package GameWebsocket

// CalculateLID calculates the Leader ID based on the commander's target index.
// Logic:
// Index 0 -> LID 0
// Index 1-20 -> LID = Index + 1
// Index 21+ -> LID = Index + 7
func CalculateLID(targetIndex int) int {
	if targetIndex == 0 {
		return 0
	} else if targetIndex >= 1 && targetIndex <= 20 {
		return targetIndex + 1
	} else {
		// Index 21+
		return targetIndex + 8
	}
}
