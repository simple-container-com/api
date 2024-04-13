package mongodb

import (
	"net/url"
)

func appendUserPasswordAndDBToMongoUri(mongoUri string, user, password, dbName string) string {
	if mongoUrlParsed, err := url.Parse(mongoUri); err != nil {
		return mongoUri
	} else {
		mongoUrlParsed.User = url.UserPassword(user, password)
		mongoUrlParsed.Path = dbName
		return mongoUrlParsed.String()
	}
}
