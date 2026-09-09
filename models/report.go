package models

type HeatMapCell struct {
	Stage string  `json:"stage"`
	Value float64 `json:"value"`
	Leads int     `json:"leads"`
}

type HeatMapRepRow struct {
	Rep    string        `json:"rep"`
	Stages []HeatMapCell `json:"stages"`
	Total  HeatMapCell   `json:"total"`
}

type HeatMapResponse struct {
	Reps       []HeatMapRepRow `json:"reps"`
	StageTotal []HeatMapCell   `json:"stageTotal"`
	GrandTotal HeatMapCell     `json:"grandTotal"`
}

type LeadAggregationRow struct {
	Owner string
	Stage string
	Value float64
	Count int
}
