package users

import "time"

type User struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	FullName   string    `json:"fullName"`
	GivenName  string    `json:"givenName"`
	FamilyName string    `json:"familyName"`
	PictureURL string    `json:"pictureUrl"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
