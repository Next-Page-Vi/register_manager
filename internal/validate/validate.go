// Package validate normalizes and validates user input shared by services.
package validate

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"register_manager/internal/apperr"
	"register_manager/internal/auth"
)

// UUID returns the canonical lowercase form of a client-supplied UUID.
func UUID(s, field string) (string, error) {
	u, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", apperr.Invalid("invalid_id").With("field", field)
	}
	return u.String(), nil
}

// NewID returns a new random UUID.
func NewID() string { return uuid.NewString() }

var localeRe = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Locale validates a BCP 47 language tag shape. The supported language list
// lives in the frontend, so adding a language needs no server change.
func Locale(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 35 || !localeRe.MatchString(s) {
		return "", apperr.Invalid("invalid_locale")
	}
	return s, nil
}

// OptionalLocale validates an optional locale; nil clears it.
func OptionalLocale(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}
	v, err := Locale(*s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// OptionalHexColor validates an optional "#rrggbb" colour and returns it in
// lowercase; nil clears it.
func OptionalHexColor(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}
	v := strings.ToLower(strings.TrimSpace(*s))
	if !hexColorRe.MatchString(v) {
		return nil, apperr.Invalid("invalid_color")
	}
	return &v, nil
}

// Timezone validates an IANA time zone name.
func Timezone(s string) (*time.Location, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "Local" {
		return nil, apperr.Invalid("invalid_timezone")
	}
	loc, err := time.LoadLocation(s)
	if err != nil {
		return nil, apperr.Invalid("invalid_timezone")
	}
	return loc, nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Slug validates an instance slug: 2-40 lowercase letters, digits and
// single hyphens.
func Slug(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 2 || len(s) > 40 || !slugRe.MatchString(s) {
		return "", apperr.Invalid("invalid_slug")
	}
	return s, nil
}

// Login validates a back-office login name: 1-64 characters without
// whitespace or control characters.
func Login(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 64 {
		return "", apperr.Invalid("invalid_login")
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", apperr.Invalid("invalid_login")
		}
	}
	return s, nil
}

// Name validates a person or instance name: free text of 1..max runes.
// Names are not split or restricted to any script.
func Name(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > max || hasControl(s) {
		return "", apperr.Invalid("invalid_name").With("max", max)
	}
	return s, nil
}

// Text validates optional free text up to max runes (newlines allowed).
func Text(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", apperr.Invalid("text_too_long").With("max", max)
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", apperr.Invalid("invalid_text")
		}
	}
	return s, nil
}

// Password validates a new password.
func Password(s string) error {
	n := utf8.RuneCountInString(s)
	if n < auth.MinPasswordLength || n > 256 {
		return apperr.Invalid("invalid_password").With("min", auth.MinPasswordLength)
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// OptionalText normalizes nonempty text. Clearing a value is explicit: nil.
func OptionalText(s *string, max int) (*string, error) {
	if s == nil {
		return nil, nil
	}
	value, err := Text(*s, max)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, apperr.Invalid("invalid_text")
	}
	return &value, nil
}
