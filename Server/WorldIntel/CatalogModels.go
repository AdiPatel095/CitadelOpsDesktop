package WorldIntel

import (
	"encoding/json"
	"time"
)

const OfficialCatalogSource = "ggs-official-items"

type CatalogDatasetSummary struct {
	DatasetKey       string    `json:"datasetKey"`
	DatasetLabel     string    `json:"datasetLabel"`
	Category         string    `json:"category"`
	Source           string    `json:"source"`
	SourceVersion    string    `json:"sourceVersion"`
	SourceURL        string    `json:"sourceUrl"`
	DatasetDigest    string    `json:"datasetDigest"`
	Fields           []string  `json:"fields"`
	RowCount         int       `json:"rowCount"`
	CapturedAt       time.Time `json:"capturedAt"`
	ContributorCount int64     `json:"contributorCount"`
}

type CatalogDatasetCatalogResponse struct {
	Source   string                  `json:"source"`
	Datasets []CatalogDatasetSummary `json:"datasets"`
}

type CatalogDatasetResponse struct {
	CatalogDatasetSummary
	Rows    json.RawMessage         `json:"rows"`
	History []CatalogDatasetSummary `json:"history"`
}
