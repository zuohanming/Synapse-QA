package model

import "time"

type UIAsset struct {
	ID          int64     `json:"id"`
	AssetType   string    `json:"assetType"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Method      string    `json:"method"`
	Locator     string    `json:"locator"`
	Action      string    `json:"action"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UIAssetRequest struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Method      string `json:"method"`
	Locator     string `json:"locator"`
	Action      string `json:"action"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type UIAssetFilter struct {
	Keyword  string
	ID       string
	PageName string
	PageURL  string
	Product  string
	Module   string
}

type PageElement struct {
	ID        int64     `json:"id"`
	PageID    int64     `json:"pageId"`
	Name      string    `json:"name"`
	Type1     string    `json:"type1"`
	Locator1  string    `json:"locator1"`
	Index1    string    `json:"index1"`
	Type2     string    `json:"type2"`
	Locator2  string    `json:"locator2"`
	Index2    string    `json:"index2"`
	Type3     string    `json:"type3"`
	Locator3  string    `json:"locator3"`
	Index3    string    `json:"index3"`
	AIPrompt  string    `json:"aiPrompt"`
	WaitTime  string    `json:"waitTime"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type PageElementRequest struct {
	PageID   int64  `json:"pageId"`
	Name     string `json:"name"`
	Type1    string `json:"type1"`
	Locator1 string `json:"locator1"`
	Index1   string `json:"index1"`
	Type2    string `json:"type2"`
	Locator2 string `json:"locator2"`
	Index2   string `json:"index2"`
	Type3    string `json:"type3"`
	Locator3 string `json:"locator3"`
	Index3   string `json:"index3"`
	AIPrompt string `json:"aiPrompt"`
	WaitTime string `json:"waitTime"`
}
