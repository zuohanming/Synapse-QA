package model

type MockGenerator struct {
	Category    string `json:"category"`
	CategoryCN  string `json:"categoryCn"`
	Placeholder string `json:"placeholder"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Syntax      string `json:"syntax"`
	Example     string `json:"example"`
}

type MockPreviewRequest struct {
	Placeholder string `json:"placeholder"`
	Count       int    `json:"count"`
}

type MockPreviewResponse struct {
	Placeholder string   `json:"placeholder"`
	Results     []string `json:"results"`
}
