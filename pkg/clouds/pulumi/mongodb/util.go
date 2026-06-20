// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package mongodb

import (
	"net/url"
	"strings"
)

// AppendUserPasswordAndDBToMongoUri injects credentials and a database
// name into a MongoDB connection URI.
//
// Implementation note: MongoDB URIs allow a comma-separated host list
// (`mongodb://a:27017,b:27017,c:27017/db?opts`), which `net/url.Parse`
// rejected outright starting with Go 1.26 ("invalid port after host").
// We do the surgery on strings instead so multi-host replica-set URIs
// keep working.
func AppendUserPasswordAndDBToMongoUri(mongoUri, user, password, dbName string) string {
	schemeEnd := strings.Index(mongoUri, "://")
	if schemeEnd == -1 {
		return mongoUri
	}
	scheme := mongoUri[:schemeEnd]
	rest := mongoUri[schemeEnd+3:]

	if at := strings.LastIndex(rest, "@"); at != -1 {
		rest = rest[at+1:]
	}

	hosts := rest
	query := ""
	if q := strings.IndexByte(rest, '?'); q != -1 {
		hosts = rest[:q]
		query = rest[q:]
	}
	if p := strings.IndexByte(hosts, '/'); p != -1 {
		hosts = hosts[:p]
	}

	creds := url.UserPassword(user, password).String()
	return scheme + "://" + creds + "@" + hosts + "/" + dbName + query
}
