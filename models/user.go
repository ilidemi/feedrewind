package models

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/oops"

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
var ErrUserNotFound = errors.New("user not found")

func User_FindByEmail(tx pgw.Queryable, email string) (*User, error) {
	return user_FindBy(tx, "email", email)
}

func User_FindByAuthToken(tx pgw.Queryable, authToken string) (*User, error) {
	return user_FindBy(tx, "auth_token", authToken)
}

func user_FindBy(tx pgw.Queryable, column string, value string) (*User, error) {
	row := tx.QueryRow(`
		select id, email, password_digest, auth_token, name, product_user_id
		from users
		where `+column+` = $1
	`, value)
	var user User
	err := row.Scan(
		&user.Id, &user.Email, &user.PasswordDigest, &user.AuthToken, &user.Name, &user.ProductUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	return &user, nil
}

func User_ExistsByProductUserId(tx pgw.Queryable, productUserId ProductUserId) (bool, error) {
	row := tx.QueryRow("select 1 from users where product_user_id = $1", productUserId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func User_UpdatePassword(tx pgw.Queryable, id UserId, password string) (*User, error) {
	passwordDigest, err := generatePasswordDigest(password)
	if err != nil {
		return nil, err
	}

	authToken, err := generateAuthToken(tx)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRow(`
		update users
		set password_digest = $1, auth_token = $2 where id = $3
		returning id, email, password_digest, auth_token, name, product_user_id
	`, passwordDigest, authToken, id)
	var user User
	err = row.Scan(
		&user.Id, &user.Email, &user.PasswordDigest, &user.AuthToken, &user.Name, &user.ProductUserId,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func User_Create(
	tx pgw.Queryable, email string, password string, name string, productUserId ProductUserId,
) (*User, error) {
	passwordDigest, err := generatePasswordDigest(password)
	if err != nil {
		return nil, err
	}

	authToken, err := generateAuthToken(tx)
	if err != nil {
		return nil, err
	}

	idInt, err := mutil.GenerateRandomId(tx, "users")
	if err != nil {
		return nil, err
	}
	id := UserId(idInt)

	_, err = tx.Exec(`
		insert into users(id, email, password_digest, auth_token, name, product_user_id)
		values($1, $2, $3, $4, $5, $6)
	`, id, email, passwordDigest, authToken, name, productUserId)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
		pgErr.ConstraintName == "users_email_unique" {
		return nil, ErrUserAlreadyExists
	} else if err != nil {
		return nil, err
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

type UserWithRss struct {
	AnySubcriptionNotPausedOrFinished bool
	Rss                               string
	ProductUserId                     ProductUserId
}

func User_GetWithRss(tx pgw.Queryable, userId UserId) (*UserWithRss, error) {
	row := tx.QueryRow(`
		select
			(
				select count(1) from subscriptions
				where subscriptions.user_id = $1 and
					not is_paused and
					final_item_published_at is null
			) > 0,
			(select body from user_rsses where user_id = $1),
			product_user_id
		from users
		where id = $1
	`, userId)
	var u UserWithRss
	err := row.Scan(&u.AnySubcriptionNotPausedOrFinished, &u.Rss, &u.ProductUserId)
	return &u, err
}

func generatePasswordDigest(password string) (string, error) {
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}

	passwordDigestBytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", oops.Wrap(err)
	}
	return string(passwordDigestBytes), nil
}

func generateAuthToken(tx pgw.Queryable) (string, error) {
	authTokenBytes := make([]byte, config.AuthTokenLength)
	for {
		_, err := rand.Reader.Read(authTokenBytes)
		if err != nil {
			return "", oops.Wrap(err)
		}
		authTokenStr := base64.RawStdEncoding.EncodeToString(authTokenBytes)

		row := tx.QueryRow("select 1 from users where auth_token = $1", authTokenStr)
		var one int
		err = row.Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return authTokenStr, nil
		} else if err != nil {
			return "", err
		}

		// continue
	}
}
