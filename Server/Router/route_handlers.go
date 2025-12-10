package Router

import (
	"CitadelDesktop/Server/License"
	"encoding/json"
	"net/http"
)

func HandleReconfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		HardwareID string `json:"hardwareID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Cost is hardcoded for now or could be passed in request if validated, but server should enforce
	const cost = 10000

	if !License.UseCredits(cost, "reconfigure") {
		http.Error(w, "Insufficient credits", http.StatusPaymentRequired)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Reconfiguration successful",
	})
}

func HandleGetLicense(w http.ResponseWriter, r *http.Request) {
	hardwareID := License.GetHardwareID()
	credits := License.GetCredits()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hardwareID": hardwareID,
		"credits":    credits,
	})
}
