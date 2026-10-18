package api

import (
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"register_manager/internal/booking"
	"register_manager/internal/paging"
)

// Huma query parameters cannot be pointers. Its wrapper interface preserves
// presence here while services receive ordinary optional values.
type optionalParam[T any] struct {
	parsed T
	value  *T
}

func (p *optionalParam[T]) Receiver() reflect.Value {
	return reflect.ValueOf(&p.parsed).Elem()
}
func (p *optionalParam[T]) OnParamSet(present bool, _ any) {
	if present {
		p.value = &p.parsed
	}
}
func (optionalParam[T]) Schema(registry huma.Registry) *huma.Schema {
	return huma.SchemaFromType(registry, reflect.TypeFor[T]())
}

func (p *optionalParam[T]) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	u := ctx.URL()
	name := strings.TrimPrefix(prefix.String(), "query.")
	if p.value == nil && u.Query().Has(name) {
		return []error{&huma.ErrorDetail{Location: prefix.String(), Message: "must not be empty"}}
	}
	return nil
}

type PageParams struct {
	Cursor optionalParam[string] `query:"cursor" minLength:"1"`
	Limit  optionalParam[int]    `query:"limit" minimum:"1" maximum:"200"`
}

func (p PageParams) Page() paging.Input {
	return paging.Input{Cursor: p.Cursor.value, Limit: p.Limit.value}
}

// Preserve an explicitly empty date so the service returns invalid_date,
// just as it does for malformed and out-of-range dates.
type dateParam struct{ optionalParam[string] }

func (p *dateParam) Resolve(ctx huma.Context, _ *huma.PathBuffer) []error {
	u := ctx.URL()
	if u.Query().Has("date") {
		value := u.Query().Get("date")
		p.value = &value
	}
	return nil
}

type AvailabilityParams struct {
	PageParams
	Date dateParam `query:"date" doc:"Instance-local date (YYYY-MM-DD); omitted means nearest times"`
}

func (p AvailabilityParams) Availability() booking.AvailabilityInput {
	return booking.AvailabilityInput{Input: p.Page(), Date: p.Date.value}
}
