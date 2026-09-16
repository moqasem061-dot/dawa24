package pages

// Media loading policy for the landing page hero carousel.
//
// Every slide is in the DOM at once — that is how x-show works — so without
// this the browser fetches, decodes and starts playing all of them before the
// reader has seen the first. On a phone that is several megabytes and several
// decoder pipelines competing for the main thread, which is what "the page
// freezes" looks like from the outside.

// heroImageLoading loads the first slide eagerly, because it is the largest
// contentful paint, and defers the rest.
func heroImageLoading(index int) string {
	if index == 0 {
		return "eager"
	}
	return "lazy"
}

// heroMediaPreload gives the first slide's video its metadata so it can start
// immediately, and fetches nothing at all for slides behind it.
func heroMediaPreload(index int) string {
	if index == 0 {
		return "metadata"
	}
	return "none"
}
