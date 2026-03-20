package domain

type Category struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

type CategoryCreate struct {
	Name string `json:"name"`
}

type CategoryUpdate struct {
	Name *string `json:"name"`
}
