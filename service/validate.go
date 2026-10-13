package service

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/labstack/echo/v4"
)

const (
	// maxTagLength bounds the player tag. BattleTags cap out well below this, so
	// anything longer is either a mistake or an attempt to abuse the upstream.
	maxTagLength = 64
)

// tagPattern matches the tags we are willing to forward upstream: unicode letters,
// digits and underscores, optionally followed by a '-' or '#' discriminator.
// Console gamertags may contain spaces, so those are allowed too.
//
// Rejecting everything else is what prevents path traversal ("../"), query
// injection ("?x=y") and fragment truncation ("#") in the upstream URL.
var tagPattern = regexp.MustCompile(`^[\p{L}\p{N}_][\p{L}\p{N}_ .]{1,30}(?:[-#][0-9]{3,10})?$`)

// validPlatforms enumerates the platforms this API accepts.
var validPlatforms = map[string]struct{}{
	ovrstat.PlatformPC:      {},
	ovrstat.PlatformConsole: {},
}

// validatePlatform normalizes and checks the platform parameter.
func validatePlatform(platform string) (string, bool) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	_, ok := validPlatforms[platform]
	return platform, ok
}

// validateTag checks the player tag against tagPattern.
func validateTag(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" || len(tag) > maxTagLength {
		return "", false
	}
	if !tagPattern.MatchString(tag) {
		return "", false
	}
	return tag, true
}

// requestParams extracts, validates and normalizes the platform and tag path
// parameters. It returns an HTTP error ready to be surfaced when either is invalid.
func requestParams(c echo.Context) (platform, tag string, err error) {
	platform, ok := validatePlatform(c.Param("platform"))
	if !ok {
		return "", "", newErr(http.StatusBadRequest,
			"Invalid platform. Expected 'pc' or 'console'.")
	}

	tag, ok = validateTag(c.Param("tag"))
	if !ok {
		return "", "", newErr(http.StatusBadRequest,
			"Invalid player tag.")
	}

	return platform, tag, nil
}
