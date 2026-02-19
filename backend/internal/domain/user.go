package domain

type User struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	PassHash []byte `json:"-"` // никогда не отдаем в JSON
}
