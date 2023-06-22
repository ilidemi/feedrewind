package models

import (
	"context"
	"errors"
	"feedrewind/db"
	"time"

	"github.com/jackc/pgx/v5"
)

type UserId int64

type User struct {
	Id             UserId
	Email          string
	PasswordDigest string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AuthToken      string
	Name           string
	ProductUserId  ProductUserId
}

// Returns nil if not found
func User_MustFindByEmail(email string) *User {
	return user_mustFindBy("email", email)
}

// Returns nil if not found
func User_MustFindByAuthToken(authToken string) *User {
	return user_mustFindBy("auth_token", authToken)
}

// Returns nil if not found
func user_mustFindBy(column string, value string) *User {
	ctx := context.Background()
	row := db.Conn.QueryRow(ctx, `
		select id, email, password_digest, created_at, updated_at, auth_token, name, product_user_id
		from users
		where `+column+` = $1
	`, value)
	var user User
	err := row.Scan(
		&user.Id, &user.Email, &user.PasswordDigest, &user.CreatedAt, &user.UpdatedAt, &user.AuthToken,
		&user.Name, &user.ProductUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		panic(err)
	}

	return &user
}
