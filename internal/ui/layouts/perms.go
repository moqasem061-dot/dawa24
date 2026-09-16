package layouts

// Has reports whether the permission slice contains p. Navigation uses it to
// render only the links the acting member may reach (a warehouse clerk must not
// see billing).
func Has(perms []string, p string) bool {
	for _, x := range perms {
		if x == p {
			return true
		}
	}
	return false
}
