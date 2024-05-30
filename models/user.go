package models

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/oops"
	"feedrewind/util"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type UserId int64

type User struct {
	Id            UserId
	Email         string
	Name          string
	ProductUserId ProductUserId
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
		select id, email, name, product_user_id
		from users_without_discarded
		where `+column+` = $1
	`, value)
	var user User
	err := row.Scan(
		&user.Id, &user.Email, &user.Name, &user.ProductUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	return &user, nil
}

func User_Exists(tx pgw.Queryable, userId UserId) (bool, error) {
	row := tx.QueryRow("select 1 from users_without_discarded where id = $1", userId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func User_ExistsByProductUserId(tx pgw.Queryable, productUserId ProductUserId) (bool, error) {
	row := tx.QueryRow("select 1 from users_with_discarded where product_user_id = $1", productUserId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func User_GetProductUserId(tx pgw.Queryable, userId UserId) (ProductUserId, error) {
	row := tx.QueryRow(`select product_user_id from users_without_discarded where id = $1`, userId)
	var productUserId ProductUserId
	err := row.Scan(&productUserId)
	if err != nil {
		return "", err
	}
	return productUserId, nil
}

type UserWithPassword struct {
	Id             UserId
	Email          string
	PasswordDigest string
	AuthToken      string
	Name           string
	ProductUserId  ProductUserId
}

func UserWithPassword_FindByEmail(tx pgw.Queryable, email string) (*UserWithPassword, error) {
	row := tx.QueryRow(`
		select id, email, password_digest, auth_token, name, product_user_id
		from users_without_discarded
		where email = $1
	`, email)
	var user UserWithPassword
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

func UserWithPassword_UpdatePassword(tx pgw.Queryable, id UserId, password string) (*UserWithPassword, error) {
	passwordDigest, err := generatePasswordDigest(password)
	if err != nil {
		return nil, err
	}

	authToken, err := generateAuthToken(tx)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRow(`
		update users_without_discarded
		set password_digest = $1, auth_token = $2 where id = $3
		returning id, email, password_digest, auth_token, name, product_user_id
	`, passwordDigest, authToken, id)
	var user UserWithPassword
	err = row.Scan(
		&user.Id, &user.Email, &user.PasswordDigest, &user.AuthToken, &user.Name, &user.ProductUserId,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func UserWithPassword_Create(
	tx pgw.Queryable, email string, password string, name string, productUserId ProductUserId,
	offerId OfferId, maybeStripeSubscriptionId *string, maybeStripeCustomerId *string,
	maybeBillingInterval *BillingInterval, maybeStripeCurrentPeriodEnd *time.Time,
) (*UserWithPassword, error) {
	passwordDigest, err := generatePasswordDigest(password)
	if err != nil {
		return nil, err
	}

	authToken, err := generateAuthToken(tx)
	if err != nil {
		return nil, err
	}

	idInt, err := mutil.RandomId(tx, "users_with_discarded")
	if err != nil {
		return nil, err
	}
	id := UserId(idInt)

	_, err = tx.Exec(`
		insert into users_without_discarded(
			id, email, password_digest, auth_token, name, product_user_id, offer_id,
			stripe_subscription_id, stripe_customer_id, billing_interval, stripe_current_period_end
		)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, email, passwordDigest, authToken, name, productUserId, offerId, maybeStripeSubscriptionId,
		maybeStripeCustomerId, maybeBillingInterval, maybeStripeCurrentPeriodEnd,
	)
	if util.ViolatesUnique(err, "users_email_without_discarded") {
		return nil, ErrUserAlreadyExists
	} else if err != nil {
		return nil, err
	}

	return &UserWithPassword{
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

		row := tx.QueryRow("select 1 from users_with_discarded where auth_token = $1", authTokenStr)
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

// UserSettings

type DeliveryChannel string

const (
	DeliveryChannelSingleFeed    DeliveryChannel = "single_feed"
	DeliveryChannelMultipleFeeds DeliveryChannel = "multiple_feeds"
	DeliveryChannelEmail         DeliveryChannel = "email"
)

type UserSettings struct {
	UserId               UserId
	Timezone             string
	Version              int
	MaybeDeliveryChannel *DeliveryChannel
}

func UserSettings_Create(tx pgw.Queryable, userId UserId, timezone string) error {
	_, err := tx.Exec(`
		insert into user_settings(user_id, timezone, delivery_channel, version)
		values ($1, $2, null, 1)
	`, userId, timezone)
	return err
}

func UserSettings_Get(tx pgw.Queryable, userId UserId) (*UserSettings, error) {
	row := tx.QueryRow(`
		select timezone, version, delivery_channel from user_settings where user_id = $1
	`, userId)
	var us UserSettings
	us.UserId = userId
	err := row.Scan(&us.Timezone, &us.Version, &us.MaybeDeliveryChannel)
	if err != nil {
		return nil, err
	}

	return &us, nil
}

func UserSettings_SaveTimezone(
	tx pgw.Queryable, userId UserId, timezone string, version int,
) error {
	_, err := tx.Exec(`
		update user_settings set timezone = $1, version = $2 where user_id = $3
	`, timezone, version, userId)
	return err
}

func UserSettings_SaveDeliveryChannelVersion(
	tx pgw.Queryable, userId UserId, deliveryChannel DeliveryChannel, version int,
) error {
	_, err := tx.Exec(`
		update user_settings set delivery_channel = $1, version = $2 where user_id = $3
	`, deliveryChannel, version, userId)
	return err
}

func UserSettings_SaveDeliveryChannel(
	tx pgw.Queryable, userId UserId, deliveryChannel DeliveryChannel,
) error {
	_, err := tx.Exec(`
		update user_settings set delivery_channel = $1 where user_id = $2
	`, deliveryChannel, userId)
	return err
}
