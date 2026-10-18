package booking

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestPolicyBoundsMatchSchema(t *testing.T) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	schema := huma.SchemaFromType(registry, reflect.TypeFor[Policy]())
	for _, field := range policyFields {
		for _, value := range []int{field.min - 1, field.min, field.max, field.max + 1} {
			p := DefaultPolicy()
			p.DailyCustomerLimit, p.PerCustomerDailyLimit, p.PerCustomerWeeklyLimit, p.MaxFutureBookings = ptr(1), ptr(1), ptr(1), ptr(1)
			*field.value(&p) = value
			valid := value >= field.min && value <= field.max
			if (ValidatePolicy(p) == nil) != valid {
				t.Fatalf("service: %s=%d", field.name, value)
			}
			encoded, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var data any
			if err := json.Unmarshal(encoded, &data); err != nil {
				t.Fatal(err)
			}
			result := &huma.ValidateResult{}
			huma.Validate(registry, schema, &huma.PathBuffer{}, huma.ModeWriteToServer, data, result)
			if (len(result.Errors) == 0) != valid {
				t.Fatalf("schema: %s=%d: %v", field.name, value, result.Errors)
			}
		}
	}
	if err := ValidatePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
}
