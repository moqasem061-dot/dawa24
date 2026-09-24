package database

// QuoteLiteral escapes s as a SQL string literal. It exists for the statements
// that cannot take a bind parameter, CREATE ROLE and ALTER ROLE ... PASSWORD
// among them; everywhere else, use a parameter.
func QuoteLiteral(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(append(out, '\''))
}
