package config

import (
	"errors"
	"net/url"
)

// withDatabasePassword puts the raw password of ADMIN_DATABASE_PASSWORD into
// the database URL, percent-encoded, so a password with @ : / # % works
// without hand-encoding and the database container and the API share one
// value. Without a password the URL is used as it is. Errors never quote the
// URL or the password.
func withDatabasePassword(rawURL, password string) (string, error) {
	if password == "" || rawURL == "" {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.New("database.url is not a valid URL")
	}
	if u.User == nil || u.User.Username() == "" {
		return "", errors.New("database.password needs a user name in database.url, as mysql://admin@mysql:3306/base_admin")
	}
	if _, set := u.User.Password(); set {
		return "", errors.New("database.url already contains a password; leave it out when database.password (ADMIN_DATABASE_PASSWORD) is set")
	}
	u.User = url.UserPassword(u.User.Username(), password)
	return u.String(), nil
}
