package service

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

type postgresCLIConnectionInfo struct {
	connInfo string
	env      []string
}

// postgresCLIConnInfo converts DSNs into libpq conninfo plus password environment.
func postgresCLIConnInfo(dsn string) (postgresCLIConnectionInfo, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return postgresCLIConnectionInfo{}, fmt.Errorf("postgres dsn is empty")
	}

	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		if conn, ok, err := postgresKeywordCLIConnInfo(trimmed); ok || err != nil {
			return conn, err
		}
		return postgresCLIConnectionInfo{connInfo: trimmed}, nil
	}

	parsed, err := parsePostgresURLDSN(trimmed)
	if err != nil {
		return postgresCLIConnectionInfo{}, err
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

	return postgresCLIConnectionInfo{
		connInfo: strings.Join(parts, " "),
		env:      postgresPasswordEnv(parsed.password),
	}, nil
}

type parsedPostgresURL struct {
	host     string
	port     string
	user     string
	password string
	dbname   string
	params   map[string]string
}

// parsePostgresURLDSN parses URL-form PostgreSQL DSNs without logging secrets.
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

// splitPostgresHostPort handles host, host:port, and bracketed IPv6 authorities.
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

// quoteLibpqConnValue quotes a value for libpq keyword/value conninfo.
func quoteLibpqConnValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

type postgresConnInfoParam struct {
	key   string
	value string
}

// postgresKeywordCLIConnInfo removes password from keyword conninfo and moves it to env.
func postgresKeywordCLIConnInfo(connInfo string) (postgresCLIConnectionInfo, bool, error) {
	params, ok, err := parsePostgresKeywordConnInfo(connInfo)
	if !ok || err != nil {
		return postgresCLIConnectionInfo{}, ok, err
	}

	password := ""
	parts := make([]string, 0, len(params))
	for _, param := range params {
		if strings.EqualFold(param.key, "password") {
			password = param.value
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", param.key, quoteLibpqConnValue(param.value)))
	}

	return postgresCLIConnectionInfo{
		connInfo: strings.Join(parts, " "),
		env:      postgresPasswordEnv(password),
	}, true, nil
}

// parsePostgresKeywordConnInfo parses libpq keyword/value connection strings.
func parsePostgresKeywordConnInfo(connInfo string) ([]postgresConnInfoParam, bool, error) {
	params := []postgresConnInfoParam{}
	index := 0
	for {
		skipPostgresConnInfoSpaces(connInfo, &index)
		if index >= len(connInfo) {
			break
		}

		keyStart := index
		for index < len(connInfo) && connInfo[index] != '=' && !isPostgresConnInfoSpace(connInfo[index]) {
			index++
		}
		if keyStart == index {
			return nil, len(params) > 0, fmt.Errorf("parse postgres conninfo near %q", connInfo[index:])
		}
		key := connInfo[keyStart:index]
		skipPostgresConnInfoSpaces(connInfo, &index)
		if index >= len(connInfo) || connInfo[index] != '=' {
			if len(params) == 0 {
				return nil, false, nil
			}
			return nil, true, fmt.Errorf("parse postgres conninfo key %q: missing '='", key)
		}
		index++
		skipPostgresConnInfoSpaces(connInfo, &index)

		value, err := parsePostgresConnInfoValue(connInfo, &index)
		if err != nil {
			return nil, true, err
		}
		params = append(params, postgresConnInfoParam{key: key, value: value})
	}

	if len(params) == 0 {
		return nil, false, nil
	}
	return params, true, nil
}

// parsePostgresConnInfoValue reads one quoted or unquoted libpq value.
func parsePostgresConnInfoValue(connInfo string, index *int) (string, error) {
	if *index >= len(connInfo) {
		return "", nil
	}
	if connInfo[*index] != '\'' {
		start := *index
		for *index < len(connInfo) && !isPostgresConnInfoSpace(connInfo[*index]) {
			*index = *index + 1
		}
		return connInfo[start:*index], nil
	}

	*index = *index + 1
	var value strings.Builder
	for *index < len(connInfo) {
		ch := connInfo[*index]
		*index = *index + 1
		if ch == '\'' {
			return value.String(), nil
		}
		if ch == '\\' && *index < len(connInfo) {
			ch = connInfo[*index]
			*index = *index + 1
		}
		value.WriteByte(ch)
	}
	return "", fmt.Errorf("parse postgres conninfo: unterminated quoted value")
}

// skipPostgresConnInfoSpaces advances over libpq whitespace bytes.
func skipPostgresConnInfoSpaces(connInfo string, index *int) {
	for *index < len(connInfo) && isPostgresConnInfoSpace(connInfo[*index]) {
		*index = *index + 1
	}
}

// isPostgresConnInfoSpace mirrors libpq whitespace accepted between key/value tokens.
func isPostgresConnInfoSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

// postgresPasswordEnv returns a PGPASSWORD environment assignment when needed.
func postgresPasswordEnv(password string) []string {
	if password == "" {
		return nil
	}
	return []string{"PGPASSWORD=" + password}
}
