package App

import "fmt"

const defaultHorseTravelBoostID = -1

func validateHorseTravelBoostID(value int) error {
	switch value {
	case 0, -1, 1007, 1008, 1009:
		return nil
	default:
		return fmt.Errorf("horseTravelBoostId must be -1, 1007, 1008, or 1009")
	}
}

func horseTravelBoostFields(value int) (int, int) {
	switch value {
	case 1007, 1008, 1009:
		return value, 0
	default:
		return defaultHorseTravelBoostID, 1
	}
}

func applyHorseTravelBoost(body *attackBody, value int) {
	if body == nil {
		return
	}
	body.Booster, body.PremiumTravel = horseTravelBoostFields(value)
}
