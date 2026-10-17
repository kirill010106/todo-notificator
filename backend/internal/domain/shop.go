package domain

import "time"

type PersonalReward struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Price int64  `json:"price"`
}
type Wallet struct {
	Version       int              `json:"version"`
	Coins         int64            `json:"coins"`
	CustomRewards []PersonalReward `json:"customRewards"`
	Boosters      map[string]int64 `json:"boosters"`
	OwnedItems    []string         `json:"ownedItems"`
	ActiveTheme   string           `json:"activeTheme"`
	ActiveSound   string           `json:"activeSound"`
	ActiveBooster bool             `json:"activeBooster"`
	Imported      bool             `json:"imported"`
}
type ShopItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	Price       int64  `json:"price"`
	Description string `json:"description"`
}

var ShopCatalog = []ShopItem{
	{"ocean", "Океан", "theme", 40, "Спокойный синий акцент"},
	{"violet", "Лаванда", "theme", 40, "Мягкий сиреневый акцент"},
	{"bell", "Колокольчик", "sound", 30, "Лёгкий сигнал завершения"},
	{"xp_boost", "Двойной фокус", "booster", 100, "Следующая засчитанная сессия: 40 XP вместо 20. Задачи и достижения не умножаются."},
}

type ShopAction struct {
	Action string  `json:"action"`
	ItemID string  `json:"item_id"`
	Title  string  `json:"title"`
	Price  int64   `json:"price"`
	Legacy *Wallet `json:"legacy,omitempty"`
}
type LedgerEntry struct {
	ID          int64     `json:"id"`
	Coins       int64     `json:"coins"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
