package models

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type UserId int64

type User struct {
	Id             UserId
	Email          string
	PasswordDigest string
	AuthToken      string
	Name           string
	ProductUserId  ProductUserId
}

var ErrPasswordTooShort = errors.New("password is too short")
var ErrUserAlreadyExists = errors.New("user already exists")

// Returns nil if not found
func User_MustFindByEmail(ctx context.Context, tx pgw.Queryable, email string) *User {
	return user_mustFindBy(ctx, tx, "email", email)
}

// Returns nil if not found
func User_MustFindByAuthToken(ctx context.Context, tx pgw.Queryable, authToken string) *User {
	return user_mustFindBy(ctx, tx, "auth_token", authToken)
}

// Returns nil if not found
func user_mustFindBy(ctx context.Context, tx pgw.Queryable, column string, value string) *User {
	row := tx.QueryRow(ctx, `
		select id, email, password_digest, auth_token, name, product_user_id
		from users
		where `+column+` = $1
	`, value)
	var user User
	err := row.Scan(
		&user.Id, &user.Email, &user.PasswordDigest, &user.AuthToken, &user.Name, &user.ProductUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		panic(err)
	}

	return &user
}

func User_MustExistsByProductUserId(ctx context.Context, tx pgw.Queryable, productUserId ProductUserId) bool {
	row := tx.QueryRow(ctx, "select 1 from users where product_user_id = $1", productUserId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	} else if err != nil {
		panic(err)
	}

	return true
}

func User_UpdatePassword(ctx context.Context, tx pgw.Queryable, id UserId, password string) (*User, error) {
	passwordDigest, err := generatePasswordDigest(password)
	if err != nil {
		return nil, err
	}

	authToken, err := generateAuthToken(ctx, tx)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx, `
		update users
		set password_digest = $1, auth_token = $2 where id = $3
		returning id, email, password_digest, auth_token, name, product_user_id
	`, passwordDigest, authToken, id)
	var user User
	err = row.Scan(
		&user.Id, &user.Email, &user.PasswordDigest, &user.AuthToken, &user.Name, &user.ProductUserId,
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't update password in db: %v", err)
	}
	return &user, nil
}

func User_Create(
	ctx context.Context, tx pgw.Queryable, email string, password string, name string,
	productUserId ProductUserId,
) (*User, error) {
	passwordDigest, err := generatePasswordDigest(password)
	if err != nil {
		return nil, fmt.Errorf("couldn't generate password digest: %v", err)
	}

	authToken, err := generateAuthToken(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("couldn't generate auth token: %v", err)
	}

	id := UserId(mutil.MustGenerateRandomId(ctx, tx, "users"))

	_, err = tx.Exec(ctx, `
		insert into users(id, email, password_digest, auth_token, name, product_user_id)
		values($1, $2, $3, $4, $5, $6)
	`, id, email, passwordDigest, authToken, name, productUserId)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
		pgErr.ConstraintName == "users_email_unique" {
		return nil, ErrUserAlreadyExists
	} else if err != nil {
		return nil, fmt.Errorf("couldn't create user: %v", err)
	}

	return &User{
		Id:             id,
		Email:          email,
		PasswordDigest: passwordDigest,
		AuthToken:      authToken,
		Name:           name,
		ProductUserId:  productUserId,
	}, nil
}

func generatePasswordDigest(password string) (string, error) {
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}

	passwordDigestBytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("couldn't hash password: %v", err)
	}
	return string(passwordDigestBytes), nil
}

func generateAuthToken(ctx context.Context, tx pgw.Queryable) (string, error) {
	authTokenBytes := make([]byte, config.AuthTokenLength)
	for {
		_, err := rand.Reader.Read(authTokenBytes)
		if err != nil {
			return "", fmt.Errorf("couldn't read random bytes: %v", err)
		}
		authTokenStr := base64.RawStdEncoding.EncodeToString(authTokenBytes)

		row := tx.QueryRow(ctx, "select 1 from users where auth_token = $1", authTokenStr)
		var one int
		err = row.Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return authTokenStr, nil
		} else if err != nil {
			return "", fmt.Errorf("couldn't query auth token: %v", err)
		}

		// continue
	}
}
