package mongodb

import (
	"net/url"
)

func appendUserPasswordToMongoUri(mongoUri string, user, password string) string {
	if mongoUrlParsed, err := url.Parse(mongoUri); err != nil {
		return mongoUri
	} else {
		mongoUrlParsed.User = url.UserPassword(user, password)
		return mongoUrlParsed.String()
	}
}
