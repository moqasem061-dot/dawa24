// Package antiscrape refuses automated bulk collection of the catalogue
// listing without getting in a pharmacist's way.
//
// What it defends is one route: GET /catalog. That listing publishes supplier
// identity, net supply price, stock and expiry across the whole market in a
// single paginated view, which is the commercially valuable part of this
// platform and the one thing an afternoon with a HTTP client can take in full.
// Its neighbours are deliberately not guarded — /catalog/{id} yields one
// product per request, and /suppliers, /offers and /jobs are already bounded
// server-side — so this costs one middleware on one route.
//
// The design is three cheap layers rather than one clever one:
//
//  1. classification — refuse callers that announce themselves as a HTTP
//     library or a content harvester, and treat "not obviously a browser" as
//     suspicious rather than as normal;
//  2. budgets — a sliding request allowance per caller per window, so a patient
//     scraper wearing a browser User-Agent still cannot outrun a human;
//  3. penalties — a caller that fills the listing's hidden honeypot field is
//     put on the refused list for an hour.
//
// None of this is a bot-detection product and it is not trying to be. It raises
// the cost of taking the catalogue from "trivial" to "deliberate", at the price
// of one string scan and one Redis INCR per request to /catalog.
// Fingerprinting, JavaScript challenges and proof-of-work were considered and
// rejected: they cost more in latency and support tickets than the marginal
// scraper they stop.
package antiscrape

import (
	"net/http"
	"strings"
)

// Class is what a request looks like it came from.
type Class uint8

const (
	// ClassBrowser is a request that presents as a mainstream browser.
	ClassBrowser Class = iota
	// ClassCrawler is a search or social crawler we want to keep indexing the
	// public pages. Allowed, on a small allowance.
	ClassCrawler
	// ClassUnknown is a request that is neither: no User-Agent, no Accept, or a
	// User-Agent that no browser has ever sent. Served, on a small allowance.
	ClassUnknown
	// ClassAutomation is a request that names itself as a HTTP client, a
	// scraping framework or a content harvester. Refused on data pages.
	ClassAutomation
)

func (c Class) String() string {
	switch c {
	case ClassBrowser:
		return "browser"
	case ClassCrawler:
		return "crawler"
	case ClassAutomation:
		return "automation"
	default:
		return "unknown"
	}
}

// searchCrawlers are the agents that may index the public marketplace.
//
// The line drawn through the whole of this file is not "bot or not a bot" — it
// is **does this caller send a pharmacy back to us, or does it only take the
// price list**. A search engine indexes the catalogue and returns buyers; an
// assistant answering "where can I buy Augmentin" returns a buyer too. A
// training crawler and an SEO backlink harvester return nothing, and take the
// same data.
//
// The User-Agent is not verified by reverse DNS. Doing so costs a DNS lookup on
// the request path to defend against a scraper who could simply have used a
// browser string instead, so the allowance below is small on purpose: claiming
// to be Googlebot buys a lower request budget than claiming to be Chrome.
var searchCrawlers = []string{
	"googlebot", "google-inspectiontool", "adsbot-google", "mediapartners-google",
	"bingbot", "bingpreview", "msnbot",
	"duckduckbot", "applebot", "yandexbot", "baiduspider", "slurp",
	"facebookexternalhit", "twitterbot", "linkedinbot", "pinterestbot",
	"whatsapp", "telegrambot", "discordbot", "embedly", "slackbot",
}

// assistantAgents fetch a page because a person just asked for it.
//
// These are the AI agents the platform wants to work with, and they are a
// different thing from the training crawlers below even though both come from
// the same companies. ChatGPT-User, Perplexity-User and Claude-User appear when
// somebody types a question and the assistant goes to look; OAI-SearchBot and
// friends build the index those answers cite. Both end in a person seeing
// Dawa24 and a link back to it.
//
// The names are close enough to their training counterparts to matter:
// "chatgpt-user" is not "gptbot", "claude-user" is not "claudebot", and
// "perplexity-user" is not "perplexitybot". Classify checks this list first, so
// a substring meant for a harvester can never swallow one of these.
//
// They are metered on the crawler budget, not the browser one: an assistant
// fetching one page for one question fits easily, and a caller wearing the name
// to walk the catalogue does not.
var assistantAgents = []string{
	"chatgpt-user", "oai-searchbot",
	"perplexity-user",
	"claude-user", "claude-searchbot",
	"duckassistbot", "bingbot-assistant",
	"youbot", "phindbot",
}

// automationAgents are refused outright on the guarded routes.
//
// Four groups, and all four are unwanted here for the same reason: none of them
// ends with a pharmacy placing an order.
//
//   - generic HTTP clients and scraping frameworks (curl, requests, scrapy)
//   - headless browser drivers (puppeteer, playwright, selenium)
//   - site mirroring tools
//   - commercial SEO harvesters and AI *training* collectors
//
// The training collectors are the deliberate part. GPTBot, ClaudeBot, CCBot,
// Bytespider and Google-Extended exist to copy the corpus into a model; they
// send nobody here and the site's own robots.txt already declares
// "Content-Signal: ai-train=no". Refusing them in code is that declaration
// enforced. Their user-triggered siblings are in assistantAgents above and are
// let through.
//
// A scraper that changes its User-Agent to Chrome defeats this list in one
// line. It is still worth having: the overwhelming majority of scraping traffic
// never bothers, and the ones that do are then paying the budget of a browser.
var automationAgents = []string{
	// HTTP clients and scripting runtimes
	"curl/", "wget", "libwww-perl", "lwp::", "python-requests", "python-urllib",
	"aiohttp", "httpx/", "urllib", "mechanize", "node-fetch", "axios/",
	"okhttp", "apache-httpclient", "java/", "jakarta", "guzzle", "php/",
	"go-http-client", "ruby", "restsharp", "postmanruntime", "insomnia",
	"httpie", "winhttp", "powershell", "wininet",
	// Scraping and automation frameworks
	"scrapy", "puppeteer", "playwright", "selenium", "webdriver", "phantomjs",
	"headlesschrome", "headless_chrome", "chrome-lighthouse", "cypress",
	// Site mirroring
	"httrack", "webcopier", "webzip", "teleport", "offline explorer",
	"sitesucker", "wget/", "cyotek",
	// Commercial harvesters, SEO crawlers and AI training collectors
	"ahrefsbot", "semrushbot", "mj12bot", "dotbot", "blexbot", "dataforseo",
	"seokicks", "megaindex", "serpstat", "zoominfobot", "petalbot",
	"bytespider", "gptbot", "ccbot", "claudebot", "claude-web", "anthropic-ai",
	"perplexitybot", "amazonbot", "omgili", "diffbot", "imagesiftbot",
	"scrapingbee", "scraperapi", "brightdot", "dataprovider",
	"google-extended", "applebot-extended", "meta-externalagent",
	"meta-externalfetcher", "facebookbot", "cohere-ai", "cohere-training",
	"timpibot", "webzio", "awario", "peer39", "img2dataset",
	// Scanners; not scrapers, but nothing on a storefront wants them either
	"zgrab", "masscan", "nmap", "nikto", "sqlmap", "wpscan",
}

// Classify decides what a request looks like from its headers alone.
//
// Order matters: the crawler allowlist is consulted first so that a genuine
// Googlebot string is not caught by a substring meant for a harvester.
func Classify(r *http.Request) Class {
	ua := strings.ToLower(strings.TrimSpace(r.Header.Get("User-Agent")))
	if ua == "" {
		return ClassUnknown
	}

	// Allowlists first, always. "chatgpt-user" must not be caught by a
	// substring written for "gptbot", and "claude-user" must not be caught by
	// one written for "claudebot".
	if containsAny(ua, searchCrawlers) || containsAny(ua, assistantAgents) {
		return ClassCrawler
	}
	if containsAny(ua, automationAgents) {
		return ClassAutomation
	}

	// Every mainstream browser still sends the "Mozilla/5.0" compatibility
	// prefix, and a self-respecting HTTP client sends its own name instead.
	// Neither is proof, so a miss downgrades to Unknown rather than refusing.
	if !strings.Contains(ua, "mozilla/") {
		return ClassUnknown
	}

	// A browser asks for something. A request with no Accept header at all is a
	// script that forgot to set one.
	if strings.TrimSpace(r.Header.Get("Accept")) == "" {
		return ClassUnknown
	}

	return ClassBrowser
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
