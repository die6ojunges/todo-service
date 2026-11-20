package todo

import "testing"

func TestValidationErrorMessage(t *testing.T) {
	cases := []struct {
		err  ValidationError
		want string
	}{
		{ValidationError{Field: "title", Value: "  ", Rule: "must not be empty or blank"}, `invalid title="  ": must not be empty or blank`},
		{ValidationError{Field: "completed", Rule: "is required"}, `invalid completed: is required`},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}
