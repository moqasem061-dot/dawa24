package layouts

// BrandLogoURL substitutes only the bundled default. Uploaded brand artwork
// keeps its URL, so changing the site logo in settings still takes effect.
func BrandLogoURL(url string) string {
	if url == "" || url == "/static/img/logo.png" {
		return Asset("/static/img/logo-240.avif")
	}
	return Asset(url)
}

func BrandLogoSrcset(url string) string {
	if url != "" && url != "/static/img/logo.png" {
		return ""
	}
	return Asset("/static/img/logo-240.avif") + " 240w, " + Asset("/static/img/logo-480.avif") + " 480w"
}
