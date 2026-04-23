package service

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

func postgresCLIConnInfo(dsn string) (string, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return "", fmt.Errorf("postgres dsn is empty")
	}

	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		return trimmed, nil
	}

	parsed, err := parsePostgresURLDSN(trimmed)
	if err != nil {
		return "", err
	}

	parts := make([]string, 0, 6+len(parsed.params))
	appendPart := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, quoteLibpqConnValue(value)))
	}

	appendPart("host", parsed.host)
	appendPart("port", parsed.port)
	appendPart("user", parsed.user)
	appendPart("password", parsed.password)
	appendPart("dbname", parsed.dbname)

	keys := make([]string, 0, len(parsed.params))
	for key := range parsed.params {
		switch key {
		case "host", "port", "user", "password", "dbname":
			continue
		default:
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendPart(key, parsed.params[key])
	}

	return strings.Join(parts, " "), nil
}

type parsedPostgresURL struct {
	host     string
	port     string
	user     string
	password string
	dbname   string
	params   map[string]string
}

func parsePostgresURLDSN(dsn string) (*parsedPostgresURL, error) {
	remainder := dsn
	if idx := strings.Index(remainder, "://"); idx >= 0 {
		remainder = remainder[idx+3:]
	}

	pathStart := strings.Index(remainder, "/")
	authority := remainder
	pathAndQuery := ""
	if pathStart >= 0 {
		authority = remainder[:pathStart]
		pathAndQuery = remainder[pathStart+1:]
	}
	if authority == "" {
		return nil, fmt.Errorf("postgres dsn missing authority")
	}

	userinfo := ""
	hostport := authority
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		userinfo = authority[:at]
		hostport = authority[at+1:]
	}

	host, port, err := splitPostgresHostPort(hostport)
	if err != nil {
		return nil, err
	}

	user := ""
	password := ""
	if userinfo != "" {
		if colon := strings.Index(userinfo, ":"); colon >= 0 {
			user = userinfo[:colon]
			password = userinfo[colon+1:]
		} else {
			user = userinfo
		}
		decodedUser, err := url.PathUnescape(user)
		if err != nil {
			return nil, fmt.Errorf("decode postgres username: %w", err)
		}
		decodedPassword, err := url.PathUnescape(password)
		if err != nil {
			return nil, fmt.Errorf("decode postgres password: %w", err)
		}
		user = decodedUser
		password = decodedPassword
	}

	dbname := ""
	rawQuery := ""
	if q := strings.Index(pathAndQuery, "?"); q >= 0 {
		dbname = pathAndQuery[:q]
		rawQuery = pathAndQuery[q+1:]
	} else {
		dbname = pathAndQuery
	}
	if dbname != "" {
		decodedDBName, err := url.PathUnescape(dbname)
		if err != nil {
			return nil, fmt.Errorf("decode postgres database name: %w", err)
		}
		dbname = decodedDBName
	}

	params := map[string]string{}
	if rawQuery != "" {
		values, err := url.ParseQuery(rawQuery)
		if err != nil {
			return nil, fmt.Errorf("parse postgres dsn query: %w", err)
		}
		for key, list := range values {
			if len(list) == 0 {
				continue
			}
			params[key] = list[len(list)-1]
		}
	}

	if host == "" {
		host = params["host"]
	}
	if port == "" {
		port = params["port"]
	}
	if user == "" {
		user = params["user"]
	}
	if password == "" {
		password = params["password"]
	}
	if dbname == "" {
		dbname = params["dbname"]
	}

	return &parsedPostgresURL{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		dbname:   dbname,
		params:   params,
	}, nil
}

func splitPostgresHostPort(hostport string) (string, string, error) {
	if hostport == "" {
		return "", "", nil
	}
	if strings.HasPrefix(hostport, "[") {
		host, port, err := net.SplitHostPort(hostport)
		if err != nil {
			return "", "", fmt.Errorf("parse postgres host/port: %w", err)
		}
		return host, port, nil
	}
	if strings.Count(hostport, ":") == 1 {
		host, port, err := net.SplitHostPort(hostport)
		if err == nil {
			return host, port, nil
		}
	}
	return hostport, "", nil
}

func quoteLibpqConnValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}
